package worker

import (
	"time"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
)

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
