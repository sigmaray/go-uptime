package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/internal/urlcheck"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// claimWaveMultiplier задаёт размер каждого claim относительно check concurrency, чтобы следующий
// claim мог начаться, пока предыдущая волна ещё завершает медленные проверки.
const claimWaveMultiplier = 2

// runDueMonitors захватывает ограниченный набор due-мониторов и запускает проверки без ожидания.
func (w *MonitorWorker) runDueMonitors() {
	// На паузе (e2e) не трогаем БД и не запускаем новые HTTP-проверки.
	if w.Paused() {
		return
	}

	// Backpressure: ноль, если волна или persist-очередь переполнены.
	limit := w.claimBudget()
	if limit < 1 {
		return
	}

	now := time.Now()
	// Транзакция с FOR UPDATE SKIP LOCKED — только due-строки, без блокировки чужих воркеров.
	due, err := w.claimDueMonitors(now, limit)
	if err != nil {
		log.Error().Err(err).Msg("failed to load due monitor urls")
		return
	}
	if len(due) == 0 {
		return
	}

	// Fire-and-forget goroutine на каждый монитор; loop не ждёт завершения.
	w.dispatchChecks(due, w.checkMonitor)
}

// claimWaveLimit — максимальное число мониторов, захватываемых за один scheduling tick.
// Возвращает удвоенный HTTP concurrency, чтобы медленные проверки не голодали следующий claim.
func (w *MonitorWorker) claimWaveLimit() int {
	return w.checkConcurrency * claimWaveMultiplier
}

// claimBudget — сколько дополнительных мониторов можно захватить с учётом текущей незавершённой работы.
// Возвращает ноль, когда уже захваченные проверки заполнили лимит волны (backpressure).
// Также ограничивает claim свободной ёмкостью persist, чтобы завершённые проверки не терялись,
// пока channel result или остатки flush в памяти переполнены.
func (w *MonitorWorker) claimBudget() int {
	limit := w.claimWaveLimit()
	pending := int(w.waveDue.Load())
	if pending >= limit {
		// Текущая волна ещё не освободила место — новый claim не берём.
		return 0
	}
	budget := limit - pending

	// Резервируем ёмкость persist для уже захваченных проверок и остатков flush
	// в batch loop (они больше не лежат в resultJobs). pending вычитаем отдельно:
	// эти мониторы уже claim-нуты и обязательно займут слот в очереди persist,
	// даже если HTTP-проверка ещё не завершилась.
	free := cap(w.resultJobs) - len(w.resultJobs) - int(w.persistBacklog.Load()) - pending
	if free < 1 {
		// Некуда класть завершённые проверки — останавливаем claim до flush.
		return 0
	}
	if budget > free {
		// Бюджет волны не может превышать свободные слоты persist.
		budget = free
	}
	return budget
}

// monitorScheduleUpdate — время следующей запланированной проверки одного захваченного монитора.
type monitorScheduleUpdate struct {
	// ID — monitor_urls.id строки, захваченной для текущей волны.
	ID uint
	// NextCheckAt — предварительное время, когда монитор можно снова проверить.
	NextCheckAt time.Time
}

// claimDueMonitors блокирует currently due мониторы и откладывает их следующую проверку до начала probing.
// now — текущее время worker для выбора due-строк и вычисления предварительного расписания.
// limit ограничивает число due-строк, захватываемых в этой транзакции; значения ниже 1 ничего не захватывают.
func (w *MonitorWorker) claimDueMonitors(now time.Time, limit int) ([]models.MonitorURL, error) {
	if limit < 1 {
		return nil, nil
	}

	var due []models.MonitorURL
	err := w.db.Transaction(func(tx *gorm.DB) error {
		// SKIP LOCKED: другой процесс уже держит строку — пропускаем, не ждём.
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("next_check_at IS NULL OR next_check_at <= ?", now).
			Order("next_check_at asc NULLS FIRST, id asc").
			Limit(limit).
			Find(&due).Error
		if err != nil {
			return err
		}
		if len(due) == 0 {
			return nil
		}

		// Глобальный интервал из app_settings; у монитора может быть свой override.
		globalInterval := models.GetCheckIntervalSeconds(tx)
		updates := make([]monitorScheduleUpdate, 0, len(due))
		for _, monitor := range due {
			intervalSec := models.MonitorCheckIntervalSeconds(monitor, globalInterval)
			// Сдвигаем next_check_at вперёд на lease — монитор не попадёт в повторный claim, пока идёт probe.
			updates = append(updates, monitorScheduleUpdate{
				ID:          monitor.ID,
				NextCheckAt: now.Add(claimLeaseDuration(intervalSec)),
			})
		}
		// Одним UPDATE CASE — атомарно для всех захваченных id в этой транзакции.
		return updateClaimedMonitorSchedules(tx, updates)
	})
	if err != nil {
		return nil, err
	}
	return due, nil
}

// claimLeaseDuration — как долго захваченный монитор остаётся вне due-очереди до завершения probing.
// intervalSeconds — настроенный интервал проверки монитора.
// Lease покрывает полную волну claim (claimWaveMultiplier раундов проверки) плюс короткий буфер flush, чтобы
// перекрывающиеся волны не могли повторно захватить монитор, пока он ждёт слота HTTP-semaphore,
// выполняется probe или ждёт запись batch в БД (processBatch перезапишет next_check_at окончательно).
func claimLeaseDuration(intervalSeconds int) time.Duration {
	interval := time.Duration(intervalSeconds) * time.Second
	// Худший случай: последний монитор в волне 2x concurrency ждёт полный раунд проверки (claimWaveMultiplier × timeout),
	// затем сам probe, затем до ~2 с batch flush — только после этого persist обновит расписание.
	lease := urlcheck.RequestTimeout*time.Duration(claimWaveMultiplier) + 2*time.Second
	if interval > lease {
		// Длинный интервал монитора — lease не короче настроенного периода.
		return interval
	}
	return lease
}

// updateClaimedMonitorSchedules записывает предварительные next_check_at одним SQL-обновлением.
// tx — транзакция, в которой due-строки мониторов уже заблокированы; updates — захваченные расписания.
func updateClaimedMonitorSchedules(tx *gorm.DB, updates []monitorScheduleUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	ids := make([]uint, 0, len(updates))
	for _, update := range updates {
		ids = append(ids, update.ID)
	}

	// Bulk UPDATE через CASE id WHEN ... — один round-trip вместо N UPDATE.
	caseSQL, args := buildNextCheckAtCaseExpression(updates)
	result := tx.Model(&models.MonitorURL{}).
		Where("id IN ?", ids).
		Update("next_check_at", gorm.Expr(caseSQL, args...))
	if result.Error != nil {
		return result.Error
	}
	// Все захваченные строки должны обновиться; иначе кто-то удалил монитор между SELECT и UPDATE.
	if result.RowsAffected != int64(len(updates)) {
		return fmt.Errorf("claimed %d monitor schedules, updated %d", len(updates), result.RowsAffected)
	}
	return nil
}

// buildNextCheckAtCaseExpression строит CASE-выражение для bulk-обновления расписаний.
// updates содержит id мониторов и соответствующие предварительные next_check_at.
// Ветки THEN приводятся к timestamptz, чтобы PostgreSQL принял результат CASE для next_check_at.
func buildNextCheckAtCaseExpression(updates []monitorScheduleUpdate) (string, []interface{}) {
	var b strings.Builder
	args := make([]interface{}, 0, len(updates)*2)
	b.WriteString("CASE id")
	for _, update := range updates {
		// Плейсхолдеры GORM подставят id и UTC-время для каждой ветки CASE.
		b.WriteString(" WHEN ? THEN ?::timestamptz")
		args = append(args, update.ID, update.NextCheckAt.UTC())
	}
	b.WriteString(" END")
	return b.String(), args
}

// dispatchChecks запускает goroutine проверок для мониторов и возвращается без ожидания.
// monitors — захваченный набор для проверки; checkFn выполняет одну проверку и должна быть безопасна для concurrent-вызовов.
// Общий semaphore ограничивает суммарную HTTP-нагрузку между перекрывающимися волнами.
// Слот concurrency освобождается до постановки результата в очередь, чтобы медленный flush в БД не
// удерживал HTTP-слоты, пока resultJobs заполнен.
func (w *MonitorWorker) dispatchChecks(monitors []models.MonitorURL, checkFn func(models.MonitorURL) checkResult) {
	if len(monitors) == 0 {
		return
	}

	// Сразу учитываем всю волну в waveDue — claimBudget видит backpressure.
	w.waveDue.Add(int64(len(monitors)))
	for _, monitor := range monitors {
		m := monitor
		w.wavesWG.Add(1)
		go func() {
			defer w.wavesWG.Done()
			defer w.waveDue.Add(-1)

			// Блокируемся на semaphore — лимит одновременных HTTP между волнами.
			w.checkSem <- struct{}{}
			w.waveStarted.Add(1)
			w.inFlight.Add(1)
			res := checkFn(m)
			// Освобождаем HTTP-слот до enqueue: медленный flush/persist не должен
			// удерживать semaphore, пока resultJobs переполнен или batch loop отстаёт.
			<-w.checkSem
			w.inFlight.Add(-1)
			w.waveStarted.Add(-1)

			// Блокирующая отправка — результат не теряем; claimBudget ограничивает новые claim.
			w.enqueueCheckResult(res)
		}()
	}
}

// enqueueCheckResult кладёт результат одной проверки в resultJobs, не удерживая check slot.
// res — завершённый результат проверки для сохранения.
// Отправка блокируется при полной очереди, чтобы результаты не терялись; HTTP-слоты уже
// освобождены, а claimBudget останавливает новые claim, пока очередь persist переполнена.
func (w *MonitorWorker) enqueueCheckResult(res checkResult) {
	// Намеренно блокируем goroutine — полная очередь означает backpressure на persist.
	w.resultJobs <- res
}

// IsMonitorDue сообщает, нужно ли проверять монитор в now по времени последней проверки.
// lastCheckedAt равен nil, если монитор ещё ни разу не проверялся.
// interval — эффективный интервал проверки монитора.
// now — текущее время для расчёта due.
func IsMonitorDue(lastCheckedAt *time.Time, interval time.Duration, now time.Time) bool {
	if lastCheckedAt == nil {
		// Никогда не проверяли — считаем due сразу.
		return true
	}
	return now.Sub(*lastCheckedAt) >= interval
}

// checkMonitor проверяет один монитор и возвращает результат для batch-сохранения.
// monitor — захваченная строка monitor_urls для проверки.
func (w *MonitorWorker) checkMonitor(monitor models.MonitorURL) checkResult {
	// Жёсткий таймаут на весь HTTP round-trip — не зависаем на медленных хостах.
	ctx, cancel := context.WithTimeout(context.Background(), urlcheck.RequestTimeout)
	defer cancel()

	displayName := models.MonitorDisplayName(monitor)
	result := urlcheck.Probe(ctx, w.client, monitor.URL)
	elapsed := int(result.DurationMs)

	if !result.Up {
		// In-memory ring log для UI «последние запросы» — не ждём batch flush.
		w.recordMonitorRequest(displayName, monitor.URL, result.StatusCode, result.DurationMs, false, result.ErrMsg)
		return checkResult{monitor: monitor, isUp: false, errMsg: result.ErrMsg, elapsed: intPtr(elapsed)}
	}

	w.recordMonitorRequest(displayName, monitor.URL, result.StatusCode, result.DurationMs, true, "")
	return checkResult{monitor: monitor, isUp: true, errMsg: "", elapsed: intPtr(elapsed)}
}

// recordMonitorRequest сохраняет результат одной проверки в кольцевой in-memory request log.
// monitorName — отображаемое имя; url — проверяемый адрес; statusCode и responseTimeMs
// описывают HTTP-результат; isUp — доступность; errMsg объясняет сбои.
func (w *MonitorWorker) recordMonitorRequest(monitorName, url string, statusCode int, responseTimeMs int64, isUp bool, errMsg string) {
	// Потокобезопасный ring buffer — UI diagnostics без запроса к БД.
	applog.AddMonitorRequest(monitorName, url, statusCode, responseTimeMs, isUp, errMsg)
}

// intPtr возвращает указатель на v для optional integer полей.
// v — целое значение для boxing.
func intPtr(v int) *int {
	return &v
}
