package worker

import (
	"time"

	"go-uptime/models"

	"github.com/rs/zerolog/log"
)

// backfillUptimeStatsIfNeeded rebuilds uptime buckets from check history when stats tables are empty.
// It runs once at worker start so existing monitors get aggregated history without a manual migration.
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
	}
}

// maintenanceLoop prunes old records on its own ticker so retention work stays off the check path.
// It closes maintenanceDone when stop is signaled so Stop can wait for a clean exit.
func (w *MonitorWorker) maintenanceLoop() {
	defer close(w.maintenanceDone)

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.runMaintenance()
		case <-w.stop:
			return
		}
	}
}

// runMaintenance prunes incidents, check history, and uptime buckets when the worker is active.
// It is a no-op while paused or when the database or config handle is missing.
func (w *MonitorWorker) runMaintenance() {
	if w.Paused() || w.db == nil || w.cfg == nil {
		return
	}

	if err := models.PruneIncidents(w.db, w.cfg.IncidentRetentionDays, w.cfg.MaxResolvedIncidentsPerMonitor); err != nil {
		log.Error().Err(err).Msg("failed to prune incidents")
	}

	if err := models.PruneMonitorChecks(w.db); err != nil {
		log.Error().Err(err).Msg("failed to prune monitor checks")
	}

	if err := models.PruneUptimeStats(w.db); err != nil {
		log.Error().Err(err).Msg("failed to prune uptime stats")
	}
}
