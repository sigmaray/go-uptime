package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"go-uptime/models"
	"go-uptime/worker"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
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

// heartbeatMinuteBar is one minute column in the past-hour heartbeat chart.
type heartbeatMinuteBar struct {
	// Label is the short clock time shown in tooltips (HH:MM in local time).
	Label string
	// Title is the full accessible tooltip for the column.
	Title string
	// Success is how many heartbeats succeeded in this minute.
	Success int
	// Failed is how many heartbeats failed in this minute.
	Failed int
	// Total is Success + Failed.
	Total int
	// HeightPercent is the column height relative to the busiest minute (0–100).
	HeightPercent int
	// SuccessPercent is the success share of this column height (0–100 of HeightPercent stack).
	SuccessPercent int
	// FailedPercent is the failed share of this column height.
	FailedPercent int
}

// heartbeatHourChart is the past-hour per-minute success/failure breakdown.
type heartbeatHourChart struct {
	// Bars are exactly models.HeartbeatHourMinutes columns, oldest first.
	Bars []heartbeatMinuteBar
	// MaxPerMinute is the largest Total among Bars (chart scale).
	MaxPerMinute int
	// TotalSuccess is successful heartbeats across the whole hour.
	TotalSuccess int
	// TotalFailed is failed heartbeats across the whole hour.
	TotalFailed int
	// Total is TotalSuccess + TotalFailed.
	Total int
	// StartLabel is the oldest minute label shown on the X axis.
	StartLabel string
	// EndLabel is the newest minute label shown on the X axis.
	EndLabel string
}

// heartbeatHourChartPayload is the JSON shape consumed by Chart.js on the info page.
type heartbeatHourChartPayload struct {
	// Labels are minute clock labels (HH:MM), oldest first.
	Labels []string `json:"labels"`
	// Success is successful heartbeat counts aligned with Labels.
	Success []int `json:"success"`
	// Failed is failed heartbeat counts aligned with Labels.
	Failed []int `json:"failed"`
}

// tableRowCount is one PostgreSQL application table with its current row count.
type tableRowCount struct {
	// Name is the PostgreSQL table name (for example "monitor_urls").
	Name string
	// Count is the number of rows currently stored in the table.
	Count int64
}

// applicationTableModels lists every GORM model whose PostgreSQL table the app uses.
// Order is alphabetical by table name so the info page stays stable across reloads.
var applicationTableModels = []struct {
	name  string
	model any
}{
	{name: "app_settings", model: &models.AppSetting{}},
	{name: "incidents", model: &models.Incident{}},
	{name: "monitor_checks", model: &models.MonitorCheck{}},
	{name: "monitor_urls", model: &models.MonitorURL{}},
	{name: "stat_daily", model: &models.StatDaily{}},
	{name: "stat_hourly", model: &models.StatHourly{}},
	{name: "stat_minutely", model: &models.StatMinutely{}},
	{name: "users", model: &models.User{}},
}

// InfoPage shows monitor backlog, live worker metrics, and PostgreSQL table sizes.
func (h *Handler) InfoPage(c *gin.Context) {
	var monitors []models.MonitorURL
	if err := h.DB.Find(&monitors).Error; err != nil {
		log.Error().Err(err).Msg("failed to load monitors for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load monitors.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}

	tableCounts, err := loadTableRowCounts(h.DB)
	if err != nil {
		log.Error().Err(err).Msg("failed to load table row counts for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load table row counts.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}

	now := time.Now()
	minuteCounts, err := models.CountHeartbeatsByMinute(h.DB, now)
	if err != nil {
		log.Error().Err(err).Msg("failed to load heartbeat minute counts for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load heartbeat chart.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}
	heartbeatChart := buildHeartbeatHourChart(minuteCounts, now)
	chartJSON, err := marshalHeartbeatHourChartJSON(heartbeatChart)
	if err != nil {
		log.Error().Err(err).Msg("failed to encode heartbeat chart for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load heartbeat chart.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}

	globalIntervalSeconds := models.GetCheckIntervalSeconds(h.DB)
	backlog := computeMonitorBacklog(monitors, globalIntervalSeconds, now)

	checkConcurrency := worker.DefaultCheckConcurrency
	if h.Config != nil && h.Config.CheckConcurrency > 0 {
		checkConcurrency = h.Config.CheckConcurrency
	}

	workerStats := worker.Stats{}
	if h.Worker != nil {
		workerStats = h.Worker.Stats()
	}

	h.renderPage(c, http.StatusOK, "admin/info/index.html", gin.H{
		"TotalMonitors":          backlog.Total,
		"DueWaiting":             backlog.DueWaiting,
		"NeverChecked":           backlog.NeverChecked,
		"CheckConcurrency":       checkConcurrency,
		"MostOverdue":            backlog.MostOverdue,
		"WorkerStats":            workerStats,
		"UtilizationGauges":      buildUtilizationGauges(workerStats),
		"FleetComposition":       buildFleetComposition(monitors),
		"BacklogComposition":     buildBacklogComposition(backlog),
		"HeartbeatHourChart":     heartbeatChart,
		"HeartbeatHourChartJSON": chartJSON,
		"TableCounts":            tableCounts,
	}, PageOptions{Title: "Info", ActiveNav: "info"})
}

// loadTableRowCounts returns the current row count for each application table.
// db is the GORM handle used to run COUNT queries against PostgreSQL.
func loadTableRowCounts(db *gorm.DB) ([]tableRowCount, error) {
	counts := make([]tableRowCount, 0, len(applicationTableModels))
	for _, entry := range applicationTableModels {
		var count int64
		if err := db.Model(entry.model).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("count rows in %s: %w", entry.name, err)
		}
		counts = append(counts, tableRowCount{Name: entry.name, Count: count})
	}
	return counts, nil
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
// capacity is the capacity or wave size used as the bar denominator.
func newUtilizationGauge(label string, value, capacity int) utilizationGauge {
	return utilizationGauge{
		Label:   label,
		Value:   value,
		Max:     capacity,
		Percent: percentOf(value, capacity),
		Detail:  fmt.Sprintf("%d / %d", value, capacity),
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

// buildHeartbeatHourChart turns sparse per-minute counts into a fixed 60-column chart.
// counts are non-empty minute buckets from models.CountHeartbeatsByMinute.
// now is the same reference clock used when loading those counts.
func buildHeartbeatHourChart(counts []models.HeartbeatMinuteCount, now time.Time) heartbeatHourChart {
	windowEnd := now.UTC().Truncate(time.Minute)
	windowStart := windowEnd.Add(-time.Duration(models.HeartbeatHourMinutes-1) * time.Minute)
	loc := now.Location()

	byMinute := make(map[int64]models.HeartbeatMinuteCount, len(counts))
	for _, count := range counts {
		byMinute[count.BucketAt.UTC().Truncate(time.Minute).Unix()] = count
	}

	bars := make([]heartbeatMinuteBar, 0, models.HeartbeatHourMinutes)
	maxPerMinute := 0
	totalSuccess := 0
	totalFailed := 0

	for i := 0; i < models.HeartbeatHourMinutes; i++ {
		bucketAt := windowStart.Add(time.Duration(i) * time.Minute)
		count := byMinute[bucketAt.Unix()]
		success := int(count.Success)
		failed := int(count.Failed)
		total := success + failed
		if total > maxPerMinute {
			maxPerMinute = total
		}
		totalSuccess += success
		totalFailed += failed

		label := bucketAt.In(loc).Format("15:04")
		bars = append(bars, heartbeatMinuteBar{
			Label:   label,
			Title:   fmt.Sprintf("%s — %d successful, %d failed (%d total)", label, success, failed, total),
			Success: success,
			Failed:  failed,
			Total:   total,
		})
	}

	for i := range bars {
		bar := &bars[i]
		bar.HeightPercent = percentOf(bar.Total, maxPerMinute)
		if bar.Total > 0 {
			bar.SuccessPercent = percentOf(bar.Success, bar.Total)
			bar.FailedPercent = 100 - bar.SuccessPercent
			if bar.Failed == 0 {
				bar.FailedPercent = 0
				bar.SuccessPercent = 100
			} else if bar.Success == 0 {
				bar.SuccessPercent = 0
				bar.FailedPercent = 100
			}
		}
	}

	startLabel := ""
	endLabel := ""
	if len(bars) > 0 {
		startLabel = bars[0].Label
		endLabel = bars[len(bars)-1].Label
	}

	return heartbeatHourChart{
		Bars:         bars,
		MaxPerMinute: maxPerMinute,
		TotalSuccess: totalSuccess,
		TotalFailed:  totalFailed,
		Total:        totalSuccess + totalFailed,
		StartLabel:   startLabel,
		EndLabel:     endLabel,
	}
}

// marshalHeartbeatHourChartJSON encodes chart series for the Chart.js renderer.
// chart is the past-hour view model already filled with minute bars.
func marshalHeartbeatHourChartJSON(chart heartbeatHourChart) (template.JS, error) {
	payload := heartbeatHourChartPayload{
		Labels:  make([]string, len(chart.Bars)),
		Success: make([]int, len(chart.Bars)),
		Failed:  make([]int, len(chart.Bars)),
	}
	for i, bar := range chart.Bars {
		payload.Labels[i] = bar.Label
		payload.Success[i] = bar.Success
		payload.Failed[i] = bar.Failed
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal heartbeat hour chart: %w", err)
	}
	// JSON from encoding/json is safe to embed as a JS literal.
	return template.JS(raw), nil //nolint:gosec // G203: trusted JSON, not user HTML
}

// percentOf returns value as an integer percent of capacity, clamped to 0–100.
// value is the numerator (current usage or segment count).
// capacity is the denominator (capacity or total); non-positive capacity yields 0.
func percentOf(value, capacity int) int {
	if capacity <= 0 || value <= 0 {
		return 0
	}
	percent := (value * 100) / capacity
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
