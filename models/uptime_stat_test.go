package models

import (
	"testing"
	"time"
)

func TestUptimeSummaryPercent(t *testing.T) {
	tests := []struct {
		name    string
		summary UptimeSummary
		want    float64
	}{
		{
			name:    "no data",
			summary: UptimeSummary{},
			want:    -1,
		},
		{
			name:    "full uptime",
			summary: UptimeSummary{UpSeconds: 3600, TotalSeconds: 3600},
			want:    100,
		},
		{
			name:    "half uptime",
			summary: UptimeSummary{UpSeconds: 1800, TotalSeconds: 3600},
			want:    50,
		},
		{
			name:    "rounded uptime",
			summary: UptimeSummary{UpSeconds: 1, TotalSeconds: 3},
			want:    33.33,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.Percent()
			if got != tt.want {
				t.Fatalf("Percent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUptimePeriodEligible(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	period := 24 * time.Hour

	tests := []struct {
		name      string
		createdAt time.Time
		want      bool
	}{
		{
			name:      "monitor younger than period",
			createdAt: now.Add(-2 * time.Hour),
			want:      false,
		},
		{
			name:      "monitor older than period",
			createdAt: now.Add(-25 * time.Hour),
			want:      true,
		},
		{
			name:      "monitor exactly at period boundary",
			createdAt: now.Add(-period),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uptimePeriodEligible(tt.createdAt, now, period)
			if got != tt.want {
				t.Fatalf("uptimePeriodEligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEffectiveSince(t *testing.T) {
	since := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 4, 11, 0, 0, 0, time.UTC)

	got := effectiveSince(createdAt, since)
	if !got.Equal(createdAt) {
		t.Fatalf("effectiveSince() = %v, want %v", got, createdAt)
	}

	older := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)
	got = effectiveSince(older, since)
	if !got.Equal(since) {
		t.Fatalf("effectiveSince() = %v, want %v", got, since)
	}
}

func TestBuildUptimeHistoryBarsSkipsPreCreationMinutes(t *testing.T) {
	if testing.Short() {
		t.Skip("requires database")
	}

	db := openTestDB(t)
	resetUptimeStatTables(t, db)

	now := time.Date(2026, 7, 4, 12, 30, 0, 0, time.UTC)
	createdAt := now.Add(-10 * time.Minute)

	monitor := MonitorURL{Name: "history", URL: "https://history.example.com", CreatedAt: createdAt}
	if err := db.Create(&monitor).Error; err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	for i := 0; i < 5; i++ {
		bucketAt := now.Add(-time.Duration(4-i) * time.Minute).Truncate(time.Minute)
		row := StatMinutely{
			MonitorURLID: monitor.ID,
			BucketAt:     bucketAt,
			UpSeconds:    60,
			TotalSeconds: 60,
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("create bucket: %v", err)
		}
	}

	bars, err := BuildUptimeHistoryBars(db, monitor.ID, createdAt, now)
	if err != nil {
		t.Fatalf("BuildUptimeHistoryBars: %v", err)
	}
	if len(bars) != uptimeHistoryMinutes {
		t.Fatalf("bar count = %d, want %d", len(bars), uptimeHistoryMinutes)
	}

	noDataBeforeCreation := 0
	upAfterCreation := 0
	for _, bar := range bars {
		if bar.BucketAt.Before(createdAt.Truncate(time.Minute)) {
			if bar.State != UptimeBarNoData {
				t.Fatalf("bucket %v before creation has state %q, want nodata", bar.BucketAt, bar.State)
			}
			noDataBeforeCreation++
		}
		if !bar.BucketAt.Before(createdAt.Truncate(time.Minute)) && bar.State == UptimeBarUp {
			upAfterCreation++
		}
	}
	if noDataBeforeCreation == 0 {
		t.Fatal("expected some no-data bars before monitor creation")
	}
	if upAfterCreation != 5 {
		t.Fatalf("up bars after creation = %d, want 5", upAfterCreation)
	}
}

func TestTruncateToBucket(t *testing.T) {
	ts := time.Date(2026, 7, 4, 15, 47, 33, 500, time.UTC)

	minute := truncateToBucket(ts, uptimeGranularityMinutely)
	wantMinute := time.Date(2026, 7, 4, 15, 47, 0, 0, time.UTC)
	if !minute.Equal(wantMinute) {
		t.Fatalf("minute bucket = %v, want %v", minute, wantMinute)
	}

	hour := truncateToBucket(ts, uptimeGranularityHourly)
	wantHour := time.Date(2026, 7, 4, 15, 0, 0, 0, time.UTC)
	if !hour.Equal(wantHour) {
		t.Fatalf("hour bucket = %v, want %v", hour, wantHour)
	}

	day := truncateToBucket(ts, uptimeGranularityDaily)
	wantDay := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	if !day.Equal(wantDay) {
		t.Fatalf("day bucket = %v, want %v", day, wantDay)
	}
}

func TestAddDurationToGranularitySplitsAcrossBuckets(t *testing.T) {
	if testing.Short() {
		t.Skip("requires database")
	}

	db := openTestDB(t)
	resetUptimeStatTables(t, db)

	monitor := MonitorURL{Name: "test", URL: "https://example.com"}
	if err := db.Create(&monitor).Error; err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	start := time.Date(2026, 7, 4, 10, 59, 30, 0, time.UTC)
	end := time.Date(2026, 7, 4, 11, 0, 30, 0, time.UTC)

	if err := addDurationToGranularity(db, monitor.ID, start, end, true, uptimeGranularityMinutely); err != nil {
		t.Fatalf("addDurationToGranularity: %v", err)
	}

	var buckets []StatMinutely
	if err := db.Where("monitor_url_id = ?", monitor.ID).Order("bucket_at asc").Find(&buckets).Error; err != nil {
		t.Fatalf("load buckets: %v", err)
	}
	if len(buckets) != 2 {
		t.Fatalf("bucket count = %d, want 2", len(buckets))
	}
	if buckets[0].TotalSeconds != 30 || buckets[1].TotalSeconds != 30 {
		t.Fatalf("bucket seconds = [%d, %d], want [30, 30]", buckets[0].TotalSeconds, buckets[1].TotalSeconds)
	}
}

func TestBackfillUptimeStats(t *testing.T) {
	if testing.Short() {
		t.Skip("requires database")
	}

	db := openTestDB(t)
	resetUptimeStatTables(t, db)

	monitor := MonitorURL{Name: "backfill", URL: "https://backfill.example.com"}
	if err := db.Create(&monitor).Error; err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	base := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	checks := []MonitorCheck{
		{MonitorURLID: monitor.ID, CheckedAt: base, IsUp: true},
		{MonitorURLID: monitor.ID, CheckedAt: base.Add(time.Minute), IsUp: true},
		{MonitorURLID: monitor.ID, CheckedAt: base.Add(2 * time.Minute), IsUp: false},
	}
	if err := db.Create(&checks).Error; err != nil {
		t.Fatalf("create checks: %v", err)
	}

	if err := BackfillUptimeStats(db); err != nil {
		t.Fatalf("BackfillUptimeStats: %v", err)
	}

	uptime, err := LoadMonitorUptime(db, monitor.ID, base.Add(-25*time.Hour), base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("LoadMonitorUptime: %v", err)
	}
	if got := uptime.Hours24.Percent(); got != 50 {
		t.Fatalf("24h uptime = %v, want 50", got)
	}
}
