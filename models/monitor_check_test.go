package models

import (
	"testing"
	"time"
)

func TestCountHeartbeatsByMinute(t *testing.T) {
	// Arrange: монитор и checks в текущей, более старой минуте и вне окна агрегации.
	db := openTestDB(t)
	resetUptimeStatTables(t, db)

	now := time.Date(2026, 7, 25, 12, 0, 30, 0, time.UTC)
	monitor := MonitorURL{Name: "HB", URL: "https://hb.example", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now}
	if err := db.Create(&monitor).Error; err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	checks := []MonitorCheck{
		{MonitorURLID: monitor.ID, CheckedAt: now.Truncate(time.Minute), IsUp: true},
		{MonitorURLID: monitor.ID, CheckedAt: now.Truncate(time.Minute).Add(10 * time.Second), IsUp: false},
		{MonitorURLID: monitor.ID, CheckedAt: now.Truncate(time.Minute).Add(-3 * time.Minute), IsUp: true},
		{MonitorURLID: monitor.ID, CheckedAt: now.Add(-2 * time.Hour), IsUp: true}, // вне окна
	}
	for i := range checks {
		if err := db.Create(&checks[i]).Error; err != nil {
			t.Fatalf("create check: %v", err)
		}
	}

	// Act: агрегируем heartbeats по минутным bucket'ам.
	got, err := CountHeartbeatsByMinute(db, now)
	if err != nil {
		t.Fatalf("CountHeartbeatsByMinute: %v", err)
	}
	// Assert: в окне только две минуты с данными.
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}

	byUnix := make(map[int64]HeartbeatMinuteCount, len(got))
	for _, row := range got {
		byUnix[row.BucketAt.Unix()] = row
	}

	// Assert: текущая минута — 1 success, 1 failed.
	current := byUnix[now.Truncate(time.Minute).Unix()]
	if current.Success != 1 || current.Failed != 1 {
		t.Fatalf("current minute = %+v, want success 1 failed 1", current)
	}
	// Assert: минута −3 — только success.
	older := byUnix[now.Truncate(time.Minute).Add(-3*time.Minute).Unix()]
	if older.Success != 1 || older.Failed != 0 {
		t.Fatalf("older minute = %+v, want success 1 failed 0", older)
	}
}

func TestHeartbeatMinuteCountTotal(t *testing.T) {
	// Act: Total() суммирует success и failed.
	got := HeartbeatMinuteCount{Success: 2, Failed: 3}.Total()
	// Assert: сумма равна 5.
	if got != 5 {
		t.Fatalf("Total() = %d, want 5", got)
	}
}

func TestPruneMonitorChecksKeepsMostRecentPerMonitor(t *testing.T) {
	// Arrange: maxMonitorChecksPerMonitor+2 checks с монотонным CheckedAt.
	db := openTestDB(t)
	resetUptimeStatTables(t, db)

	monitor := MonitorURL{Name: "Prune", URL: "https://prune-checks.example"}
	if err := db.Create(&monitor).Error; err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	// Усечение до микросекунд: PostgreSQL timestamptz не хранит наносекунды.
	start := time.Now().Add(-monitorCheckRetention - time.Hour).Truncate(time.Microsecond)
	checks := make([]MonitorCheck, 0, maxMonitorChecksPerMonitor+2)
	for i := 0; i < maxMonitorChecksPerMonitor+2; i++ {
		checks = append(checks, MonitorCheck{
			MonitorURLID: monitor.ID,
			CheckedAt:    start.Add(time.Duration(i) * time.Second),
			IsUp:         true,
		})
	}
	if err := db.Create(&checks).Error; err != nil {
		t.Fatalf("create checks: %v", err)
	}

	// Act: prune удаляет самые старые записи сверх лимита.
	if err := PruneMonitorChecks(db); err != nil {
		t.Fatalf("PruneMonitorChecks: %v", err)
	}

	// Assert: остаётся ровно maxMonitorChecksPerMonitor записей.
	var count int64
	if err := db.Model(&MonitorCheck{}).Where("monitor_url_id = ?", monitor.ID).Count(&count).Error; err != nil {
		t.Fatalf("count checks: %v", err)
	}
	if count != maxMonitorChecksPerMonitor {
		t.Fatalf("remaining checks = %d, want %d", count, maxMonitorChecksPerMonitor)
	}

	// Assert: самая старая сохранённая — третья по счёту (первые две удалены).
	var oldest MonitorCheck
	if err := db.Where("monitor_url_id = ?", monitor.ID).Order("checked_at asc").First(&oldest).Error; err != nil {
		t.Fatalf("load oldest check: %v", err)
	}
	wantOldest := start.Add(2 * time.Second)
	if !oldest.CheckedAt.Equal(wantOldest) {
		t.Fatalf("oldest remaining check = %v, want %v", oldest.CheckedAt, wantOldest)
	}
}
