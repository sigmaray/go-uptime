package worker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-uptime/config"
	"go-uptime/internal/urlcheck"
	"go-uptime/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMonitorWorkerRunning(t *testing.T) {
	// Assert: nil worker никогда не считается running.
	var nilWorker *MonitorWorker
	if nilWorker.Running() {
		t.Fatal("nil worker should not be running")
	}

	// Arrange: новый worker до Start().
	w := New(nil, &config.Config{CheckConcurrency: 1})
	if w.Running() {
		t.Fatal("expected worker not running before Start")
	}

	// Act: имитируем активный worker без запуска циклов с БД.
	w.started.Store(true)
	// Assert: started=true и открытый stop channel → Running()=true.
	if !w.Running() {
		t.Fatal("expected worker running while started and stop channel open")
	}

	// Act: закрываем stop — worker должен перестать быть running.
	close(w.stop)
	if w.Running() {
		t.Fatal("expected worker not running after stop channel closed")
	}
}

func TestMonitorWorkerPauseSkipsDueMonitors(t *testing.T) {
	// Assert: Pause/Resume на nil worker безопасны и не ставят paused.
	var nilWorker *MonitorWorker
	nilWorker.Pause()
	nilWorker.Resume()
	if nilWorker.Paused() {
		t.Fatal("nil worker should not report paused")
	}

	// Arrange + Act: пауза worker без БД.
	w := New(nil, &config.Config{CheckConcurrency: 1})
	w.Pause()
	if !w.Paused() {
		t.Fatal("expected worker paused after Pause")
	}
	// Assert: при паузе runDueMonitors не обращается к nil DB.
	w.runDueMonitors()

	// Act + Assert: Resume снимает паузу.
	w.Resume()
	if w.Paused() {
		t.Fatal("expected worker not paused after Resume")
	}
}

func TestSetBrowserLikeHeaders(t *testing.T) {
	// Arrange: GET-запрос без заголовков.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	// Act: добавляем browser-like заголовки как при реальной проверке URL.
	urlcheck.SetBrowserLikeHeaders(req)

	// Assert: все обязательные заголовки непустые.
	want := []string{
		"User-Agent",
		"Accept",
		"Accept-Language",
		"Upgrade-Insecure-Requests",
		"Sec-Fetch-Dest",
		"Sec-Fetch-Mode",
		"Sec-Fetch-Site",
		"Sec-Fetch-User",
	}
	for _, name := range want {
		if got := req.Header.Get(name); got == "" {
			t.Fatalf("%s header is empty", name)
		}
	}
}

func TestIsMonitorDue(t *testing.T) {
	// Arrange: интервал 1 минута и фиксированное «сейчас».
	interval := time.Minute
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	t.Run("never checked", func(t *testing.T) {
		// Act + Assert: nil LastCheckedAt — первая проверка всегда due.
		if !IsMonitorDue(nil, interval, now) {
			t.Fatal("expected first check to be due")
		}
	})

	t.Run("interval not elapsed", func(t *testing.T) {
		// Arrange: последняя проверка 30 секунд назад при интервале 60.
		last := now.Add(-30 * time.Second)
		// Act + Assert: ещё рано.
		if IsMonitorDue(&last, interval, now) {
			t.Fatal("expected monitor to be skipped")
		}
	})

	t.Run("interval elapsed", func(t *testing.T) {
		// Arrange: прошла полная минута с last check.
		last := now.Add(-time.Minute)
		// Act + Assert: монитор due.
		if !IsMonitorDue(&last, interval, now) {
			t.Fatal("expected monitor to be due")
		}
	})
}

func TestRunChecksConcurrentlyRespectsLimit(t *testing.T) {
	// Arrange: 20 мониторов, лимит 3 goroutine, каждая check длится 30ms.
	const (
		monitorCount  = 20
		maxConcurrent = 3
		checkDuration = 30 * time.Millisecond
	)

	monitors := make([]models.MonitorURL, monitorCount)
	for i := range uint(monitorCount) {
		monitors[i].ID = i + 1
	}

	var (
		inFlight atomic.Int32
		peak     atomic.Int32
		checked  atomic.Int32
		mu       sync.Mutex
		seen     = make(map[uint]bool)
	)

	// Act: запускаем concurrent checks с подсчётом peak in-flight.
	start := time.Now()
	runChecksConcurrently(monitors, maxConcurrent, func(m models.MonitorURL) {
		current := inFlight.Add(1)
		for {
			prev := peak.Load()
			if current <= prev || peak.CompareAndSwap(prev, current) {
				break
			}
		}

		time.Sleep(checkDuration)

		inFlight.Add(-1)
		checked.Add(1)

		mu.Lock()
		seen[m.ID] = true
		mu.Unlock()
	})
	elapsed := time.Since(start)

	// Assert: все мониторы проверены ровно один раз.
	if got := int(checked.Load()); got != monitorCount {
		t.Fatalf("checked %d monitors, want %d", got, monitorCount)
	}
	// Assert: peak concurrency не превышает лимит.
	if got := int(peak.Load()); got > maxConcurrent {
		t.Fatalf("peak concurrency %d exceeds limit %d", got, maxConcurrent)
	}
	if len(seen) != monitorCount {
		t.Fatalf("saw %d unique monitors, want %d", len(seen), monitorCount)
	}

	// Assert: elapsed меньше serial floor — проверки шли параллельно, не последовательно.
	serialFloor := time.Duration(monitorCount) * checkDuration
	if elapsed >= serialFloor {
		t.Fatalf("elapsed %v looks serial (serial floor %v)", elapsed, serialFloor)
	}
}

func TestRunChecksConcurrentlyEmptyAndInvalidLimit(t *testing.T) {
	// Act + Assert: nil/пустой список — checkFn не вызывается.
	runChecksConcurrently(nil, 5, func(models.MonitorURL) {
		t.Fatal("checkFn must not run for empty list")
	})

	// Arrange: один монитор и maxConcurrent=0 (нормализуется в 1).
	var ran atomic.Int32
	runChecksConcurrently([]models.MonitorURL{{ID: 1}}, 0, func(models.MonitorURL) {
		ran.Add(1)
	})
	// Assert: ровно один вызов checkFn.
	if ran.Load() != 1 {
		t.Fatalf("expected single check with invalid limit, got %d", ran.Load())
	}
}

func TestStatsTracksWaveProgress(t *testing.T) {
	// Arrange: worker с concurrency=2, 4 монитора, checkFn блокируется на release.
	w := New(nil, &config.Config{CheckConcurrency: 2})

	release := make(chan struct{})
	started := make(chan struct{}, 4)
	monitors := []models.MonitorURL{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}

	// Act: dispatchChecks в фоне — первые 2 goroutine стартуют и ждут release.
	w.dispatchChecks(monitors, func(models.MonitorURL) checkResult {
		started <- struct{}{}
		<-release
		return checkResult{}
	})

	// Assert: две проверки уже in-flight.
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for in-flight checks")
		}
	}

	// Assert: Stats отражает 4 due, 2 in-flight, 2 waiting, лимиты очередей.
	stats := w.Stats()
	if stats.DueThisWave != 4 {
		t.Fatalf("DueThisWave = %d, want 4", stats.DueThisWave)
	}
	if stats.InFlight != 2 {
		t.Fatalf("InFlight = %d, want 2", stats.InFlight)
	}
	if stats.WaitingForSlot != 2 {
		t.Fatalf("WaitingForSlot = %d, want 2", stats.WaitingForSlot)
	}
	if stats.MaxConcurrency != 2 {
		t.Fatalf("MaxConcurrency = %d, want 2", stats.MaxConcurrency)
	}
	if stats.NotifyCapacity != notifyQueueSize {
		t.Fatalf("NotifyCapacity = %d, want %d", stats.NotifyCapacity, notifyQueueSize)
	}
	if stats.NotifyQueued != 0 {
		t.Fatalf("NotifyQueued = %d, want 0", stats.NotifyQueued)
	}

	// Act: отпускаем все checks и ждём завершения волны.
	close(release)
	w.wavesWG.Wait()

	// Assert: после волны счётчики обнуляются.
	stats = w.Stats()
	if stats.DueThisWave != 0 || stats.InFlight != 0 || stats.WaitingForSlot != 0 {
		t.Fatalf("expected idle stats after wave, got %+v", stats)
	}
}

func TestClaimBudgetBackpressure(t *testing.T) {
	// Arrange: worker с concurrency=2 → wave limit = 4.
	w := New(nil, &config.Config{CheckConcurrency: 2})
	// Act + Assert: пустая волна — полный budget.
	if got := w.claimBudget(); got != 4 {
		t.Fatalf("claimBudget() = %d, want 4", got)
	}

	// Arrange: все 4 слота волны уже «заняты» waveDue.
	w.waveDue.Store(4)
	if got := w.claimBudget(); got != 0 {
		t.Fatalf("claimBudget() with full pending = %d, want 0", got)
	}

	// Arrange: один слот освобождён.
	w.waveDue.Store(1)
	if got := w.claimBudget(); got != 3 {
		t.Fatalf("claimBudget() with one pending = %d, want 3", got)
	}
}

func TestClaimBudgetResultQueueBackpressure(t *testing.T) {
	// Arrange: resultJobs заполнена до resultQueueSize.
	w := New(nil, &config.Config{CheckConcurrency: 2})

	for i := uint(1); i <= uint(resultQueueSize); i++ {
		w.resultJobs <- checkResult{monitor: models.MonitorURL{ID: i}}
	}
	// Act + Assert: при полной result-очереди budget=0.
	if got := w.claimBudget(); got != 0 {
		t.Fatalf("claimBudget() with full result queue = %d, want 0", got)
	}

	// Act: освобождаем один слот и сбрасываем waveDue.
	<-w.resultJobs
	w.waveDue.Store(0)
	if got := w.claimBudget(); got != 1 {
		t.Fatalf("claimBudget() with one free result slot = %d, want 1", got)
	}
}

func TestClaimBudgetPersistBacklogBackpressure(t *testing.T) {
	// Arrange: persistBacklog на максимуме — нельзя claim новые результаты.
	w := New(nil, &config.Config{CheckConcurrency: 2})
	w.persistBacklog.Store(int64(resultQueueSize))
	if got := w.claimBudget(); got != 0 {
		t.Fatalf("claimBudget() with full persist backlog = %d, want 0", got)
	}

	// Act + Assert: один слот persist backlog освобождён — budget=1.
	w.persistBacklog.Store(int64(resultQueueSize - 1))
	if got := w.claimBudget(); got != 1 {
		t.Fatalf("claimBudget() with one free persist slot = %d, want 1", got)
	}
}

func TestEnqueueCheckResultBlocksUntilSpace(t *testing.T) {
	// Arrange: resultJobs полностью заполнена.
	w := New(nil, &config.Config{CheckConcurrency: 1})
	for i := uint(1); i <= uint(resultQueueSize); i++ {
		w.resultJobs <- checkResult{monitor: models.MonitorURL{ID: i}}
	}

	// Act: enqueue в goroutine — должен блокироваться, пока нет места.
	done := make(chan struct{})
	go func() {
		w.enqueueCheckResult(checkResult{monitor: models.MonitorURL{ID: 9999}, isUp: true})
		close(done)
	}()

	// Assert: за 50ms enqueue не завершился — блокировка работает.
	select {
	case <-done:
		t.Fatal("enqueueCheckResult returned while result queue was full")
	case <-time.After(50 * time.Millisecond):
	}

	// Act: освобождаем один слот.
	<-w.resultJobs

	// Assert: enqueue завершается после появления места.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("enqueueCheckResult did not complete after space freed")
	}
	if len(w.resultJobs) != resultQueueSize {
		t.Fatalf("result queue length = %d, want %d", len(w.resultJobs), resultQueueSize)
	}
}

func TestRetainFailedBatchDropsAfterMaxAttempts(t *testing.T) {
	// Arrange: batch с persistAttempts == max — элемент должен отброситься.
	batch := []checkResult{{
		monitor:         models.MonitorURL{ID: 1},
		persistAttempts: maxPersistRequeues,
	}}
	// Act + Assert: пустой результат после max попыток.
	got := retainFailedBatch(batch)
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0 after max attempts", len(got))
	}

	// Arrange: на одну попытку меньше max — элемент остаётся с инкрементом.
	batch = []checkResult{{
		monitor:         models.MonitorURL{ID: 2},
		persistAttempts: maxPersistRequeues - 1,
	}}
	got = retainFailedBatch(batch)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].persistAttempts != maxPersistRequeues {
		t.Fatalf("persistAttempts = %d, want %d", got[0].persistAttempts, maxPersistRequeues)
	}
}

func TestClaimLeaseDurationAtLeastProbeTimeout(t *testing.T) {
	// Act + Assert: короткий интервал — lease не меньше probe timeout * multiplier + запас.
	wantMin := urlcheck.RequestTimeout*time.Duration(claimWaveMultiplier) + 2*time.Second
	if got := claimLeaseDuration(10); got != wantMin {
		t.Fatalf("claimLeaseDuration(10) = %v, want %v", got, wantMin)
	}
	// Act + Assert: длинный интервал — lease равен интервалу в секундах.
	if got := claimLeaseDuration(120); got != 120*time.Second {
		t.Fatalf("claimLeaseDuration(120) = %v, want 120s", got)
	}
}

func TestStatsReportsNotifyQueueDepth(t *testing.T) {
	// Arrange: worker и одно notify job в очереди.
	w := New(nil, &config.Config{})
	w.enqueueNotification(models.MonitorURL{ID: 1, NotifySMTP: true}, false, "down")
	// Act + Assert: NotifyQueued отражает глубину очереди.
	stats := w.Stats()
	if stats.NotifyQueued != 1 {
		t.Fatalf("NotifyQueued = %d, want 1", stats.NotifyQueued)
	}
}

func TestClaimDueMonitorsPostponesScheduleBeforeChecks(t *testing.T) {
	// Arrange: три монитора — never checked, past due, future next_check_at.
	db := openWorkerTestDB(t)
	resetWorkerTestTables(t, db)

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	duePast := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	monitors := []models.MonitorURL{
		{Name: "never checked", URL: "https://never.example.com"},
		{Name: "past due", URL: "https://past.example.com", NextCheckAt: &duePast},
		{Name: "future", URL: "https://future.example.com", NextCheckAt: &future},
	}
	if err := db.Create(&monitors).Error; err != nil {
		t.Fatalf("create monitors: %v", err)
	}

	w := New(db, &config.Config{CheckConcurrency: 1})
	// Act: claim due — только never checked и past due.
	claimed, err := w.claimDueMonitors(now, w.claimWaveLimit())
	if err != nil {
		t.Fatalf("claimDueMonitors: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %d monitors, want 2", len(claimed))
	}

	// Assert: у claimed мониторов next_check_at сдвинут в будущее (lease).
	var refreshed []models.MonitorURL
	if err := db.Order("id asc").Find(&refreshed).Error; err != nil {
		t.Fatalf("reload monitors: %v", err)
	}
	if len(refreshed) != 3 {
		t.Fatalf("reloaded %d monitors, want 3", len(refreshed))
	}
	for i := 0; i < 2; i++ {
		monitor := refreshed[i]
		if monitor.NextCheckAt == nil {
			t.Fatalf("monitor %d next_check_at is nil after claim", monitor.ID)
		}
		if !monitor.NextCheckAt.After(now) {
			t.Fatalf("monitor %d next_check_at = %v, want after %v", monitor.ID, monitor.NextCheckAt, now)
		}
		// Короткие интервалы всё равно дают lease, достаточный для полной перекрывающейся волны.
		minLease := now.Add(urlcheck.RequestTimeout*time.Duration(claimWaveMultiplier) + 2*time.Second)
		if monitor.NextCheckAt.Before(minLease) {
			t.Fatalf("monitor %d next_check_at = %v, want >= %v", monitor.ID, monitor.NextCheckAt, minLease)
		}
	}

	// Act + Assert: повторный claim сразу не возвращает уже «занятые» мониторы.
	claimedAgain, err := w.claimDueMonitors(now.Add(time.Second), w.claimWaveLimit())
	if err != nil {
		t.Fatalf("second claimDueMonitors: %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("second claim returned %d monitors, want 0", len(claimedAgain))
	}
}

func TestClaimDueMonitorsRespectsLimit(t *testing.T) {
	// Arrange: три due монитора, limit claim = 2.
	db := openWorkerTestDB(t)
	resetWorkerTestTables(t, db)

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	duePast := now.Add(-time.Minute)
	monitors := []models.MonitorURL{
		{Name: "a", URL: "https://a.example.com", NextCheckAt: &duePast},
		{Name: "b", URL: "https://b.example.com", NextCheckAt: &duePast},
		{Name: "c", URL: "https://c.example.com", NextCheckAt: &duePast},
	}
	if err := db.Create(&monitors).Error; err != nil {
		t.Fatalf("create monitors: %v", err)
	}

	w := New(db, &config.Config{CheckConcurrency: 10})
	// Act: первый claim с limit=2.
	claimed, err := w.claimDueMonitors(now, 2)
	if err != nil {
		t.Fatalf("claimDueMonitors: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %d monitors, want 2", len(claimed))
	}

	// Act + Assert: второй claim забирает оставшегося одного.
	remaining, err := w.claimDueMonitors(now, 10)
	if err != nil {
		t.Fatalf("second claimDueMonitors: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining claimed %d, want 1", len(remaining))
	}
}

func TestProcessBatchReturnsErrorForMissingMonitor(t *testing.T) {
	// Arrange: batch с несуществующим monitor ID.
	db := openWorkerTestDB(t)
	resetWorkerTestTables(t, db)

	w := New(db, &config.Config{CheckConcurrency: 1})
	// Act: processBatch для missing monitor.
	err := w.processBatch([]checkResult{
		{
			monitor: models.MonitorURL{
				ID:  999,
				URL: "https://missing.example.com",
			},
			isUp: true,
		},
	})
	// Assert: ошибка, check в БД не создаётся.
	if err == nil {
		t.Fatal("processBatch returned nil error for missing monitor")
	}

	var checks int64
	if err := db.Model(&models.MonitorCheck{}).Count(&checks).Error; err != nil {
		t.Fatalf("count monitor checks: %v", err)
	}
	if checks != 0 {
		t.Fatalf("monitor checks count = %d, want 0", checks)
	}
}

func TestProcessBatchBulkUpdatesMonitorsAndIncidents(t *testing.T) {
	// Arrange: recovering (down→up, open incident) и failing (up→down).
	db := openWorkerTestDB(t)
	resetWorkerTestTables(t, db)

	wasUp := true
	wasDown := false
	lastChecked := time.Date(2026, 7, 25, 11, 58, 0, 0, time.UTC)
	monitors := []models.MonitorURL{
		{Name: "recovering", URL: "https://up.example.com", IsUp: &wasDown, LastCheckedAt: &lastChecked},
		{Name: "failing", URL: "https://down.example.com", IsUp: &wasUp, LastCheckedAt: &lastChecked},
	}
	if err := db.Create(&monitors).Error; err != nil {
		t.Fatalf("create monitors: %v", err)
	}
	if err := db.Create(&models.Incident{
		MonitorURLID: monitors[0].ID,
		StartedAt:    lastChecked,
		ErrorMessage: "old",
	}).Error; err != nil {
		t.Fatalf("create open incident: %v", err)
	}

	elapsed := 12
	w := New(db, &config.Config{CheckConcurrency: 2})
	// Act: batch — monitor[0] up (resolve incident), monitor[1] down (new incident).
	err := w.processBatch([]checkResult{
		{monitor: monitors[0], isUp: true, elapsed: &elapsed},
		{monitor: monitors[1], isUp: false, errMsg: "timeout", elapsed: &elapsed},
	})
	if err != nil {
		t.Fatalf("processBatch: %v", err)
	}

	// Assert: is_up и last_error обновлены корректно.
	var refreshed []models.MonitorURL
	if err := db.Order("id asc").Find(&refreshed).Error; err != nil {
		t.Fatalf("reload monitors: %v", err)
	}
	if len(refreshed) != 2 {
		t.Fatalf("reloaded %d monitors, want 2", len(refreshed))
	}
	if refreshed[0].IsUp == nil || !*refreshed[0].IsUp {
		t.Fatalf("monitor 0 is_up = %v, want true", refreshed[0].IsUp)
	}
	if refreshed[1].IsUp == nil || *refreshed[1].IsUp {
		t.Fatalf("monitor 1 is_up = %v, want false", refreshed[1].IsUp)
	}
	if refreshed[1].LastError != "timeout" {
		t.Fatalf("monitor 1 last_error = %q, want timeout", refreshed[1].LastError)
	}

	// Assert: два monitor_checks записаны.
	var checks int64
	if err := db.Model(&models.MonitorCheck{}).Count(&checks).Error; err != nil {
		t.Fatalf("count checks: %v", err)
	}
	if checks != 2 {
		t.Fatalf("checks = %d, want 2", checks)
	}

	// Assert: один open и один resolved incident.
	var openCount int64
	if err := db.Model(&models.Incident{}).Where("resolved_at IS NULL").Count(&openCount).Error; err != nil {
		t.Fatalf("count open incidents: %v", err)
	}
	if openCount != 1 {
		t.Fatalf("open incidents = %d, want 1", openCount)
	}

	var resolvedCount int64
	if err := db.Model(&models.Incident{}).Where("resolved_at IS NOT NULL").Count(&resolvedCount).Error; err != nil {
		t.Fatalf("count resolved incidents: %v", err)
	}
	if resolvedCount != 1 {
		t.Fatalf("resolved incidents = %d, want 1", resolvedCount)
	}
}

func TestCollapseBatchByMonitorKeepsLast(t *testing.T) {
	// Arrange: два результата для monitor ID=1 — второй должен победить.
	batch := []checkResult{
		{monitor: models.MonitorURL{ID: 1}, isUp: false, errMsg: "first"},
		{monitor: models.MonitorURL{ID: 2}, isUp: true},
		{monitor: models.MonitorURL{ID: 1}, isUp: true, errMsg: ""},
	}
	// Act: collapse оставляет последний результат на монитор.
	got := collapseBatchByMonitor(batch)
	// Assert: два уникальных monitor, monitor 1 — up.
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].monitor.ID != 1 || !got[0].isUp {
		t.Fatalf("first collapsed result = %+v, want monitor 1 up", got[0])
	}
	if got[1].monitor.ID != 2 {
		t.Fatalf("second collapsed result id = %d, want 2", got[1].monitor.ID)
	}
}

func TestNewUsesConfiguredCheckConcurrency(t *testing.T) {
	// Act + Assert: явный CheckConcurrency из config.
	cfg := &config.Config{CheckConcurrency: 7}
	w := New(nil, cfg)
	if w.checkConcurrency != 7 {
		t.Fatalf("checkConcurrency = %d, want 7", w.checkConcurrency)
	}

	// Act + Assert: 0 → DefaultCheckConcurrency.
	w = New(nil, &config.Config{CheckConcurrency: 0})
	if w.checkConcurrency != DefaultCheckConcurrency {
		t.Fatalf("checkConcurrency = %d, want default %d", w.checkConcurrency, DefaultCheckConcurrency)
	}
}

func TestEnqueueNotificationSkipsDisabledChannels(t *testing.T) {
	// Arrange: монитор без включённых каналов notify.
	w := New(nil, &config.Config{})
	// Act: enqueue не должен добавлять job.
	w.enqueueNotification(models.MonitorURL{ID: 1}, false, "down")
	// Assert: очередь пуста.
	if len(w.notifyJobs) != 0 {
		t.Fatalf("expected empty queue, got %d jobs", len(w.notifyJobs))
	}
}

func TestEnqueueNotificationDoesNotBlockWhenQueueFull(t *testing.T) {
	// Arrange: notifyJobs заполнена до notifyQueueSize.
	w := New(nil, &config.Config{})
	for i := range uint(notifyQueueSize) {
		w.notifyJobs <- notifyJob{monitor: models.MonitorURL{ID: i + 1}}
	}

	// Act: enqueue при полной очереди — drop, не block.
	start := time.Now()
	w.enqueueNotification(models.MonitorURL{ID: 999, NotifySMTP: true}, false, "timeout")
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("enqueue blocked for %v on full queue", elapsed)
	}
	// Assert: размер очереди не изменился.
	if len(w.notifyJobs) != notifyQueueSize {
		t.Fatalf("queue length = %d, want %d", len(w.notifyJobs), notifyQueueSize)
	}
}

func TestNotifyLoopDeliversQueuedJobs(t *testing.T) {
	// Arrange: worker с mock notifySender и запущенным notifyLoop.
	w := New(nil, &config.Config{CheckConcurrency: 1})
	delivered := make(chan notifyJob, 1)
	w.notifySender = func(monitor models.MonitorURL, isUp bool, errMsg string) {
		delivered <- notifyJob{monitor: monitor, isUp: isUp, errMsg: errMsg}
	}
	go w.notifyLoop()

	monitor := models.MonitorURL{
		ID:             42,
		URL:            "https://example.com",
		NotifyTelegram: true,
	}
	// Act: ставим job в очередь.
	w.enqueueNotification(monitor, false, "timeout")

	// Assert: sender получил job с ожидаемыми полями.
	select {
	case job := <-delivered:
		if job.monitor.ID != 42 || job.isUp || job.errMsg != "timeout" {
			t.Fatalf("unexpected job: %+v", job)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification delivery")
	}

	// Act: закрываем очередь — loop должен завершиться.
	close(w.notifyJobs)
	select {
	case <-w.notifyDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notifyLoop to exit")
	}
}

func TestEnqueueNotificationDoesNotWaitForSlowSender(t *testing.T) {
	// Arrange: sender блокирует первый job до close(blockFirst).
	w := New(nil, &config.Config{CheckConcurrency: 1})
	var calls atomic.Int32
	started := make(chan struct{}, 2)
	blockFirst := make(chan struct{})
	w.notifySender = func(models.MonitorURL, bool, string) {
		n := calls.Add(1)
		started <- struct{}{}
		if n == 1 {
			<-blockFirst
		}
	}
	go w.notifyLoop()

	monitor := models.MonitorURL{ID: 1, NotifyTelegram: true}
	w.enqueueNotification(monitor, false, "first")

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("sender did not start")
	}

	// Act: второй enqueue пока sender занят первым job.
	start := time.Now()
	w.enqueueNotification(monitor, true, "")
	// Assert: enqueue не ждёт завершения медленного sender.
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("enqueue waited %v for busy sender", elapsed)
	}

	close(blockFirst)
	close(w.notifyJobs)
	select {
	case <-w.notifyDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notifyLoop to exit")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("delivered %d jobs, want 2", got)
	}
}

// openWorkerTestDB открывает изолированную PostgreSQL БД для интеграционных тестов worker.
// t — активный тест для skip или fatal-ошибок.
func openWorkerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := os.Getenv("GO_UPTIME_TEST_DATABASE_NAME")
	if dbName == "" {
		// Без env — интеграционные тесты worker пропускаются, не падают.
		t.Skip("GO_UPTIME_TEST_DATABASE_NAME is not set")
	}

	cfg := config.DatabaseConfig{
		Host:     workerEnvOrDefault("GO_UPTIME_DATABASE_HOST", "localhost"),
		Port:     workerEnvOrDefault("GO_UPTIME_DATABASE_PORT", "5432"),
		User:     workerEnvOrDefault("GO_UPTIME_DATABASE_USER", "postgres"),
		Password: workerEnvOrDefault("GO_UPTIME_DATABASE_PASSWORD", "postgres"),
		DBName:   dbName + "_worker",
	}
	ensureWorkerTestDatabase(t, cfg)

	db, err := gorm.Open(postgres.Open(fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open worker test db: %v", err)
	}
	// Схема как в production — все таблицы, нужные claim/batch/incidents.
	if err := db.AutoMigrate(
		&models.User{},
		&models.MonitorURL{},
		&models.MonitorCheck{},
		&models.Incident{},
		&models.AppSetting{},
		&models.StatMinutely{},
		&models.StatHourly{},
		&models.StatDaily{},
	); err != nil {
		t.Fatalf("migrate worker test schema: %v", err)
	}
	return db
}

// ensureWorkerTestDatabase создаёт тестовую БД worker, если её ещё нет.
// t — активный тест; cfg содержит имя целевой БД и параметры подключения.
func ensureWorkerTestDatabase(t *testing.T, cfg config.DatabaseConfig) {
	t.Helper()

	// Подключаемся к postgres — CREATE DATABASE нельзя внутри транзакции целевой БД.
	adminDB, err := gorm.Open(postgres.Open(fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password,
	)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres admin db: %v", err)
	}
	sqlDB, err := adminDB.DB()
	if err != nil {
		t.Fatalf("postgres admin handle: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	var exists int64
	if err := adminDB.Raw("SELECT COUNT(1) FROM pg_database WHERE datname = ?", cfg.DBName).Scan(&exists).Error; err != nil {
		t.Fatalf("lookup worker test database: %v", err)
	}
	if exists > 0 {
		return
	}
	if err := adminDB.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, cfg.DBName)).Error; err != nil {
		t.Fatalf("create worker test database: %v", err)
	}
}

// resetWorkerTestTables очищает таблицы, используемые интеграционными тестами worker.
// t — активный тест; db — подключение к тестовой БД worker.
func resetWorkerTestTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	// CASCADE сбрасывает FK-зависимости между таблицами worker-тестов.
	for _, table := range []string{"stat_minutely", "stat_hourly", "stat_daily", "monitor_checks", "incidents", "monitor_urls", "app_settings"} {
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)).Error; err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

// workerEnvOrDefault возвращает значение переменной окружения или fallback.
// key — имя переменной окружения; fallback используется, когда значение пустое.
func workerEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
