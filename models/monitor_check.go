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
	// HeartbeatHourMinutes is how many one-minute buckets the admin info heartbeat chart covers.
	HeartbeatHourMinutes = 60
)

// HeartbeatMinuteCount is successful and failed heartbeat totals for one minute bucket.
type HeartbeatMinuteCount struct {
	// BucketAt is the start of the minute (UTC, truncated).
	BucketAt time.Time
	// Success is how many heartbeats reported up in this minute.
	Success int64
	// Failed is how many heartbeats reported down in this minute.
	Failed int64
}

// Total returns Success + Failed for the minute.
func (c HeartbeatMinuteCount) Total() int64 {
	return c.Success + c.Failed
}

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

// CountHeartbeatsByMinute aggregates heartbeats into one-minute success/failure buckets.
// db is the database handle used to load recent heartbeats.
// now is the reference clock; the window ends at the current truncated minute and spans HeartbeatHourMinutes.
// Returned rows only include minutes that had at least one heartbeat; callers fill empty minutes.
func CountHeartbeatsByMinute(db *gorm.DB, now time.Time) ([]HeartbeatMinuteCount, error) {
	windowEnd := now.UTC().Truncate(time.Minute)
	windowStart := windowEnd.Add(-time.Duration(HeartbeatHourMinutes-1) * time.Minute)
	until := windowEnd.Add(time.Minute)

	var checks []MonitorCheck
	if err := db.Select("checked_at", "is_up").
		Where("checked_at >= ? AND checked_at < ?", windowStart, until).
		Find(&checks).Error; err != nil {
		return nil, err
	}

	byMinute := make(map[int64]*HeartbeatMinuteCount, HeartbeatHourMinutes)
	for _, check := range checks {
		bucketAt := check.CheckedAt.UTC().Truncate(time.Minute)
		key := bucketAt.Unix()
		entry, ok := byMinute[key]
		if !ok {
			entry = &HeartbeatMinuteCount{BucketAt: bucketAt}
			byMinute[key] = entry
		}
		if check.IsUp {
			entry.Success++
		} else {
			entry.Failed++
		}
	}

	counts := make([]HeartbeatMinuteCount, 0, len(byMinute))
	for _, entry := range byMinute {
		counts = append(counts, *entry)
	}
	return counts, nil
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
