package models

import "gorm.io/gorm"

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
