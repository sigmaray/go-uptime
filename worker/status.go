package worker

import (
	"fmt"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
)

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
		w.enqueueNotification(monitor, true, "")
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
		w.enqueueNotification(monitor, false, errMsg)
	}
}

func (w *MonitorWorker) recordCheck(monitorID uint, checkedAt time.Time, isUp bool, responseTimeMs *int) {
	if err := models.RecordMonitorCheck(w.db, monitorID, checkedAt, isUp, responseTimeMs); err != nil {
		log.Error().Err(err).Uint("monitor_id", monitorID).Msg("failed to record monitor check")
	}
}
