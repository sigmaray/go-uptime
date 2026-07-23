package models

import (
	"errors"
	"net/url"
	"strings"

	"gorm.io/gorm"
)

// MonitorCheckIntervalSeconds returns the effective check interval for a monitor.
// monitor is the monitored URL whose optional override is inspected.
// globalSeconds is the default interval from app_settings.
func MonitorCheckIntervalSeconds(monitor MonitorURL, globalSeconds int) int {
	if monitor.CheckIntervalSeconds != nil && *monitor.CheckIntervalSeconds >= 10 {
		return *monitor.CheckIntervalSeconds
	}
	return globalSeconds
}

// DefaultMonitorName returns the site hostname from rawURL when no display name is provided.
// rawURL is the monitor target URL used to derive a fallback name.
func DefaultMonitorName(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return trimmed
	}
	return parsed.Host
}

// MonitorDisplayName returns the configured name or falls back to the URL hostname.
// monitor is the monitored URL whose display name is resolved.
func MonitorDisplayName(monitor MonitorURL) string {
	name := strings.TrimSpace(monitor.Name)
	if name != "" {
		return name
	}
	return DefaultMonitorName(monitor.URL)
}

// ResolveMonitorName returns trimmed name or the default derived from rawURL.
// name is the optional user-provided display name.
// rawURL is used when name is empty.
func ResolveMonitorName(name, rawURL string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		return trimmed
	}
	return DefaultMonitorName(rawURL)
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
