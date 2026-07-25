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
					w.processBatch(batch)
				}
				return
			}
			batch = append(batch, res)
			// If we reached the batch size limit (e.g., 150), flush immediately.
			if len(batch) >= 150 {
				w.processBatch(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.processBatch(batch)
				batch = nil
			}
		}
	}
}

// processBatch executes a single database transaction to save all check results.
// It updates monitor statuses, tracks incident start/end times, calculates uptime stats,
// and bulk-inserts all individual check history records.
func (w *MonitorWorker) processBatch(batch []checkResult) {
	now := time.Now()

	err := w.db.Transaction(func(tx *gorm.DB) error {
		var checks []models.MonitorCheck
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

			if err := tx.Model(&models.MonitorURL{}).Where("id = ?", res.monitor.ID).Updates(updates).Error; err != nil {
				log.Error().Err(err).Uint("monitor_id", res.monitor.ID).Msg("failed to update monitor status in batch")
				continue
			}

			// We still need to calculate minutely/hourly/daily stats
			// We can use the existing UpdateUptimeStats logic here since it relies on DB reads
			if err := models.UpdateUptimeStats(tx, res.monitor.ID, now, res.isUp); err != nil {
				log.Error().Err(err).Uint("monitor_id", res.monitor.ID).Msg("failed to update uptime stats in batch")
			}

			// Create the history record to be bulk-inserted at the end of the transaction.
			checks = append(checks, models.MonitorCheck{
				MonitorURLID:   res.monitor.ID,
				CheckedAt:      now,
				IsUp:           res.isUp,
				ResponseTimeMs: res.elapsed,
			})

			// Handle Incident lifecycle: close existing if UP, create/update if DOWN.
			openIncident, _ := models.FindOpenIncident(tx, res.monitor.ID)
			if res.isUp {
				if openIncident != nil {
					tx.Model(openIncident).Update("resolved_at", now)
				}
			} else {
				if openIncident == nil {
					tx.Create(&models.Incident{
						MonitorURLID: res.monitor.ID,
						StartedAt:    now,
						ErrorMessage: res.errMsg,
					})
				} else if openIncident.ErrorMessage != res.errMsg {
					tx.Model(openIncident).Update("error_message", res.errMsg)
				}
			}
		}

		// Perform a bulk insert for all monitor checks in a single SQL query.
		if len(checks) > 0 {
			if err := tx.Create(&checks).Error; err != nil { // GORM bulk insert
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Error().Err(err).Msg("failed to process monitor checks batch")
		return
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
}
