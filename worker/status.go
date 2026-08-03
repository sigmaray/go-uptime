package worker

import (
	"fmt"
	"strings"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// batchResultsLoop receives check results from the worker routines and batches them.
// By writing to the database in bulk (e.g., every 2 seconds or 150 items),
// we avoid N+1 write issues and drastically reduce PostgreSQL transaction overhead.
func (w *MonitorWorker) batchResultsLoop() {
	defer close(w.batchDone)

	// Flush the batch every 2 seconds even if we haven't reached the batch size limit.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var batch []checkResult

	for {
		select {
		case res, ok := <-w.resultJobs:
			if !ok {
				// The result channel was closed (worker is stopping); drain leftovers locally.
				for len(batch) > 0 {
					batch = w.flushBatch(batch)
					w.persistBacklog.Store(int64(len(batch)))
				}
				return
			}
			batch = append(batch, res)
			w.persistBacklog.Store(int64(len(batch)))
			// If we reached the batch size limit (e.g., 150), flush immediately.
			if len(batch) >= 150 {
				batch = w.flushBatch(batch)
				w.persistBacklog.Store(int64(len(batch)))
			}
		case <-ticker.C:
			if len(batch) > 0 {
				batch = w.flushBatch(batch)
				w.persistBacklog.Store(int64(len(batch)))
			}
		}
	}
}

// flushBatch persists a completed result batch and retries briefly on failure.
// batch is the set of completed checks ready to be written to PostgreSQL.
// It returns results that still need another flush attempt (nil/empty on success).
// Leftovers stay in the batch loop so a full resultJobs channel cannot drop them
// and so the batch goroutine never blocks sending into the channel it alone reads.
func (w *MonitorWorker) flushBatch(batch []checkResult) []checkResult {
	const maxImmediateAttempts = 3

	var err error
	for attempt := 1; attempt <= maxImmediateAttempts; attempt++ {
		err = w.processBatch(batch)
		if err == nil {
			return nil
		}
		log.Error().Err(err).Int("attempt", attempt).Msg("failed to process monitor checks batch")
		if attempt < maxImmediateAttempts {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}
	}
	return retainFailedBatch(batch)
}

// maxPersistRequeues caps how many times a single check result may be retried after failed flushes.
const maxPersistRequeues = 5

// retainFailedBatch increments persist attempts and keeps results that may still be retried.
// batch is the set that exhausted immediate flush retries.
// Results that exceeded maxPersistRequeues are logged and discarded.
func retainFailedBatch(batch []checkResult) []checkResult {
	out := make([]checkResult, 0, len(batch))
	for _, res := range batch {
		res.persistAttempts++
		if res.persistAttempts > maxPersistRequeues {
			log.Error().
				Uint("monitor_id", res.monitor.ID).
				Int("persist_attempts", res.persistAttempts).
				Msg("dropping monitor check result after persist retries")
			continue
		}
		out = append(out, res)
	}
	return out
}

// monitorStatusUpdate holds one monitor row's fields written after a completed probe.
type monitorStatusUpdate struct {
	// ID is the monitor_urls.id being updated.
	ID uint
	// IsUp is the latest probe availability.
	IsUp bool
	// NextCheckAt is the final schedule after the probe completed.
	NextCheckAt time.Time
	// LastError is the probe error text; empty when the probe succeeded.
	LastError string
}

// processBatch executes a single all-or-nothing database transaction to save check results.
// It bulk-updates monitor statuses, tracks incident start/end times, calculates uptime stats,
// and bulk-inserts all individual check history records.
// batch is the set of completed HTTP probes to persist.
func (w *MonitorWorker) processBatch(batch []checkResult) error {
	if len(batch) == 0 {
		return nil
	}

	batch = collapseBatchByMonitor(batch)
	now := time.Now()

	err := w.db.Transaction(func(tx *gorm.DB) error {
		ids := make([]uint, 0, len(batch))
		for _, res := range batch {
			ids = append(ids, res.monitor.ID)
		}
		var existingIDs []uint
		if err := tx.Model(&models.MonitorURL{}).Where("id IN ?", ids).Pluck("id", &existingIDs).Error; err != nil {
			return fmt.Errorf("load existing monitors for batch: %w", err)
		}
		if len(existingIDs) == 0 {
			return fmt.Errorf("bulk update monitor statuses: none of %d monitors still exist", len(batch))
		}
		if len(existingIDs) != len(batch) {
			exist := make(map[uint]struct{}, len(existingIDs))
			for _, id := range existingIDs {
				exist[id] = struct{}{}
			}
			filtered := make([]checkResult, 0, len(existingIDs))
			for _, res := range batch {
				if _, ok := exist[res.monitor.ID]; ok {
					filtered = append(filtered, res)
				}
			}
			log.Warn().
				Int("dropped", len(batch)-len(filtered)).
				Int("remaining", len(filtered)).
				Msg("dropping deleted monitors from check result batch")
			batch = filtered
		}

		checks := make([]models.MonitorCheck, 0, len(batch))
		statusUpdates := make([]monitorStatusUpdate, 0, len(batch))
		globalInterval := models.GetCheckIntervalSeconds(tx)

		for _, res := range batch {
			nextAt := now.Add(time.Duration(models.MonitorCheckIntervalSeconds(res.monitor, globalInterval)) * time.Second)
			statusUpdates = append(statusUpdates, monitorStatusUpdate{
				ID:          res.monitor.ID,
				IsUp:        res.isUp,
				NextCheckAt: nextAt,
				LastError:   res.errMsg,
			})

			if err := models.ApplyUptimeInterval(tx, res.monitor.ID, res.monitor.LastCheckedAt, now, res.isUp); err != nil {
				return fmt.Errorf("update uptime stats for monitor %d: %w", res.monitor.ID, err)
			}

			checks = append(checks, models.MonitorCheck{
				MonitorURLID:   res.monitor.ID,
				CheckedAt:      now,
				IsUp:           res.isUp,
				ResponseTimeMs: res.elapsed,
			})
		}

		if err := updateMonitorStatuses(tx, now, statusUpdates); err != nil {
			return err
		}

		if err := applyIncidentChanges(tx, now, batch); err != nil {
			return err
		}

		if len(checks) > 0 {
			if err := tx.Create(&checks).Error; err != nil {
				return fmt.Errorf("insert monitor checks: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Trigger notifications outside the database transaction so we don't hold locks.
	for _, res := range batch {
		var wasUp, wasDown bool
		if res.monitor.IsUp != nil {
			wasUp = *res.monitor.IsUp
			wasDown = !wasUp
		}

		if res.isUp && wasDown {
			applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is UP", models.MonitorDisplayName(res.monitor), res.monitor.URL))
			w.enqueueNotification(res.monitor, true, "")
		} else if !res.isUp && wasUp {
			applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is DOWN: %s", models.MonitorDisplayName(res.monitor), res.monitor.URL, res.errMsg))
			w.enqueueNotification(res.monitor, false, res.errMsg)
		}
	}
	return nil
}

// collapseBatchByMonitor keeps the last result per monitor so bulk updates stay one row per id.
// batch is the flushed check-result slice that may contain duplicate monitor ids.
func collapseBatchByMonitor(batch []checkResult) []checkResult {
	if len(batch) <= 1 {
		return batch
	}

	indexByID := make(map[uint]int, len(batch))
	out := make([]checkResult, 0, len(batch))
	for _, res := range batch {
		if i, ok := indexByID[res.monitor.ID]; ok {
			out[i] = res
			continue
		}
		indexByID[res.monitor.ID] = len(out)
		out = append(out, res)
	}
	return out
}

// updateMonitorStatuses writes is_up, last_checked_at, next_check_at, and last_error for many monitors.
// tx is the open transaction; now is the shared last_checked_at timestamp; updates are per-monitor fields.
func updateMonitorStatuses(tx *gorm.DB, now time.Time, updates []monitorStatusUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	ids := make([]uint, 0, len(updates))
	var isUpCase, nextAtCase, errCase strings.Builder
	isUpArgs := make([]interface{}, 0, len(updates)*2)
	nextAtArgs := make([]interface{}, 0, len(updates)*2)
	errArgs := make([]interface{}, 0, len(updates)*2)

	isUpCase.WriteString("CASE id")
	nextAtCase.WriteString("CASE id")
	errCase.WriteString("CASE id")
	for _, update := range updates {
		ids = append(ids, update.ID)

		isUpCase.WriteString(" WHEN ? THEN ?::boolean")
		isUpArgs = append(isUpArgs, update.ID, update.IsUp)

		nextAtCase.WriteString(" WHEN ? THEN ?::timestamptz")
		nextAtArgs = append(nextAtArgs, update.ID, update.NextCheckAt.UTC())

		errCase.WriteString(" WHEN ? THEN ?::text")
		errArgs = append(errArgs, update.ID, update.LastError)
	}
	isUpCase.WriteString(" END")
	nextAtCase.WriteString(" END")
	errCase.WriteString(" END")

	result := tx.Model(&models.MonitorURL{}).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"is_up":           gorm.Expr(isUpCase.String(), isUpArgs...),
			"last_checked_at": now,
			"next_check_at":   gorm.Expr(nextAtCase.String(), nextAtArgs...),
			"last_error":      gorm.Expr(errCase.String(), errArgs...),
		})
	if result.Error != nil {
		return fmt.Errorf("bulk update monitor statuses: %w", result.Error)
	}
	if result.RowsAffected != int64(len(updates)) {
		return fmt.Errorf("bulk update monitor statuses: updated %d of %d", result.RowsAffected, len(updates))
	}
	return nil
}

// applyIncidentChanges resolves, creates, or updates open incidents for a completed check batch.
// tx is the open transaction; now is the incident resolution/start timestamp; batch holds probe outcomes.
func applyIncidentChanges(tx *gorm.DB, now time.Time, batch []checkResult) error {
	monitorIDs := make([]uint, 0, len(batch))
	for _, res := range batch {
		monitorIDs = append(monitorIDs, res.monitor.ID)
	}

	openByMonitor, err := models.FindOpenIncidentsByMonitorIDs(tx, monitorIDs)
	if err != nil {
		return fmt.Errorf("load open incidents: %w", err)
	}

	resolveIDs := make([]uint, 0)
	createIncidents := make([]models.Incident, 0)
	for _, res := range batch {
		openIncident, hasOpen := openByMonitor[res.monitor.ID]
		if res.isUp {
			if hasOpen {
				resolveIDs = append(resolveIDs, openIncident.ID)
			}
			continue
		}
		if !hasOpen {
			createIncidents = append(createIncidents, models.Incident{
				MonitorURLID: res.monitor.ID,
				StartedAt:    now,
				ErrorMessage: res.errMsg,
			})
			continue
		}
		if openIncident.ErrorMessage != res.errMsg {
			if err := tx.Model(&models.Incident{}).Where("id = ?", openIncident.ID).
				Update("error_message", res.errMsg).Error; err != nil {
				return fmt.Errorf("update incident %d for monitor %d: %w", openIncident.ID, res.monitor.ID, err)
			}
		}
	}

	if len(resolveIDs) > 0 {
		if err := tx.Model(&models.Incident{}).Where("id IN ?", resolveIDs).
			Update("resolved_at", now).Error; err != nil {
			return fmt.Errorf("resolve incidents: %w", err)
		}
	}
	if len(createIncidents) > 0 {
		for i := range createIncidents {
			if err := tx.Create(&createIncidents[i]).Error; err != nil {
				if models.IsUniqueViolation(err) {
					continue
				}
				return fmt.Errorf("create incident for monitor %d: %w", createIncidents[i].MonitorURLID, err)
			}
		}
	}
	return nil
}
