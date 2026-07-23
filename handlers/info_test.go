package handlers

import (
	"testing"
	"time"

	"go-uptime/models"
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
