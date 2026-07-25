package handlers

import (
	"fmt"
	"net/http"
	"time"

	"go-uptime/models"
	"go-uptime/worker"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// overdueMonitorView holds presentation fields for the most overdue monitor.
type overdueMonitorView struct {
	ID          uint
	Name        string
	URL         string
	LastChecked string
	OverdueBy   string
	HasOverdue  bool
}

// monitorBacklog summarizes due/waiting monitors for the admin info page.
type monitorBacklog struct {
	Total        int
	DueWaiting   int
	NeverChecked int
	MostOverdue  overdueMonitorView
}

// utilizationGauge is one Bootstrap progress bar for live worker capacity.
type utilizationGauge struct {
	// Label is the short metric name shown above the bar.
	Label string
	// Value is the current absolute count.
	Value int
	// Max is the denominator for the gauge; zero means an idle empty bar.
	Max int
	// Percent is Value/Max capped to 0–100 for CSS width.
	Percent int
	// Detail is the "value / max" text next to the bar.
	Detail string
}

// compositionSegment is one mutually exclusive slice of a stacked composition chart.
type compositionSegment struct {
	// Label is the human-readable segment name (for example "Up").
	Label string
	// Count is how many monitors fall into this segment.
	Count int
	// Percent is Count/Total capped to 0–100 for CSS width.
	Percent int
	// Modifier is a BEM modifier suffix such as "up" or "due".
	Modifier string
}

// compositionChart groups stacked segments for fleet or backlog breakdowns.
type compositionChart struct {
	// Total is the sum of segment counts (usually all monitors).
	Total int
	// Segments are ordered slices used by the stacked bar and legend.
	Segments []compositionSegment
}

// InfoPage shows monitor backlog and live worker queue metrics for operators.
func (h *Handler) InfoPage(c *gin.Context) {
	var monitors []models.MonitorURL
	if err := h.DB.Find(&monitors).Error; err != nil {
		log.Error().Err(err).Msg("failed to load monitors for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load monitors.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}

	now := time.Now()
	globalIntervalSeconds := models.GetCheckIntervalSeconds(h.DB)
	backlog := computeMonitorBacklog(monitors, globalIntervalSeconds, now)

	checkConcurrency := 50
	if h.Config != nil && h.Config.CheckConcurrency > 0 {
		checkConcurrency = h.Config.CheckConcurrency
	}

	workerStats := worker.Stats{}
	if h.Worker != nil {
		workerStats = h.Worker.Stats()
	}

	h.renderPage(c, http.StatusOK, "admin/info/index.html", gin.H{
		"TotalMonitors":      backlog.Total,
		"DueWaiting":         backlog.DueWaiting,
		"NeverChecked":       backlog.NeverChecked,
		"CheckConcurrency":   checkConcurrency,
		"MostOverdue":        backlog.MostOverdue,
		"WorkerStats":        workerStats,
		"UtilizationGauges":  buildUtilizationGauges(workerStats),
		"FleetComposition":   buildFleetComposition(monitors),
		"BacklogComposition": buildBacklogComposition(backlog),
	}, PageOptions{Title: "Info", ActiveNav: "info"})
}

// buildUtilizationGauges maps live worker stats into progress-bar view models.
// stats is the point-in-time worker.Stats snapshot from the monitor worker.
func buildUtilizationGauges(stats worker.Stats) []utilizationGauge {
	waitingMax := stats.DueThisWave
	if waitingMax < stats.WaitingForSlot {
		waitingMax = stats.WaitingForSlot
	}
	return []utilizationGauge{
		newUtilizationGauge("Check slots", stats.InFlight, stats.MaxConcurrency),
		newUtilizationGauge("Waiting for slot", stats.WaitingForSlot, waitingMax),
		newUtilizationGauge("Notify queue", stats.NotifyQueued, stats.NotifyCapacity),
	}
}

// newUtilizationGauge builds one gauge with a safe percentage and detail label.
// label is the short metric title shown in the UI.
// value is the current absolute count.
// max is the capacity or wave size used as the bar denominator.
func newUtilizationGauge(label string, value, max int) utilizationGauge {
	return utilizationGauge{
		Label:   label,
		Value:   value,
		Max:     max,
		Percent: percentOf(value, max),
		Detail:  fmt.Sprintf("%d / %d", value, max),
	}
}

// buildFleetComposition counts monitors by current status for a stacked chart.
// monitors is the full monitor list loaded for the info page.
func buildFleetComposition(monitors []models.MonitorURL) compositionChart {
	up, down, unknown := 0, 0, 0
	for i := range monitors {
		monitor := monitors[i]
		if monitor.LastCheckedAt == nil {
			unknown++
			continue
		}
		if monitor.IsUp != nil && *monitor.IsUp {
			up++
			continue
		}
		down++
	}
	total := len(monitors)
	return compositionChart{
		Total: total,
		Segments: []compositionSegment{
			newCompositionSegment("Up", up, total, "up"),
			newCompositionSegment("Down", down, total, "down"),
			newCompositionSegment("Unknown", unknown, total, "unknown"),
		},
	}
}

// buildBacklogComposition turns backlog counts into mutually exclusive segments.
// backlog is the due/waiting summary already computed for the info page.
// Never-checked monitors are always due, so "Due" here means previously checked and overdue.
func buildBacklogComposition(backlog monitorBacklog) compositionChart {
	dueChecked := backlog.DueWaiting - backlog.NeverChecked
	if dueChecked < 0 {
		dueChecked = 0
	}
	onSchedule := backlog.Total - backlog.DueWaiting
	if onSchedule < 0 {
		onSchedule = 0
	}
	return compositionChart{
		Total: backlog.Total,
		Segments: []compositionSegment{
			newCompositionSegment("Due", dueChecked, backlog.Total, "due"),
			newCompositionSegment("Never checked", backlog.NeverChecked, backlog.Total, "never"),
			newCompositionSegment("On schedule", onSchedule, backlog.Total, "schedule"),
		},
	}
}

// newCompositionSegment builds one stacked-bar slice with a CSS-safe percentage.
// label is the legend text for the segment.
// count is how many items belong to the segment.
// total is the chart denominator used for Percent.
// modifier is the BEM modifier suffix applied in the template.
func newCompositionSegment(label string, count, total int, modifier string) compositionSegment {
	return compositionSegment{
		Label:    label,
		Count:    count,
		Percent:  percentOf(count, total),
		Modifier: modifier,
	}
}

// percentOf returns value as an integer percent of max, clamped to 0–100.
// value is the numerator (current usage or segment count).
// max is the denominator (capacity or total); non-positive max yields 0.
func percentOf(value, max int) int {
	if max <= 0 || value <= 0 {
		return 0
	}
	percent := (value * 100) / max
	if percent > 100 {
		return 100
	}
	return percent
}

// computeMonitorBacklog counts due monitors and picks the most overdue checked monitor.
// monitors is the full monitor list from the database.
// globalIntervalSeconds is the default check interval from app settings.
// now is the reference time for due and overdue calculations.
func computeMonitorBacklog(monitors []models.MonitorURL, globalIntervalSeconds int, now time.Time) monitorBacklog {
	backlog := monitorBacklog{Total: len(monitors)}

	var mostOverdue *models.MonitorURL
	var mostOverdueBy time.Duration

	for i := range monitors {
		monitor := &monitors[i]
		intervalSeconds := models.MonitorCheckIntervalSeconds(*monitor, globalIntervalSeconds)
		interval := time.Duration(intervalSeconds) * time.Second

		if monitor.LastCheckedAt == nil {
			backlog.NeverChecked++
		}

		if !worker.IsMonitorDue(monitor.LastCheckedAt, interval, now) {
			continue
		}
		backlog.DueWaiting++

		if monitor.LastCheckedAt == nil {
			continue
		}
		overdueBy := now.Sub(*monitor.LastCheckedAt) - interval
		if mostOverdue == nil || overdueBy > mostOverdueBy {
			mostOverdue = monitor
			mostOverdueBy = overdueBy
		}
	}

	backlog.MostOverdue = buildOverdueMonitorView(mostOverdue, mostOverdueBy)
	return backlog
}

// buildOverdueMonitorView maps the worst overdue monitor into template-friendly fields.
// monitor is the selected overdue monitor, or nil when none exist.
// overdueBy is how long past the due time that monitor is.
func buildOverdueMonitorView(monitor *models.MonitorURL, overdueBy time.Duration) overdueMonitorView {
	if monitor == nil {
		return overdueMonitorView{}
	}

	lastChecked := "—"
	if monitor.LastCheckedAt != nil {
		lastChecked = monitor.LastCheckedAt.Format("2006-01-02 15:04:05")
	}

	return overdueMonitorView{
		ID:          monitor.ID,
		Name:        models.MonitorDisplayName(*monitor),
		URL:         monitor.URL,
		LastChecked: lastChecked,
		OverdueBy:   formatDuration(overdueBy),
		HasOverdue:  true,
	}
}

// formatDuration renders a non-negative duration in a short human-readable form.
// d is the duration to format; negative values are treated as zero.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}
