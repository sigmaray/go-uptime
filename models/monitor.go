package models

import (
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
)

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

// MonitorCheckIntervalSeconds returns the effective check interval for a monitor.
// monitor is the monitored URL whose optional override is inspected.
// globalSeconds is the default interval from app_settings.
func MonitorCheckIntervalSeconds(monitor MonitorURL, globalSeconds int) int {
	if monitor.CheckIntervalSeconds != nil && *monitor.CheckIntervalSeconds >= 10 {
		return *monitor.CheckIntervalSeconds
	}
	return globalSeconds
}

// CheckIntervalSecondsFormValue returns the monitor interval for HTML forms.
// An empty string means the monitor inherits the global setting.
func (m MonitorURL) CheckIntervalSecondsFormValue() string {
	if m.CheckIntervalSeconds == nil {
		return ""
	}
	return strconv.Itoa(*m.CheckIntervalSeconds)
}

// SeedMonitorURLs creates demonstration URLs for monitoring.
// db is the database handle used to insert missing seed rows.
// It returns the number of newly created monitors and any persistence error.
func SeedMonitorURLs(db *gorm.DB) (int, error) {
	seeds := []MonitorURL{
		{Name: "Example.com", URL: "https://example.com"},
		{Name: "Example.org", URL: "https://www.example.org"},
		{Name: "HTTPBin 200", URL: "https://httpbin.org/status/200"},
		{Name: "Google Connectivity", URL: "https://www.google.com/generate_204"},
		{Name: "Cloudflare", URL: "https://www.cloudflare.com"},
		{Name: "Wikipedia", URL: "https://www.wikipedia.org"},
		{Name: "Go", URL: "https://go.dev"},
		{Name: "Python", URL: "https://www.python.org"},
		{Name: "Debian", URL: "https://www.debian.org"},
		{Name: "GNU", URL: "https://www.gnu.org"},
		{Name: "Linux Kernel", URL: "https://www.kernel.org"},
		{Name: "IETF", URL: "https://www.ietf.org"},
		{Name: "W3C", URL: "https://www.w3.org"},
		{Name: "OpenStreetMap", URL: "https://www.openstreetmap.org"},
		{Name: "DuckDuckGo", URL: "https://duckduckgo.com"},
		{Name: "Mozilla", URL: "https://www.mozilla.org"},
		{Name: "Internet Archive", URL: "https://archive.org"},
		{Name: "JSONPlaceholder", URL: "https://jsonplaceholder.typicode.com"},
		{Name: "RFC Editor", URL: "https://www.rfc-editor.org"},
		{Name: "Cloudflare Status", URL: "https://www.cloudflarestatus.com"},
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
