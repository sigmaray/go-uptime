package handlers

import (
	"testing"
	"time"

	"go-uptime/models"
	"go-uptime/worker"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "seconds", in: 45 * time.Second, want: "45s"},
		{name: "minutes", in: 2*time.Minute + 5*time.Second, want: "2m 5s"},
		{name: "hours", in: 3*time.Hour + 12*time.Minute, want: "3h 12m"},
		{name: "negative treated as zero", in: -time.Second, want: "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDuration(tt.in); got != tt.want {
				t.Fatalf("formatDuration() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildOverdueMonitorView(t *testing.T) {
	if got := buildOverdueMonitorView(nil, 0); got.HasOverdue {
		t.Fatal("expected empty view for nil monitor")
	}

	last := time.Date(2026, 7, 23, 6, 0, 0, 0, time.UTC)
	monitor := &models.MonitorURL{
		ID:            7,
		Name:          "Example",
		URL:           "https://example.com",
		LastCheckedAt: &last,
	}
	got := buildOverdueMonitorView(monitor, 90*time.Second)
	if !got.HasOverdue {
		t.Fatal("expected HasOverdue")
	}
	if got.Name != "Example" || got.ID != 7 {
		t.Fatalf("unexpected view: %+v", got)
	}
	if got.OverdueBy != "1m 30s" {
		t.Fatalf("OverdueBy = %q, want 1m 30s", got.OverdueBy)
	}
	if got.LastChecked != "2026-07-23 06:00:00" {
		t.Fatalf("LastChecked = %q", got.LastChecked)
	}
}

func TestComputeMonitorBacklog(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	interval := 60
	recent := now.Add(-30 * time.Second)
	old := now.Add(-10 * time.Minute)
	older := now.Add(-30 * time.Minute)

	monitors := []models.MonitorURL{
		{ID: 1, Name: "Fresh", URL: "https://fresh.example", LastCheckedAt: &recent, CheckIntervalSeconds: &interval},
		{ID: 2, Name: "Never", URL: "https://never.example", CheckIntervalSeconds: &interval},
		{ID: 3, Name: "Overdue", URL: "https://overdue.example", LastCheckedAt: &old, CheckIntervalSeconds: &interval},
		{ID: 4, Name: "Oldest", URL: "https://oldest.example", LastCheckedAt: &older, CheckIntervalSeconds: &interval},
	}

	got := computeMonitorBacklog(monitors, 60, now)
	if got.Total != 4 {
		t.Fatalf("Total = %d, want 4", got.Total)
	}
	if got.DueWaiting != 3 {
		t.Fatalf("DueWaiting = %d, want 3", got.DueWaiting)
	}
	if got.NeverChecked != 1 {
		t.Fatalf("NeverChecked = %d, want 1", got.NeverChecked)
	}
	if !got.MostOverdue.HasOverdue || got.MostOverdue.Name != "Oldest" {
		t.Fatalf("MostOverdue = %+v, want Oldest", got.MostOverdue)
	}
	if got.MostOverdue.OverdueBy != "29m 0s" {
		t.Fatalf("OverdueBy = %q, want 29m 0s", got.MostOverdue.OverdueBy)
	}
}

func TestPercentOf(t *testing.T) {
	tests := []struct {
		name  string
		value int
		max   int
		want  int
	}{
		{name: "half", value: 25, max: 50, want: 50},
		{name: "zero max", value: 5, max: 0, want: 0},
		{name: "zero value", value: 0, max: 10, want: 0},
		{name: "over capacity", value: 12, max: 10, want: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percentOf(tt.value, tt.max); got != tt.want {
				t.Fatalf("percentOf(%d, %d) = %d, want %d", tt.value, tt.max, got, tt.want)
			}
		})
	}
}

func TestBuildUtilizationGauges(t *testing.T) {
	gauges := buildUtilizationGauges(worker.Stats{
		DueThisWave:    10,
		InFlight:       4,
		WaitingForSlot: 6,
		MaxConcurrency: 8,
		NotifyQueued:   2,
		NotifyCapacity: 256,
	})
	if len(gauges) != 3 {
		t.Fatalf("len = %d, want 3", len(gauges))
	}
	if gauges[0].Label != "Check slots" || gauges[0].Percent != 50 || gauges[0].Detail != "4 / 8" {
		t.Fatalf("check slots gauge = %+v", gauges[0])
	}
	if gauges[1].Label != "Waiting for slot" || gauges[1].Percent != 60 || gauges[1].Detail != "6 / 10" {
		t.Fatalf("waiting gauge = %+v", gauges[1])
	}
	if gauges[2].Label != "Notify queue" || gauges[2].Percent != 0 || gauges[2].Detail != "2 / 256" {
		t.Fatalf("notify gauge = %+v", gauges[2])
	}
}

func TestBuildFleetComposition(t *testing.T) {
	now := time.Now()
	up := true
	down := false
	monitors := []models.MonitorURL{
		{LastCheckedAt: &now, IsUp: &up},
		{LastCheckedAt: &now, IsUp: &up},
		{LastCheckedAt: &now, IsUp: &down},
		{},
	}
	got := buildFleetComposition(monitors)
	if got.Total != 4 {
		t.Fatalf("Total = %d, want 4", got.Total)
	}
	if got.Segments[0].Count != 2 || got.Segments[0].Modifier != "up" {
		t.Fatalf("up segment = %+v", got.Segments[0])
	}
	if got.Segments[1].Count != 1 || got.Segments[1].Modifier != "down" {
		t.Fatalf("down segment = %+v", got.Segments[1])
	}
	if got.Segments[2].Count != 1 || got.Segments[2].Modifier != "unknown" {
		t.Fatalf("unknown segment = %+v", got.Segments[2])
	}
	if got.Segments[0].Percent != 50 {
		t.Fatalf("up percent = %d, want 50", got.Segments[0].Percent)
	}
}

func TestBuildBacklogComposition(t *testing.T) {
	got := buildBacklogComposition(monitorBacklog{
		Total:        5,
		DueWaiting:   3,
		NeverChecked: 1,
	})
	if got.Total != 5 {
		t.Fatalf("Total = %d, want 5", got.Total)
	}
	if got.Segments[0].Label != "Due" || got.Segments[0].Count != 2 {
		t.Fatalf("due segment = %+v", got.Segments[0])
	}
	if got.Segments[1].Label != "Never checked" || got.Segments[1].Count != 1 {
		t.Fatalf("never segment = %+v", got.Segments[1])
	}
	if got.Segments[2].Label != "On schedule" || got.Segments[2].Count != 2 {
		t.Fatalf("schedule segment = %+v", got.Segments[2])
	}
}

func TestBuildHeartbeatHourChart(t *testing.T) {
	now := time.Date(2026, 7, 25, 15, 30, 40, 0, time.UTC)
	current := now.Truncate(time.Minute)
	counts := []models.HeartbeatMinuteCount{
		{BucketAt: current.Add(-2 * time.Minute), Success: 3, Failed: 1},
		{BucketAt: current, Success: 0, Failed: 2},
	}

	got := buildHeartbeatHourChart(counts, now)
	if len(got.Bars) != models.HeartbeatHourMinutes {
		t.Fatalf("len(Bars) = %d, want %d", len(got.Bars), models.HeartbeatHourMinutes)
	}
	if got.MaxPerMinute != 4 {
		t.Fatalf("MaxPerMinute = %d, want 4", got.MaxPerMinute)
	}
	if got.TotalSuccess != 3 || got.TotalFailed != 3 || got.Total != 6 {
		t.Fatalf("totals = success %d failed %d total %d", got.TotalSuccess, got.TotalFailed, got.Total)
	}

	busy := got.Bars[models.HeartbeatHourMinutes-3]
	if busy.Success != 3 || busy.Failed != 1 || busy.Total != 4 {
		t.Fatalf("busy bar = %+v", busy)
	}
	if busy.HeightPercent != 100 {
		t.Fatalf("busy HeightPercent = %d, want 100", busy.HeightPercent)
	}
	if busy.SuccessPercent != 75 || busy.FailedPercent != 25 {
		t.Fatalf("busy shares = success %d failed %d", busy.SuccessPercent, busy.FailedPercent)
	}

	latest := got.Bars[models.HeartbeatHourMinutes-1]
	if latest.Success != 0 || latest.Failed != 2 || latest.HeightPercent != 50 {
		t.Fatalf("latest bar = %+v", latest)
	}
	if latest.SuccessPercent != 0 || latest.FailedPercent != 100 {
		t.Fatalf("latest shares = success %d failed %d", latest.SuccessPercent, latest.FailedPercent)
	}

	empty := got.Bars[0]
	if empty.Total != 0 || empty.HeightPercent != 0 {
		t.Fatalf("empty bar = %+v", empty)
	}
}

func TestApplicationTableModels(t *testing.T) {
	want := []string{
		"app_settings",
		"incidents",
		"monitor_checks",
		"monitor_urls",
		"stat_daily",
		"stat_hourly",
		"stat_minutely",
		"users",
	}
	if len(applicationTableModels) != len(want) {
		t.Fatalf("len = %d, want %d", len(applicationTableModels), len(want))
	}
	for i, entry := range applicationTableModels {
		if entry.name != want[i] {
			t.Fatalf("applicationTableModels[%d].name = %q, want %q", i, entry.name, want[i])
		}
		if entry.model == nil {
			t.Fatalf("applicationTableModels[%d].model is nil", i)
		}
	}
}
