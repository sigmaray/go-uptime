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
		"TotalMonitors":    backlog.Total,
		"DueWaiting":       backlog.DueWaiting,
		"NeverChecked":     backlog.NeverChecked,
		"CheckConcurrency": checkConcurrency,
		"MostOverdue":      backlog.MostOverdue,
		"WorkerStats":      workerStats,
	}, PageOptions{Title: "Info", ActiveNav: "info"})
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
