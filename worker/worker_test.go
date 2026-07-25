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
	var nilWorker *MonitorWorker
	if nilWorker.Running() {
		t.Fatal("nil worker should not be running")
	}

	w := New(nil, &config.Config{CheckConcurrency: 1})
	if w.Running() {
		t.Fatal("expected worker not running before Start")
	}

	// Simulate an active worker without launching DB-backed loops.
	w.started.Store(true)
	if !w.Running() {
		t.Fatal("expected worker running while started and stop channel open")
	}

	close(w.stop)
	if w.Running() {
		t.Fatal("expected worker not running after stop channel closed")
	}
}

func TestMonitorWorkerPauseSkipsDueMonitors(t *testing.T) {
	var nilWorker *MonitorWorker
	nilWorker.Pause()
	nilWorker.Resume()
	if nilWorker.Paused() {
		t.Fatal("nil worker should not report paused")
	}

	w := New(nil, &config.Config{CheckConcurrency: 1})
	w.Pause()
	if !w.Paused() {
		t.Fatal("expected worker paused after Pause")
	}
	// Must not touch the nil DB when paused.
	w.runDueMonitors()

	w.Resume()
	if w.Paused() {
		t.Fatal("expected worker not paused after Resume")
	}
}

func TestSetBrowserLikeHeaders(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	urlcheck.SetBrowserLikeHeaders(req)

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
	interval := time.Minute
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	t.Run("never checked", func(t *testing.T) {
		if !IsMonitorDue(nil, interval, now) {
			t.Fatal("expected first check to be due")
		}
	})

	t.Run("interval not elapsed", func(t *testing.T) {
		last := now.Add(-30 * time.Second)
		if IsMonitorDue(&last, interval, now) {
			t.Fatal("expected monitor to be skipped")
		}
	})

	t.Run("interval elapsed", func(t *testing.T) {
		last := now.Add(-time.Minute)
		if !IsMonitorDue(&last, interval, now) {
			t.Fatal("expected monitor to be due")
		}
	})
}

func TestRunChecksConcurrentlyRespectsLimit(t *testing.T) {
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
	}, nil)
	elapsed := time.Since(start)

	if got := int(checked.Load()); got != monitorCount {
		t.Fatalf("checked %d monitors, want %d", got, monitorCount)
	}
	if got := int(peak.Load()); got > maxConcurrent {
		t.Fatalf("peak concurrency %d exceeds limit %d", got, maxConcurrent)
	}
	if len(seen) != monitorCount {
		t.Fatalf("saw %d unique monitors, want %d", len(seen), monitorCount)
	}

	// Serial would take monitorCount * checkDuration; with concurrency 3 it should be much faster.
	serialFloor := time.Duration(monitorCount) * checkDuration
	if elapsed >= serialFloor {
		t.Fatalf("elapsed %v looks serial (serial floor %v)", elapsed, serialFloor)
	}
}

func TestRunChecksConcurrentlyEmptyAndInvalidLimit(t *testing.T) {
	runChecksConcurrently(nil, 5, func(models.MonitorURL) {
		t.Fatal("checkFn must not run for empty list")
	}, nil)

	var ran atomic.Int32
	runChecksConcurrently([]models.MonitorURL{{ID: 1}}, 0, func(models.MonitorURL) {
		ran.Add(1)
	}, nil)
	if ran.Load() != 1 {
		t.Fatalf("expected single check with invalid limit, got %d", ran.Load())
	}
}

func TestStatsTracksWaveProgress(t *testing.T) {
	w := New(nil, &config.Config{CheckConcurrency: 2})

	release := make(chan struct{})
	started := make(chan struct{}, 4)
	monitors := []models.MonitorURL{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runChecksConcurrently(monitors, w.checkConcurrency, func(models.MonitorURL) {
			started <- struct{}{}
			<-release
		}, &checkWaveCounters{
			due:      &w.waveDue,
			started:  &w.waveStarted,
			inFlight: &w.inFlight,
		})
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for in-flight checks")
		}
	}

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

	close(release)
	wg.Wait()

	stats = w.Stats()
	if stats.DueThisWave != 0 || stats.InFlight != 0 || stats.WaitingForSlot != 0 {
		t.Fatalf("expected idle stats after wave, got %+v", stats)
	}
}

func TestStatsReportsNotifyQueueDepth(t *testing.T) {
	w := New(nil, &config.Config{})
	w.enqueueNotification(models.MonitorURL{ID: 1, NotifySMTP: true}, false, "down")
	stats := w.Stats()
	if stats.NotifyQueued != 1 {
		t.Fatalf("NotifyQueued = %d, want 1", stats.NotifyQueued)
	}
}

func TestClaimDueMonitorsPostponesScheduleBeforeChecks(t *testing.T) {
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
	claimed, err := w.claimDueMonitors(now)
	if err != nil {
		t.Fatalf("claimDueMonitors: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %d monitors, want 2", len(claimed))
	}

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
	}

	claimedAgain, err := w.claimDueMonitors(now.Add(time.Second))
	if err != nil {
		t.Fatalf("second claimDueMonitors: %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("second claim returned %d monitors, want 0", len(claimedAgain))
	}
}

func TestProcessBatchReturnsErrorForMissingMonitor(t *testing.T) {
	db := openWorkerTestDB(t)
	resetWorkerTestTables(t, db)

	w := New(db, &config.Config{CheckConcurrency: 1})
	err := w.processBatch([]checkResult{
		{
			monitor: models.MonitorURL{
				ID:  999,
				URL: "https://missing.example.com",
			},
			isUp: true,
		},
	})
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

func TestNewUsesConfiguredCheckConcurrency(t *testing.T) {
	cfg := &config.Config{CheckConcurrency: 7}
	w := New(nil, cfg)
	if w.checkConcurrency != 7 {
		t.Fatalf("checkConcurrency = %d, want 7", w.checkConcurrency)
	}

	w = New(nil, &config.Config{CheckConcurrency: 0})
	if w.checkConcurrency != DefaultCheckConcurrency {
		t.Fatalf("checkConcurrency = %d, want default %d", w.checkConcurrency, DefaultCheckConcurrency)
	}
}

func TestEnqueueNotificationSkipsDisabledChannels(t *testing.T) {
	w := New(nil, &config.Config{})
	w.enqueueNotification(models.MonitorURL{ID: 1}, false, "down")
	if len(w.notifyJobs) != 0 {
		t.Fatalf("expected empty queue, got %d jobs", len(w.notifyJobs))
	}
}

func TestEnqueueNotificationDoesNotBlockWhenQueueFull(t *testing.T) {
	w := New(nil, &config.Config{})
	for i := range uint(notifyQueueSize) {
		w.notifyJobs <- notifyJob{monitor: models.MonitorURL{ID: i + 1}}
	}

	start := time.Now()
	w.enqueueNotification(models.MonitorURL{ID: 999, NotifySMTP: true}, false, "timeout")
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("enqueue blocked for %v on full queue", elapsed)
	}
	if len(w.notifyJobs) != notifyQueueSize {
		t.Fatalf("queue length = %d, want %d", len(w.notifyJobs), notifyQueueSize)
	}
}

func TestNotifyLoopDeliversQueuedJobs(t *testing.T) {
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
	w.enqueueNotification(monitor, false, "timeout")

	select {
	case job := <-delivered:
		if job.monitor.ID != 42 || job.isUp || job.errMsg != "timeout" {
			t.Fatalf("unexpected job: %+v", job)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification delivery")
	}

	close(w.notifyJobs)
	select {
	case <-w.notifyDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notifyLoop to exit")
	}
}

func TestEnqueueNotificationDoesNotWaitForSlowSender(t *testing.T) {
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

	start := time.Now()
	w.enqueueNotification(monitor, true, "")
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

// openWorkerTestDB opens the isolated PostgreSQL database used by worker integration tests.
// t is the active test used for skipping or fatal error reporting.
func openWorkerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := os.Getenv("GO_UPTIME_TEST_DATABASE_NAME")
	if dbName == "" {
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

// ensureWorkerTestDatabase creates the worker test database when it is missing.
// t is the active test; cfg contains the target database name and connection settings.
func ensureWorkerTestDatabase(t *testing.T, cfg config.DatabaseConfig) {
	t.Helper()

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

// resetWorkerTestTables truncates tables touched by worker integration tests.
// t is the active test; db is the worker test database connection.
func resetWorkerTestTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{"stat_minutely", "stat_hourly", "stat_daily", "monitor_checks", "incidents", "monitor_urls", "app_settings"} {
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)).Error; err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

// workerEnvOrDefault returns an environment variable value or a fallback.
// key is the environment variable name; fallback is used when the value is empty.
func workerEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
