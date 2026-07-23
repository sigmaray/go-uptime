package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// CountIncidents returns the total number of incidents across all monitors.
func CountIncidents(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&Incident{}).Count(&count).Error
	return count, err
}

// LoadIncidentsPage loads one page of incident history ordered newest first.
//
// db is the database handle used for the query.
// page is the one-based page number.
// perPage is how many incidents each page contains.
func LoadIncidentsPage(db *gorm.DB, page, perPage int) ([]Incident, error) {
	if perPage < 1 {
		perPage = AdminListPageSize
	}
	if page < 1 {
		page = 1
	}

	var incidents []Incident
	err := db.Preload("MonitorURL").
		Order("started_at desc").
		Offset(PageOffset(page, perPage)).
		Limit(perPage).
		Find(&incidents).Error
	if err != nil {
		return nil, err
	}
	return incidents, nil
}

// CountIncidentsForMonitor returns how many incidents exist for a monitor.
func CountIncidentsForMonitor(db *gorm.DB, monitorID uint) (int64, error) {
	var count int64
	err := db.Model(&Incident{}).Where("monitor_url_id = ?", monitorID).Count(&count).Error
	return count, err
}

// LoadIncidentsForMonitorPage loads one page of incident history for a monitor
// ordered by newest first.
//
// db is the database handle used for the query.
// monitorID is the `monitor_urls.id` for which incidents are loaded.
// page is the one-based page number.
// perPage is how many incidents each page contains.
func LoadIncidentsForMonitorPage(db *gorm.DB, monitorID uint, page, perPage int) ([]Incident, error) {
	if perPage < 1 {
		perPage = MonitorDetailListPageSize
	}
	if page < 1 {
		page = 1
	}

	var incidents []Incident
	err := db.Where("monitor_url_id = ?", monitorID).
		Order("started_at desc").
		Offset(PageOffset(page, perPage)).
		Limit(perPage).
		Find(&incidents).Error
	if err != nil {
		return nil, err
	}
	return incidents, nil
}

// FindOpenIncident finds an open incident for the given monitor.
// db is the database handle used for the query.
// monitorURLID is the monitor_urls.id whose open incident is loaded.
// It returns nil, nil when no open incident exists.
func FindOpenIncident(db *gorm.DB, monitorURLID uint) (*Incident, error) {
	var incident Incident
	err := db.Where("monitor_url_id = ? AND resolved_at IS NULL", monitorURLID).First(&incident).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &incident, nil
}

// PruneIncidents deletes old resolved incidents to limit data growth.
// db is the database handle used for deletions.
// retentionDays controls how long resolved incidents are kept.
// maxPerMonitor limits how many resolved incidents each monitor may retain.
func PruneIncidents(db *gorm.DB, retentionDays, maxPerMonitor int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	if err := db.Where("resolved_at IS NOT NULL AND resolved_at < ?", cutoff).
		Delete(&Incident{}).Error; err != nil {
		return err
	}

	var monitorIDs []uint
	if err := db.Model(&MonitorURL{}).Pluck("id", &monitorIDs).Error; err != nil {
		return err
	}

	for _, id := range monitorIDs {
		var excess []uint
		err := db.Model(&Incident{}).
			Where("monitor_url_id = ? AND resolved_at IS NOT NULL", id).
			Order("resolved_at DESC").
			Offset(maxPerMonitor).
			Pluck("id", &excess).Error
		if err != nil {
			return err
		}
		if len(excess) == 0 {
			continue
		}
		if err := db.Delete(&Incident{}, excess).Error; err != nil {
			return err
		}
	}

	return nil
}
