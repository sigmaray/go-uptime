package models

import (
	"testing"
	"time"
)

func TestPruneIncidentsAppliesRetentionAndPerMonitorLimit(t *testing.T) {
	db := openTestDB(t)
	resetUptimeStatTables(t, db)

	// Truncate to microseconds: PostgreSQL timestamptz does not store nanoseconds.
	now := time.Now().Truncate(time.Microsecond)
	monitor := MonitorURL{Name: "Incident prune", URL: "https://incident-prune.example"}
	if err := db.Create(&monitor).Error; err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	oldResolved := now.AddDate(0, 0, -100)
	recentResolved := []time.Time{
		now.AddDate(0, 0, -3),
		now.AddDate(0, 0, -2),
		now.AddDate(0, 0, -1),
	}
	incidents := []Incident{
		{MonitorURLID: monitor.ID, StartedAt: oldResolved.Add(-time.Hour), ResolvedAt: &oldResolved, ErrorMessage: "old"},
		{MonitorURLID: monitor.ID, StartedAt: recentResolved[0].Add(-time.Hour), ResolvedAt: &recentResolved[0], ErrorMessage: "recent 1"},
		{MonitorURLID: monitor.ID, StartedAt: recentResolved[1].Add(-time.Hour), ResolvedAt: &recentResolved[1], ErrorMessage: "recent 2"},
		{MonitorURLID: monitor.ID, StartedAt: recentResolved[2].Add(-time.Hour), ResolvedAt: &recentResolved[2], ErrorMessage: "recent 3"},
		{MonitorURLID: monitor.ID, StartedAt: now.Add(-time.Hour), ErrorMessage: "open"},
	}
	if err := db.Create(&incidents).Error; err != nil {
		t.Fatalf("create incidents: %v", err)
	}

	if err := PruneIncidents(db, 90, 2); err != nil {
		t.Fatalf("PruneIncidents: %v", err)
	}

	var resolvedCount int64
	if err := db.Model(&Incident{}).
		Where("monitor_url_id = ? AND resolved_at IS NOT NULL", monitor.ID).
		Count(&resolvedCount).Error; err != nil {
		t.Fatalf("count resolved incidents: %v", err)
	}
	if resolvedCount != 2 {
		t.Fatalf("resolved incident count = %d, want 2", resolvedCount)
	}

	var openCount int64
	if err := db.Model(&Incident{}).
		Where("monitor_url_id = ? AND resolved_at IS NULL", monitor.ID).
		Count(&openCount).Error; err != nil {
		t.Fatalf("count open incidents: %v", err)
	}
	if openCount != 1 {
		t.Fatalf("open incident count = %d, want 1", openCount)
	}

	var remaining []Incident
	if err := db.Where("monitor_url_id = ? AND resolved_at IS NOT NULL", monitor.ID).
		Order("resolved_at asc").
		Find(&remaining).Error; err != nil {
		t.Fatalf("load remaining resolved incidents: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining resolved incidents = %d, want 2", len(remaining))
	}
	if !remaining[0].ResolvedAt.Equal(recentResolved[1]) || !remaining[1].ResolvedAt.Equal(recentResolved[2]) {
		t.Fatalf("remaining resolved incidents = %+v, want newest two", remaining)
	}
}
