package handlers

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Monitors list status filter query values.
const (
	monitorsStatusAll  = ""
	monitorsStatusUp   = "up"
	monitorsStatusDown = "down"
)

// MonitorsListFilter holds status and URL fragment filters for the admin monitors list.
type MonitorsListFilter struct {
	// Status is "up", "down", or empty for all monitors.
	Status string
	// Q is a case-insensitive URL substring to match; empty means no URL filter.
	Q string
	// Path is the list page path used when building filter URLs.
	Path string
}

// parseMonitorsListFilter reads status and URL search filters from the request query.
// c is the Gin request context whose Query values are inspected.
func parseMonitorsListFilter(c *gin.Context) MonitorsListFilter {
	return MonitorsListFilter{
		Path:   "/admin/monitors",
		Status: normalizeMonitorsStatus(c.Query("status")),
		Q:      strings.TrimSpace(c.Query("q")),
	}
}

// normalizeMonitorsStatus maps a raw status query value to a supported filter.
// raw is the status query parameter from the request.
func normalizeMonitorsStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case monitorsStatusUp:
		return monitorsStatusUp
	case monitorsStatusDown:
		return monitorsStatusDown
	default:
		return monitorsStatusAll
	}
}

// Apply adds WHERE clauses for the active status and URL fragment filters.
// db is the GORM query already scoped to monitor_urls.
func (f MonitorsListFilter) Apply(db *gorm.DB) *gorm.DB {
	switch f.Status {
	case monitorsStatusUp:
		// Match the Up badge: checked at least once and currently up.
		db = db.Where("monitor_urls.last_checked_at IS NOT NULL AND monitor_urls.is_up = ?", true)
	case monitorsStatusDown:
		// Match the Down badge: checked at least once and not currently up.
		db = db.Where("monitor_urls.last_checked_at IS NOT NULL AND monitor_urls.is_up IS DISTINCT FROM ?", true)
	}
	if f.Q != "" {
		// strpos treats the needle as a literal, so %, _, and \ need no escaping.
		db = db.Where("strpos(lower(monitor_urls.url), lower(?)) > 0", f.Q)
	}
	return db
}

// QueryValues returns non-default filter parameters for pagination and sort links.
func (f MonitorsListFilter) QueryValues() url.Values {
	q := url.Values{}
	if f.Status != monitorsStatusAll {
		q.Set("status", f.Status)
	}
	if f.Q != "" {
		q.Set("q", f.Q)
	}
	if len(q) == 0 {
		return nil
	}
	return q
}

// IsActive reports whether the given status filter option is selected.
// status is "up", "down", or empty for All.
func (f MonitorsListFilter) IsActive(status string) bool {
	return f.Status == normalizeMonitorsStatus(status)
}

// StatusURL builds a list URL that switches the status filter while keeping the URL search.
// status is "up", "down", or empty for All; the link resets to page 1.
// sort is the active list sort; only its column and order are preserved (not ExtraQuery).
func (f MonitorsListFilter) StatusURL(status string, sort ListSort) string {
	next := f
	next.Status = normalizeMonitorsStatus(status)
	sortOnly := url.Values{}
	if !sort.IsDefault() {
		sortOnly.Set("sort", sort.Column)
		sortOnly.Set("order", sort.Order)
	}
	return buildAdminListURLWithQuery(next.Path, 1, mergeURLValues(sortOnly, next.QueryValues()))
}

// mergeURLValues copies keys from each non-nil url.Values into a new map.
// values is a variadic list of query maps; later entries overwrite earlier keys on conflict.
func mergeURLValues(values ...url.Values) url.Values {
	merged := url.Values{}
	for _, item := range values {
		for key, vals := range item {
			for _, value := range vals {
				merged.Add(key, value)
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
