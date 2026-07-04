// Package worker performs background HTTP checks of monitored URLs.
package worker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go-uptime/config"
	"go-uptime/internal/applog"
	"go-uptime/internal/notify"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const requestTimeout = 15 * time.Second

// MonitorWorker periodically checks URLs from the database.
type MonitorWorker struct {
	db                *gorm.DB
	cfg               *config.Config
	client            *http.Client
	stop              chan struct{}
	lastMaintenanceAt time.Time
}

// New creates a new background monitoring worker instance.
func New(db *gorm.DB, cfg *config.Config) *MonitorWorker {
	return &MonitorWorker{
		db:  db,
		cfg: cfg,
		client: &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		stop: make(chan struct{}),
	}
}

// Start runs the check loop in a separate goroutine.
func (w *MonitorWorker) Start() {
	go w.backfillUptimeStatsIfNeeded()
	go w.loop()
}

func (w *MonitorWorker) backfillUptimeStatsIfNeeded() {
	var count int64
	if err := w.db.Model(&models.StatMinutely{}).Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("failed to count uptime stats")
		return
	}
	if count > 0 {
		return
	}

	if err := models.BackfillUptimeStats(w.db); err != nil {
		log.Error().Err(err).Msg("failed to backfill uptime stats")
		applog.AddError("failed to backfill uptime stats", err.Error())
	}
}

// Stop stops the worker.
func (w *MonitorWorker) Stop() {
	close(w.stop)
}

func (w *MonitorWorker) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	w.runDueMonitors()

	for {
		select {
		case <-ticker.C:
			w.runDueMonitors()
		case <-w.stop:
			return
		}
	}
}

// runDueMonitors checks monitors whose individual interval has elapsed and runs periodic maintenance.
func (w *MonitorWorker) runDueMonitors() {
	var monitors []models.MonitorURL
	if err := w.db.Find(&monitors).Error; err != nil {
		log.Error().Err(err).Msg("failed to load monitor urls")
		applog.AddError("failed to load monitor urls", err.Error())
		return
	}

	now := time.Now()
	globalIntervalSeconds := models.GetCheckIntervalSeconds(w.db)

	for _, monitor := range monitors {
		intervalSeconds := models.MonitorCheckIntervalSeconds(monitor, globalIntervalSeconds)
		if !isMonitorDue(monitor.LastCheckedAt, time.Duration(intervalSeconds)*time.Second, now) {
			continue
		}
		w.checkMonitor(monitor)
	}

	w.runMaintenanceIfDue(now)
}

// isMonitorDue reports whether a monitor should be checked at now based on its last check time.
// lastCheckedAt is nil when the monitor has never been checked.
// interval is the effective check interval for the monitor.
// now is the current time used for the due calculation.
func isMonitorDue(lastCheckedAt *time.Time, interval time.Duration, now time.Time) bool {
	if lastCheckedAt == nil {
		return true
	}
	return now.Sub(*lastCheckedAt) >= interval
}

// runMaintenanceIfDue prunes old records at most once per minute.
// now is the current time used to throttle maintenance work.
func (w *MonitorWorker) runMaintenanceIfDue(now time.Time) {
	if !w.lastMaintenanceAt.IsZero() && now.Sub(w.lastMaintenanceAt) < time.Minute {
		return
	}
	w.lastMaintenanceAt = now

	if err := models.PruneIncidents(w.db, w.cfg.IncidentRetentionDays, w.cfg.MaxResolvedIncidentsPerMonitor); err != nil {
		log.Error().Err(err).Msg("failed to prune incidents")
		applog.AddError("failed to prune incidents", err.Error())
	}

	if err := models.PruneMonitorChecks(w.db); err != nil {
		log.Error().Err(err).Msg("failed to prune monitor checks")
		applog.AddError("failed to prune monitor checks", err.Error())
	}

	if err := models.PruneUptimeStats(w.db); err != nil {
		log.Error().Err(err).Msg("failed to prune uptime stats")
		applog.AddError("failed to prune uptime stats", err.Error())
	}
}

func (w *MonitorWorker) checkMonitor(monitor models.MonitorURL) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	displayName := models.MonitorDisplayName(monitor)
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, monitor.URL, nil)
	if err != nil {
		elapsed := time.Since(start).Milliseconds()
		w.recordMonitorRequest(displayName, monitor.URL, 0, elapsed, false, err.Error())
		w.markDown(monitor, err.Error(), intPtr(int(elapsed)))
		return
	}
	req.Header.Set("User-Agent", "go-uptime-monitor/1.0")

	resp, err := w.client.Do(req)
	elapsed := time.Since(start).Milliseconds()
	now := time.Now()
	if err != nil {
		w.recordMonitorRequest(displayName, monitor.URL, 0, elapsed, false, err.Error())
		w.markDown(monitor, err.Error(), intPtr(int(elapsed)))
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
		w.recordMonitorRequest(displayName, monitor.URL, resp.StatusCode, elapsed, false, errMsg)
		w.markDown(monitor, errMsg, intPtr(int(elapsed)))
		return
	}

	w.recordMonitorRequest(displayName, monitor.URL, resp.StatusCode, elapsed, true, "")
	w.markUp(monitor, now, intPtr(int(elapsed)))
}

func (w *MonitorWorker) recordMonitorRequest(monitorName, url string, statusCode int, responseTimeMs int64, isUp bool, errMsg string) {
	applog.AddMonitorRequest(monitorName, url, statusCode, responseTimeMs, isUp, errMsg)
}

func intPtr(v int) *int {
	return &v
}

// shouldNotifyStateChange reports whether a status transition should trigger alerts.
// previous nil IsUp means the monitor has not been checked yet — no alert on baseline.
func shouldNotifyStateChange(previous *bool, nowUp bool) bool {
	if previous == nil {
		return false
	}
	return *previous != nowUp
}

func (w *MonitorWorker) markUp(monitor models.MonitorURL, checkedAt time.Time, responseTimeMs *int) {
	wasDown := shouldNotifyStateChange(monitor.IsUp, true)

	updates := map[string]interface{}{
		"is_up":           true,
		"last_checked_at": checkedAt,
		"last_error":      "",
	}
	if err := w.db.Model(&models.MonitorURL{}).Where("id = ?", monitor.ID).Updates(updates).Error; err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to update monitor status")
		return
	}

	w.recordCheck(monitor.ID, checkedAt, true, responseTimeMs)

	openIncident, err := models.FindOpenIncident(w.db, monitor.ID)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to find open incident")
		return
	}
	if openIncident != nil {
		now := checkedAt
		if err := w.db.Model(openIncident).Update("resolved_at", now).Error; err != nil {
			log.Error().Err(err).Uint("incident_id", openIncident.ID).Msg("failed to resolve incident")
		}
	}

	if wasDown {
		applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is UP", models.MonitorDisplayName(monitor), monitor.URL))
		w.sendNotifications(monitor, true, "")
	}
}

func (w *MonitorWorker) markDown(monitor models.MonitorURL, errMsg string, responseTimeMs *int) {
	wasUp := shouldNotifyStateChange(monitor.IsUp, false)
	now := time.Now()
	updates := map[string]interface{}{
		"is_up":           false,
		"last_checked_at": now,
		"last_error":      errMsg,
	}
	if err := w.db.Model(&models.MonitorURL{}).Where("id = ?", monitor.ID).Updates(updates).Error; err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to update monitor status")
		return
	}

	w.recordCheck(monitor.ID, now, false, responseTimeMs)

	openIncident, err := models.FindOpenIncident(w.db, monitor.ID)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to find open incident")
		return
	}
	if openIncident == nil {
		incident := models.Incident{
			MonitorURLID: monitor.ID,
			StartedAt:    now,
			ErrorMessage: errMsg,
		}
		if err := w.db.Create(&incident).Error; err != nil {
			log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to create incident")
		}
	} else if openIncident.ErrorMessage != errMsg {
		_ = w.db.Model(openIncident).Update("error_message", errMsg).Error
	}

	if wasUp {
		applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is DOWN: %s", models.MonitorDisplayName(monitor), monitor.URL, errMsg))
		w.sendNotifications(monitor, false, errMsg)
	}
}

func (w *MonitorWorker) recordCheck(monitorID uint, checkedAt time.Time, isUp bool, responseTimeMs *int) {
	if err := models.RecordMonitorCheck(w.db, monitorID, checkedAt, isUp, responseTimeMs); err != nil {
		log.Error().Err(err).Uint("monitor_id", monitorID).Msg("failed to record monitor check")
	}
}

// sendNotifications sends status-change notifications when channels are configured and enabled for the monitor.
func (w *MonitorWorker) sendNotifications(monitor models.MonitorURL, isUp bool, errMsg string) {
	if !monitor.NotifyTelegram && !monitor.NotifySMTP {
		return
	}

	settings, err := models.LoadNotificationSettings(w.db)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to load notification settings")
		applog.AddError("failed to load notification settings", err.Error())
		return
	}

	change := notify.MonitorStateChange{
		DisplayName: models.MonitorDisplayName(monitor),
		URL:         monitor.URL,
		IsUp:        isUp,
		Error:       errMsg,
	}
	if err := notify.SendMonitorStateChange(settings, monitor, change); err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to send monitor notification")
		applog.AddError("failed to send monitor notification", err.Error())
	}
}
