package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	// monitorCheckRetention is the minimum age before a check may be pruned.
	monitorCheckRetention = 24 * time.Hour
	// maxMonitorChecksPerMonitor is how many recent checks are always kept per monitor.
	maxMonitorChecksPerMonitor = 200
)

const (
	// MaxRecentHeartbeatsList is how many heartbeats the global list page loads.
	MaxRecentHeartbeatsList = 500
	// MaxMonitorDetailHeartbeats is how many heartbeats a monitor detail page loads.
	MaxMonitorDetailHeartbeats = 200
)

// MonitorCheck stores the result of a single HTTP availability check.
type MonitorCheck struct {
	ID             uint       `gorm:"primaryKey"`
	MonitorURLID   uint       `gorm:"not null;index"`
	MonitorURL     MonitorURL `gorm:"foreignKey:MonitorURLID"`
	CheckedAt      time.Time
	IsUp           bool `gorm:"not null"`
	ResponseTimeMs *int
}

// RecordMonitorCheck persists one check result for uptime history and updates aggregated uptime stats.
// db is the database handle used for persistence.
// monitorID is the monitor_urls.id that was checked.
// checkedAt is when the check finished.
// isUp is whether the target responded successfully.
// responseTimeMs is the optional measured latency in milliseconds.
func RecordMonitorCheck(db *gorm.DB, monitorID uint, checkedAt time.Time, isUp bool, responseTimeMs *int) error {
	if err := UpdateUptimeStats(db, monitorID, checkedAt, isUp); err != nil {
		return err
	}

	check := MonitorCheck{
		MonitorURLID:   monitorID,
		CheckedAt:      checkedAt,
		IsUp:           isUp,
		ResponseTimeMs: responseTimeMs,
	}
	return db.Create(&check).Error
}

// CountMonitorChecks returns the total number of heartbeats across all monitors.
func CountMonitorChecks(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&MonitorCheck{}).Count(&count).Error
	return count, err
}

// LoadAllMonitorChecksPage loads one page of heartbeats across all monitors ordered newest first.
//
// db is the database handle used for the query.
// page is the one-based page number.
// perPage is how many heartbeats each page contains.
func LoadAllMonitorChecksPage(db *gorm.DB, page, perPage int) ([]MonitorCheck, error) {
	if perPage < 1 {
		perPage = AdminListPageSize
	}
	if page < 1 {
		page = 1
	}

	var checks []MonitorCheck
	err := db.Preload("MonitorURL").
		Order("checked_at desc").
		Offset(PageOffset(page, perPage)).
		Limit(perPage).
		Find(&checks).Error
	if err != nil {
		return nil, err
	}
	return checks, nil
}

// LoadRecentMonitorChecks returns the most recent heartbeats across all monitors.
// limit caps how many rows are returned; values above MaxRecentHeartbeatsList are clamped.
func LoadRecentMonitorChecks(db *gorm.DB, limit int) ([]MonitorCheck, error) {
	if limit <= 0 || limit > MaxRecentHeartbeatsList {
		limit = MaxRecentHeartbeatsList
	}

	var checks []MonitorCheck
	if err := db.Preload("MonitorURL").Order("checked_at desc").Limit(limit).Find(&checks).Error; err != nil {
		return nil, err
	}
	return checks, nil
}

// LoadMonitorChecks returns heartbeats for one monitor ordered newest first.
// limit caps how many rows are returned; values above MaxMonitorDetailHeartbeats are clamped.
func LoadMonitorChecks(db *gorm.DB, monitorID uint, limit int) ([]MonitorCheck, error) {
	if limit <= 0 || limit > MaxMonitorDetailHeartbeats {
		limit = MaxMonitorDetailHeartbeats
	}

	var checks []MonitorCheck
	if err := db.Where("monitor_url_id = ?", monitorID).
		Order("checked_at desc").
		Limit(limit).
		Find(&checks).Error; err != nil {
		return nil, err
	}
	return checks, nil
}

// CountMonitorChecksForMonitor returns how many heartbeats exist for a monitor.
func CountMonitorChecksForMonitor(db *gorm.DB, monitorID uint) (int64, error) {
	var count int64
	err := db.Model(&MonitorCheck{}).Where("monitor_url_id = ?", monitorID).Count(&count).Error
	return count, err
}

// LoadMonitorChecksPage loads one page of heartbeats for a monitor ordered newest first.
//
// db is the database handle used for the query.
// monitorID is the `monitor_urls.id` whose checks are loaded.
// page is the one-based page number.
// perPage is how many heartbeats each page contains.
func LoadMonitorChecksPage(db *gorm.DB, monitorID uint, page, perPage int) ([]MonitorCheck, error) {
	if perPage < 1 {
		perPage = MonitorDetailListPageSize
	}
	if page < 1 {
		page = 1
	}

	var checks []MonitorCheck
	err := db.Where("monitor_url_id = ?", monitorID).
		Order("checked_at desc").
		Offset(PageOffset(page, perPage)).
		Limit(perPage).
		Find(&checks).Error
	if err != nil {
		return nil, err
	}
	return checks, nil
}

// LoadMonitorChecksSince groups check results by monitor ID since the given time.
func LoadMonitorChecksSince(db *gorm.DB, since time.Time) (map[uint][]MonitorCheck, error) {
	var checks []MonitorCheck
	if err := db.Where("checked_at >= ?", since).Order("checked_at asc").Find(&checks).Error; err != nil {
		return nil, err
	}

	byMonitor := make(map[uint][]MonitorCheck, len(checks))
	for _, check := range checks {
		byMonitor[check.MonitorURLID] = append(byMonitor[check.MonitorURLID], check)
	}
	return byMonitor, nil
}

// PruneMonitorChecks deletes checks older than monitorCheckRetention that are not
// among the maxMonitorChecksPerMonitor most recent checks for their monitor.
func PruneMonitorChecks(db *gorm.DB) error {
	cutoff := time.Now().Add(-monitorCheckRetention)

	var monitorIDs []uint
	if err := db.Model(&MonitorURL{}).Pluck("id", &monitorIDs).Error; err != nil {
		return err
	}

	for _, id := range monitorIDs {
		var protected []uint
		if err := db.Model(&MonitorCheck{}).
			Where("monitor_url_id = ?", id).
			Order("checked_at DESC").
			Limit(maxMonitorChecksPerMonitor).
			Pluck("id", &protected).Error; err != nil {
			return err
		}

		query := db.Where("monitor_url_id = ? AND checked_at < ?", id, cutoff)
		if len(protected) > 0 {
			query = query.Where("id NOT IN ?", protected)
		}

		if err := query.Delete(&MonitorCheck{}).Error; err != nil {
			return err
		}
	}

	return nil
}
