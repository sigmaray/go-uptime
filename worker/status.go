package worker

import (
	"fmt"
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
				// The result channel was closed (worker is stopping), flush any remaining items.
				if len(batch) > 0 {
					w.flushBatch(batch)
				}
				return
			}
			batch = append(batch, res)
			// If we reached the batch size limit (e.g., 150), flush immediately.
			if len(batch) >= 150 {
				w.flushBatch(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.flushBatch(batch)
				batch = nil
			}
		}
	}
}

// flushBatch persists a completed result batch and logs a single operational error on failure.
// batch is the set of completed checks ready to be written to PostgreSQL.
func (w *MonitorWorker) flushBatch(batch []checkResult) {
	if err := w.processBatch(batch); err != nil {
		log.Error().Err(err).Msg("failed to process monitor checks batch")
	}
}

// processBatch executes a single all-or-nothing database transaction to save check results.
// It updates monitor statuses, tracks incident start/end times, calculates uptime stats,
// and bulk-inserts all individual check history records.
// batch is the set of completed HTTP probes to persist.
func (w *MonitorWorker) processBatch(batch []checkResult) error {
	if len(batch) == 0 {
		return nil
	}

	now := time.Now()

	err := w.db.Transaction(func(tx *gorm.DB) error {
		checks := make([]models.MonitorCheck, 0, len(batch))
		globalInterval := models.GetCheckIntervalSeconds(tx)

		for _, res := range batch {
			// Precalculate when this monitor should be checked next.
			nextAt := now.Add(time.Duration(models.MonitorCheckIntervalSeconds(res.monitor, globalInterval)) * time.Second)

			// Update the monitor's availability and next check time.
			updates := map[string]interface{}{
				"is_up":           res.isUp,
				"last_checked_at": now,
				"next_check_at":   nextAt,
				"last_error":      res.errMsg,
			}

			updateResult := tx.Model(&models.MonitorURL{}).Where("id = ?", res.monitor.ID).Updates(updates)
			if updateResult.Error != nil {
				return fmt.Errorf("update monitor %d status: %w", res.monitor.ID, updateResult.Error)
			}
			if updateResult.RowsAffected == 0 {
				return fmt.Errorf("update monitor %d status: monitor not found", res.monitor.ID)
			}

			// We still need to calculate minutely/hourly/daily stats
			// We can use the existing UpdateUptimeStats logic here since it relies on DB reads
			if err := models.UpdateUptimeStats(tx, res.monitor.ID, now, res.isUp); err != nil {
				return fmt.Errorf("update uptime stats for monitor %d: %w", res.monitor.ID, err)
			}

			// Create the history record to be bulk-inserted at the end of the transaction.
			checks = append(checks, models.MonitorCheck{
				MonitorURLID:   res.monitor.ID,
				CheckedAt:      now,
				IsUp:           res.isUp,
				ResponseTimeMs: res.elapsed,
			})

			// Handle Incident lifecycle: close existing if UP, create/update if DOWN.
			openIncident, err := models.FindOpenIncident(tx, res.monitor.ID)
			if err != nil {
				return fmt.Errorf("find open incident for monitor %d: %w", res.monitor.ID, err)
			}
			if res.isUp {
				if openIncident != nil {
					if err := tx.Model(openIncident).Update("resolved_at", now).Error; err != nil {
						return fmt.Errorf("resolve incident %d for monitor %d: %w", openIncident.ID, res.monitor.ID, err)
					}
				}
			} else {
				if openIncident == nil {
					if err := tx.Create(&models.Incident{
						MonitorURLID: res.monitor.ID,
						StartedAt:    now,
						ErrorMessage: res.errMsg,
					}).Error; err != nil {
						return fmt.Errorf("create incident for monitor %d: %w", res.monitor.ID, err)
					}
				} else if openIncident.ErrorMessage != res.errMsg {
					if err := tx.Model(openIncident).Update("error_message", res.errMsg).Error; err != nil {
						return fmt.Errorf("update incident %d for monitor %d: %w", openIncident.ID, res.monitor.ID, err)
					}
				}
			}
		}

		// Perform a bulk insert for all monitor checks in a single SQL query.
		if len(checks) > 0 {
			if err := tx.Create(&checks).Error; err != nil { // GORM bulk insert
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
