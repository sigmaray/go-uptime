package models

import (
	"testing"
	"time"
)

func TestPruneIncidentsAppliesRetentionAndPerMonitorLimit(t *testing.T) {
	// Arrange: чистая БД, один монитор, смесь старых/новых resolved и один open incident.
	db := openTestDB(t)
	resetUptimeStatTables(t, db)

	// Усечение до микросекунд: PostgreSQL timestamptz не хранит наносекунды.
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

	// Act: retention 90 дней и лимит 2 resolved на монитор.
	if err := PruneIncidents(db, 90, 2); err != nil {
		t.Fatalf("PruneIncidents: %v", err)
	}

	// Assert: остаются только 2 самых новых resolved (recent 2 и recent 3).
	var resolvedCount int64
	if err := db.Model(&Incident{}).
		Where("monitor_url_id = ? AND resolved_at IS NOT NULL", monitor.ID).
		Count(&resolvedCount).Error; err != nil {
		t.Fatalf("count resolved incidents: %v", err)
	}
	if resolvedCount != 2 {
		t.Fatalf("resolved incident count = %d, want 2", resolvedCount)
	}

	// Assert: open incident не трогается.
	var openCount int64
	if err := db.Model(&Incident{}).
		Where("monitor_url_id = ? AND resolved_at IS NULL", monitor.ID).
		Count(&openCount).Error; err != nil {
		t.Fatalf("count open incidents: %v", err)
	}
	if openCount != 1 {
		t.Fatalf("open incident count = %d, want 1", openCount)
	}

	// Assert: сохранённые resolved — именно две последние по resolved_at.
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
