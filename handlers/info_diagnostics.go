package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/worker"
)

// infoDiagnosticsRecentErrorsLimit caps how many in-memory errors appear in the JSON blob.
const infoDiagnosticsRecentErrorsLimit = 50

// infoDiagnostics is the Cursor-friendly ops snapshot embedded on the admin info page.
type infoDiagnostics struct {
	// GeneratedAt is when this snapshot was assembled (RFC3339 in JSON).
	GeneratedAt time.Time `json:"generated_at"`
	// Environment is the non-secret GO_UPTIME_ENVIRONMENT value.
	Environment string `json:"environment"`
	// Worker holds live check-wave and queue depth metrics.
	Worker infoDiagnosticsWorker `json:"worker"`
	// Monitors summarizes fleet backlog and status composition.
	Monitors infoDiagnosticsMonitors `json:"monitors"`
	// HeartbeatsPastHour aggregates successful vs failed checks in the last hour.
	HeartbeatsPastHour infoDiagnosticsHeartbeats `json:"heartbeats_past_hour"`
	// Incidents counts open and total incident rows.
	Incidents infoDiagnosticsIncidents `json:"incidents"`
	// Applog summarizes in-memory log buffers and recent errors.
	Applog infoDiagnosticsApplog `json:"applog"`
	// Tables lists application tables with row counts and on-disk sizes.
	Tables []infoDiagnosticsTable `json:"tables"`
}

// infoDiagnosticsWorker is the worker section of the diagnostics JSON.
type infoDiagnosticsWorker struct {
	// Running is true when the monitor worker loop has started and not stopped.
	Running bool `json:"running"`
	// Paused is true when new check waves are suppressed (for example during e2e).
	Paused bool `json:"paused"`
	// DueThisWave is how many claimed monitors have not finished probing yet.
	DueThisWave int `json:"due_this_wave"`
	// InFlight is how many HTTP checks are executing right now.
	InFlight int `json:"in_flight"`
	// WaitingForSlot is how many claimed monitors still wait for a concurrency slot.
	WaitingForSlot int `json:"waiting_for_slot"`
	// MaxConcurrency is the configured concurrent HTTP check limit.
	MaxConcurrency int `json:"max_concurrency"`
	// ResultQueued is completed checks waiting to persist.
	ResultQueued int `json:"result_queued"`
	// ResultCapacity is the persist channel buffer size.
	ResultCapacity int `json:"result_capacity"`
	// NotifyQueued is how many status-change alerts sit in the notify channel.
	NotifyQueued int `json:"notify_queued"`
	// NotifyCapacity is the notify channel buffer size.
	NotifyCapacity int `json:"notify_capacity"`
}

// infoDiagnosticsMonitors is the monitors section of the diagnostics JSON.
type infoDiagnosticsMonitors struct {
	// Total is how many monitors exist.
	Total int `json:"total"`
	// DueWaiting is how many monitors are already due for a check.
	DueWaiting int `json:"due_waiting"`
	// NeverChecked is how many monitors have never been probed.
	NeverChecked int `json:"never_checked"`
	// Fleet breaks monitors down by current status.
	Fleet infoDiagnosticsFleet `json:"fleet"`
	// MostOverdue is the worst overdue monitor, or null when none exist.
	MostOverdue *infoDiagnosticsOverdue `json:"most_overdue"`
}

// infoDiagnosticsFleet counts monitors by up/down/unknown status.
type infoDiagnosticsFleet struct {
	// Up is monitors last seen healthy.
	Up int `json:"up"`
	// Down is monitors last seen unhealthy.
	Down int `json:"down"`
	// Unknown is monitors that have never been checked.
	Unknown int `json:"unknown"`
}

// infoDiagnosticsOverdue describes the single most overdue monitor for diagnostics.
type infoDiagnosticsOverdue struct {
	// ID is the monitor_urls primary key.
	ID uint `json:"id"`
	// Name is the display name shown in the admin UI.
	Name string `json:"name"`
	// URL is the monitored URL.
	URL string `json:"url"`
	// LastChecked is the formatted last-check timestamp.
	LastChecked string `json:"last_checked"`
	// OverdueBy is a short human-readable overdue duration.
	OverdueBy string `json:"overdue_by"`
}

// infoDiagnosticsHeartbeats summarizes past-hour heartbeat totals.
type infoDiagnosticsHeartbeats struct {
	// Total is successful plus failed heartbeats in the window.
	Total int `json:"total"`
	// Success is successful heartbeats in the window.
	Success int `json:"success"`
	// Failed is failed heartbeats in the window.
	Failed int `json:"failed"`
}

// infoDiagnosticsIncidents summarizes incident row counts.
type infoDiagnosticsIncidents struct {
	// Total is all incident rows.
	Total int64 `json:"total"`
	// Open is incidents that are not yet resolved.
	Open int64 `json:"open"`
}

// infoDiagnosticsApplog summarizes in-memory applog buffers for Cursor analysis.
type infoDiagnosticsApplog struct {
	// ErrorsStored is how many error records are currently buffered.
	ErrorsStored int64 `json:"errors_stored"`
	// EventsStored is how many application events are currently buffered.
	EventsStored int64 `json:"events_stored"`
	// RequestsStored is how many monitor HTTP request records are buffered.
	RequestsStored int64 `json:"requests_stored"`
	// RecentErrors are the newest error entries, capped for payload size.
	RecentErrors []applog.Entry `json:"recent_errors"`
}

// infoDiagnosticsTable is one PostgreSQL application table with size metrics.
type infoDiagnosticsTable struct {
	// Name is the PostgreSQL table name.
	Name string `json:"name"`
	// RowCount is the number of rows currently stored in the table.
	RowCount int64 `json:"row_count"`
	// TotalBytes is pg_total_relation_size for the table (including indexes).
	TotalBytes int64 `json:"total_bytes"`
}

// buildInfoDiagnostics assembles the Cursor-friendly diagnostics snapshot.
// now is the snapshot timestamp written into generated_at.
// environment is the non-secret application environment name.
// w is the live monitor worker (may be nil in tests).
// workerStats is the point-in-time Stats already loaded for the info page.
// backlog is the due/waiting summary already computed for the info page.
// fleet is the up/down/unknown composition already computed for the info page.
// heartbeat is the past-hour chart already computed for the info page.
// incidentTotal is the total number of incident rows.
// incidentOpen is how many incidents are still unresolved.
// tableCounts are application tables with row counts and disk sizes.
func buildInfoDiagnostics(
	now time.Time,
	environment string,
	w *worker.MonitorWorker,
	workerStats worker.Stats,
	backlog monitorBacklog,
	fleet compositionChart,
	heartbeat heartbeatHourChart,
	incidentTotal, incidentOpen int64,
	tableCounts []tableRowCount,
) infoDiagnostics {
	running := false
	paused := false
	if w != nil {
		running = w.Running()
		paused = w.Paused()
	}

	recentErrors := applog.RecentErrors()
	if len(recentErrors) > infoDiagnosticsRecentErrorsLimit {
		recentErrors = recentErrors[:infoDiagnosticsRecentErrorsLimit]
	}
	if recentErrors == nil {
		recentErrors = []applog.Entry{}
	}

	tables := make([]infoDiagnosticsTable, 0, len(tableCounts))
	for _, table := range tableCounts {
		tables = append(tables, infoDiagnosticsTable{
			Name:       table.Name,
			RowCount:   table.Count,
			TotalBytes: table.TotalBytes,
		})
	}

	return infoDiagnostics{
		GeneratedAt: now.UTC(),
		Environment: environment,
		Worker: infoDiagnosticsWorker{
			Running:        running,
			Paused:         paused,
			DueThisWave:    workerStats.DueThisWave,
			InFlight:       workerStats.InFlight,
			WaitingForSlot: workerStats.WaitingForSlot,
			MaxConcurrency: workerStats.MaxConcurrency,
			ResultQueued:   workerStats.ResultQueued,
			ResultCapacity: workerStats.ResultCapacity,
			NotifyQueued:   workerStats.NotifyQueued,
			NotifyCapacity: workerStats.NotifyCapacity,
		},
		Monitors: infoDiagnosticsMonitors{
			Total:        backlog.Total,
			DueWaiting:   backlog.DueWaiting,
			NeverChecked: backlog.NeverChecked,
			Fleet:        fleetCountsFromChart(fleet),
			MostOverdue:  overdueFromView(backlog.MostOverdue),
		},
		HeartbeatsPastHour: infoDiagnosticsHeartbeats{
			Total:   heartbeat.Total,
			Success: heartbeat.TotalSuccess,
			Failed:  heartbeat.TotalFailed,
		},
		Incidents: infoDiagnosticsIncidents{
			Total: incidentTotal,
			Open:  incidentOpen,
		},
		Applog: infoDiagnosticsApplog{
			ErrorsStored:   applog.CountErrors(),
			EventsStored:   applog.CountEvents(),
			RequestsStored: applog.CountMonitorRequests(),
			RecentErrors:   recentErrors,
		},
		Tables: tables,
	}
}

// fleetCountsFromChart maps stacked fleet segments into the diagnostics fleet object.
// chart is the composition chart already built for the info page UI.
func fleetCountsFromChart(chart compositionChart) infoDiagnosticsFleet {
	out := infoDiagnosticsFleet{}
	for _, segment := range chart.Segments {
		switch segment.Modifier {
		case "up":
			out.Up = segment.Count
		case "down":
			out.Down = segment.Count
		case "unknown":
			out.Unknown = segment.Count
		}
	}
	return out
}

// overdueFromView converts the template overdue view into a JSON pointer or nil.
// view is the most-overdue presentation fields from computeMonitorBacklog.
func overdueFromView(view overdueMonitorView) *infoDiagnosticsOverdue {
	if !view.HasOverdue {
		return nil
	}
	return &infoDiagnosticsOverdue{
		ID:          view.ID,
		Name:        view.Name,
		URL:         view.URL,
		LastChecked: view.LastChecked,
		OverdueBy:   view.OverdueBy,
	}
}

// marshalInfoDiagnosticsJSON pretty-prints diagnostics for the info page <pre> block.
// diagnostics is the snapshot already assembled by buildInfoDiagnostics.
func marshalInfoDiagnosticsJSON(diagnostics infoDiagnostics) (string, error) {
	raw, err := json.MarshalIndent(diagnostics, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal info diagnostics: %w", err)
	}
	return string(raw), nil
}
