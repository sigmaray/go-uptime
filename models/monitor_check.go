package models

import (
	"net/url"
	"strings"
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
	ID           uint       `gorm:"primaryKey"`
	MonitorURLID uint       `gorm:"not null;index"`
	MonitorURL   MonitorURL `gorm:"foreignKey:MonitorURLID"`
	CheckedAt    time.Time
	IsUp         bool `gorm:"not null"`
}

// DefaultMonitorName returns the site hostname from rawURL when no display name is provided.
func DefaultMonitorName(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return trimmed
	}
	return parsed.Host
}

// MonitorDisplayName returns the configured name or falls back to the URL hostname.
func MonitorDisplayName(monitor MonitorURL) string {
	name := strings.TrimSpace(monitor.Name)
	if name != "" {
		return name
	}
	return DefaultMonitorName(monitor.URL)
}

// ResolveMonitorName returns trimmed name or the default derived from rawURL.
func ResolveMonitorName(name, rawURL string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		return trimmed
	}
	return DefaultMonitorName(rawURL)
}

// RecordMonitorCheck persists one check result for uptime history and updates aggregated uptime stats.
func RecordMonitorCheck(db *gorm.DB, monitorID uint, checkedAt time.Time, isUp bool) error {
	if err := UpdateUptimeStats(db, monitorID, checkedAt, isUp); err != nil {
		return err
	}

	check := MonitorCheck{
		MonitorURLID: monitorID,
		CheckedAt:    checkedAt,
		IsUp:         isUp,
	}
	return db.Create(&check).Error
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
