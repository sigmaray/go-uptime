package worker

import (
	"fmt"
	"strings"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// batchResultsLoop получает результаты проверок от worker goroutine и собирает их в batch.
// Запись в БД пакетами (например, каждые 2 секунды или 150 элементов)
// избегает N+1 записей и существенно снижает накладные расходы транзакций PostgreSQL.
func (w *MonitorWorker) batchResultsLoop() {
	defer close(w.batchDone)

	// Flush batch каждые 2 секунды, даже если лимит размера batch ещё не достигнут.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Локальный слайс — между flush сюда копятся результаты из resultJobs.
	var batch []checkResult

	for {
		select {
		case res, ok := <-w.resultJobs:
			if !ok {
				// Channel result закрыт (worker останавливается); сливаем остатки локально.
				for len(batch) > 0 {
					batch = w.flushBatch(batch)
					w.persistBacklog.Store(int64(len(batch)))
				}
				return
			}
			batch = append(batch, res)
			// Метрика для Stats и claimBudget — сколько ждёт записи вне channel.
			w.persistBacklog.Store(int64(len(batch)))
			// При достижении лимита размера batch (например, 150) сразу выполняем flush.
			if len(batch) >= 150 {
				batch = w.flushBatch(batch)
				w.persistBacklog.Store(int64(len(batch)))
			}
		case <-ticker.C:
			// Периодический flush — не держим результаты в памяти дольше 2 с.
			if len(batch) > 0 {
				batch = w.flushBatch(batch)
				w.persistBacklog.Store(int64(len(batch)))
			}
		}
	}
}

// flushBatch сохраняет завершённый batch результатов и кратко повторяет попытку при сбое.
// batch — набор завершённых проверок, готовых к записи в PostgreSQL.
// Возвращает результаты, которым нужна ещё одна попытка flush (nil/пустой при успехе).
// Остатки остаются в batch loop, чтобы полный channel resultJobs не терял их
// и goroutine batch никогда не блокировалась при отправке в channel, который читает только она.
func (w *MonitorWorker) flushBatch(batch []checkResult) []checkResult {
	const maxImmediateAttempts = 3

	var err error
	for attempt := 1; attempt <= maxImmediateAttempts; attempt++ {
		err = w.processBatch(batch)
		if err == nil {
			// Успех — локальный batch можно отбросить.
			return nil
		}
		log.Error().Err(err).Int("attempt", attempt).Msg("failed to process monitor checks batch")
		if attempt < maxImmediateAttempts {
			// Короткая пауза перед повтором — transient ошибки БД/сети.
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}
	}
	// Не удалось за три попытки — оставляем в batch loop с увеличенным persistAttempts.
	return retainFailedBatch(batch)
}

// maxPersistRequeues ограничивает, сколько раз один результат проверки можно повторить после неудачных flush.
const maxPersistRequeues = 5

// retainFailedBatch увеличивает счётчик persist attempts и сохраняет результаты, которые ещё можно повторить.
// batch — набор, исчерпавший немедленные повторы flush.
// Результаты, превысившие maxPersistRequeues, логируются и отбрасываются.
func retainFailedBatch(batch []checkResult) []checkResult {
	out := make([]checkResult, 0, len(batch))
	for _, res := range batch {
		res.persistAttempts++
		if res.persistAttempts > maxPersistRequeues {
			// Исчерпали лимит — логируем и отбрасываем, чтобы не зациклиться навечно.
			log.Error().
				Uint("monitor_id", res.monitor.ID).
				Int("persist_attempts", res.persistAttempts).
				Msg("dropping monitor check result after persist retries")
			continue
		}
		// Вернётся в batch loop — попробуем flush на следующем tick или при новых result.
		out = append(out, res)
	}
	return out
}

// monitorStatusUpdate содержит поля строки монитора, записываемые после завершённой проверки.
type monitorStatusUpdate struct {
	// ID — monitor_urls.id, который обновляется.
	ID uint
	// IsUp — последняя доступность по проверке.
	IsUp bool
	// NextCheckAt — финальное расписание после завершения проверки.
	NextCheckAt time.Time
	// LastError — текст ошибки проверки; пустой при успешной проверке.
	LastError string
}

// processBatch выполняет одну атомарную транзакцию БД для сохранения результатов проверок.
// Массово обновляет статусы мониторов, отслеживает время начала/окончания incidents,
// пересчитывает uptime stats и массово вставляет записи истории отдельных проверок.
// batch — набор завершённых HTTP-проверок для сохранения.
func (w *MonitorWorker) processBatch(batch []checkResult) error {
	if len(batch) == 0 {
		return nil
	}

	// Одна строка на monitor id — перекрывающиеся волны могут дать дубликаты.
	batch = collapseBatchByMonitor(batch)
	now := time.Now()

	err := w.db.Transaction(func(tx *gorm.DB) error {
		ids := make([]uint, 0, len(batch))
		for _, res := range batch {
			ids = append(ids, res.monitor.ID)
		}
		var existingIDs []uint
		if err := tx.Model(&models.MonitorURL{}).Where("id IN ?", ids).Pluck("id", &existingIDs).Error; err != nil {
			return fmt.Errorf("load existing monitors for batch: %w", err)
		}
		if len(existingIDs) == 0 {
			// Все мониторы удалены после claim — batch бессмысленен.
			return fmt.Errorf("bulk update monitor statuses: none of %d monitors still exist", len(batch))
		}
		// Монитор мог быть удалён в UI/API после claim и до flush batch.
		// Результат проверки для несуществующей строки бессмысленен — отбрасываем,
		// чтобы не писать checks/incidents/uptime для «осиротевших» id.
		if len(existingIDs) != len(batch) {
			exist := make(map[uint]struct{}, len(existingIDs))
			for _, id := range existingIDs {
				exist[id] = struct{}{}
			}
			filtered := make([]checkResult, 0, len(existingIDs))
			for _, res := range batch {
				if _, ok := exist[res.monitor.ID]; ok {
					filtered = append(filtered, res)
				}
			}
			log.Warn().
				Int("dropped", len(batch)-len(filtered)).
				Int("remaining", len(filtered)).
				Msg("dropping deleted monitors from check result batch")
			batch = filtered
		}

		checks := make([]models.MonitorCheck, 0, len(batch))
		statusUpdates := make([]monitorStatusUpdate, 0, len(batch))
		globalInterval := models.GetCheckIntervalSeconds(tx)

		for _, res := range batch {
			// Финальное next_check_at — от now + интервал, перезаписывает lease из claim.
			nextAt := now.Add(time.Duration(models.MonitorCheckIntervalSeconds(res.monitor, globalInterval)) * time.Second)
			statusUpdates = append(statusUpdates, monitorStatusUpdate{
				ID:          res.monitor.ID,
				IsUp:        res.isUp,
				NextCheckAt: nextAt,
				LastError:   res.errMsg,
			})

			// res.monitor.LastCheckedAt — снимок на момент claim (предыдущая граница интервала),
			// а не значение после текущего flush; uptime считается между этими двумя точками.
			if err := models.ApplyUptimeInterval(tx, res.monitor.ID, res.monitor.LastCheckedAt, now, res.isUp); err != nil {
				return fmt.Errorf("update uptime stats for monitor %d: %w", res.monitor.ID, err)
			}

			checks = append(checks, models.MonitorCheck{
				MonitorURLID:   res.monitor.ID,
				CheckedAt:      now,
				IsUp:           res.isUp,
				ResponseTimeMs: res.elapsed,
			})
		}

		if err := updateMonitorStatuses(tx, now, statusUpdates); err != nil {
			return err
		}

		if err := applyIncidentChanges(tx, now, batch); err != nil {
			return err
		}

		if len(checks) > 0 {
			// Массовая вставка истории проверок одним INSERT.
			if err := tx.Create(&checks).Error; err != nil {
				return fmt.Errorf("insert monitor checks: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Запускаем уведомления вне транзакции БД, чтобы не удерживать блокировки.
	// wasUp/wasDown берутся из снимка до flush: при IsUp == nil (первая проверка) оба false —
	// перехода ещё не было, уведомление не шлём. Оповещаем только при смене состояния UP↔DOWN.
	for _, res := range batch {
		var wasUp, wasDown bool
		if res.monitor.IsUp != nil {
			wasUp = *res.monitor.IsUp
			wasDown = !wasUp
		}

		if res.isUp && wasDown {
			applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is UP", models.MonitorDisplayName(res.monitor), res.monitor.URL))
			w.enqueueNotification(res.monitor, true, "")
		} else if !res.isUp && wasUp {
			applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is DOWN: %s", models.MonitorDisplayName(res.monitor), res.monitor.URL, res.errMsg))
			w.enqueueNotification(res.monitor, false, res.errMsg)
		}
	}
	return nil
}

// collapseBatchByMonitor оставляет последний результат на монитор, чтобы bulk-обновления были по одной строке на id.
// Перекрывающиеся волны claim могут вернуть несколько результатов с одним monitor id — оставляем последний
// (самый свежий исход проверки). batch — слайс результатов после flush, который может содержать дубликаты id.
func collapseBatchByMonitor(batch []checkResult) []checkResult {
	if len(batch) <= 1 {
		return batch
	}

	indexByID := make(map[uint]int, len(batch))
	out := make([]checkResult, 0, len(batch))
	for _, res := range batch {
		if i, ok := indexByID[res.monitor.ID]; ok {
			// Повторный id — перезаписываем более старый результат более свежим.
			out[i] = res
			continue
		}
		indexByID[res.monitor.ID] = len(out)
		out = append(out, res)
	}
	return out
}

// updateMonitorStatuses записывает is_up, last_checked_at, next_check_at и last_error для многих мониторов.
// tx — открытая транзакция; now — общий last_checked_at; updates — поля по каждому монитору.
func updateMonitorStatuses(tx *gorm.DB, now time.Time, updates []monitorStatusUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	ids := make([]uint, 0, len(updates))
	var isUpCase, nextAtCase, errCase strings.Builder
	isUpArgs := make([]interface{}, 0, len(updates)*2)
	nextAtArgs := make([]interface{}, 0, len(updates)*2)
	errArgs := make([]interface{}, 0, len(updates)*2)

	// Три параллельных CASE-выражения — один UPDATE на все поля статуса.
	isUpCase.WriteString("CASE id")
	nextAtCase.WriteString("CASE id")
	errCase.WriteString("CASE id")
	for _, update := range updates {
		ids = append(ids, update.ID)

		isUpCase.WriteString(" WHEN ? THEN ?::boolean")
		isUpArgs = append(isUpArgs, update.ID, update.IsUp)

		nextAtCase.WriteString(" WHEN ? THEN ?::timestamptz")
		nextAtArgs = append(nextAtArgs, update.ID, update.NextCheckAt.UTC())

		errCase.WriteString(" WHEN ? THEN ?::text")
		errArgs = append(errArgs, update.ID, update.LastError)
	}
	isUpCase.WriteString(" END")
	nextAtCase.WriteString(" END")
	errCase.WriteString(" END")

	// last_checked_at общий для всего batch — момент завершения flush.
	result := tx.Model(&models.MonitorURL{}).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"is_up":           gorm.Expr(isUpCase.String(), isUpArgs...),
			"last_checked_at": now,
			"next_check_at":   gorm.Expr(nextAtCase.String(), nextAtArgs...),
			"last_error":      gorm.Expr(errCase.String(), errArgs...),
		})
	if result.Error != nil {
		return fmt.Errorf("bulk update monitor statuses: %w", result.Error)
	}
	if result.RowsAffected != int64(len(updates)) {
		// Часть строк исчезла между проверкой existingIDs и UPDATE — считаем ошибкой.
		return fmt.Errorf("bulk update monitor statuses: updated %d of %d", result.RowsAffected, len(updates))
	}
	return nil
}

// applyIncidentChanges закрывает, создаёт или обновляет открытые incidents для завершённого batch проверок.
// tx — открытая транзакция; now — время закрытия/начала incident; batch содержит исходы проверок.
func applyIncidentChanges(tx *gorm.DB, now time.Time, batch []checkResult) error {
	monitorIDs := make([]uint, 0, len(batch))
	for _, res := range batch {
		monitorIDs = append(monitorIDs, res.monitor.ID)
	}

	// Один запрос — все открытые incidents для мониторов из batch.
	openByMonitor, err := models.FindOpenIncidentsByMonitorIDs(tx, monitorIDs)
	if err != nil {
		return fmt.Errorf("load open incidents: %w", err)
	}

	resolveIDs := make([]uint, 0)
	createIncidents := make([]models.Incident, 0)
	for _, res := range batch {
		openIncident, hasOpen := openByMonitor[res.monitor.ID]
		if res.isUp {
			// Монитор восстановился — закрываем открытый incident, если он есть.
			if hasOpen {
				resolveIDs = append(resolveIDs, openIncident.ID)
			}
			continue
		}
		if !hasOpen {
			// Монитор упал без открытого incident — создаём новый.
			createIncidents = append(createIncidents, models.Incident{
				MonitorURLID: res.monitor.ID,
				StartedAt:    now,
				ErrorMessage: res.errMsg,
			})
			continue
		}
		// Incident уже открыт — обновляем текст ошибки, если он изменился.
		if openIncident.ErrorMessage != res.errMsg {
			if err := tx.Model(&models.Incident{}).Where("id = ?", openIncident.ID).
				Update("error_message", res.errMsg).Error; err != nil {
				return fmt.Errorf("update incident %d for monitor %d: %w", openIncident.ID, res.monitor.ID, err)
			}
		}
	}

	if len(resolveIDs) > 0 {
		// Bulk resolve — resolved_at = now для всех восстановившихся мониторов.
		if err := tx.Model(&models.Incident{}).Where("id IN ?", resolveIDs).
			Update("resolved_at", now).Error; err != nil {
			return fmt.Errorf("resolve incidents: %w", err)
		}
	}
	if len(createIncidents) > 0 {
		for i := range createIncidents {
			if err := tx.Create(&createIncidents[i]).Error; err != nil {
				// Гонка с другим batch/worker: unique one-open-per-monitor уже создал incident — пропускаем.
				if models.IsUniqueViolation(err) {
					continue
				}
				return fmt.Errorf("create incident for monitor %d: %w", createIncidents[i].MonitorURLID, err)
			}
		}
	}
	return nil
}
