package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// PruneIncidents удаляет старые закрытые инциденты, чтобы ограничить рост данных.
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

// SeedMonitorURLs создаёт демонстрационные URL для мониторинга.
func SeedMonitorURLs(db *gorm.DB) (int, error) {
	seeds := []MonitorURL{
		{Name: "Example HTTP", URL: "http://example.com"},
		{Name: "Example HTTPS", URL: "https://example.com"},
	}
	created := 0
	for _, seed := range seeds {
		var existing MonitorURL
		err := db.Where("url = ?", seed.URL).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return created, err
		}
		if err := db.Create(&seed).Error; err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}
