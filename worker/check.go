package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/internal/urlcheck"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// claimWaveMultiplier sizes each claim relative to check concurrency so the next
// claim can start while a prior wave is still draining slow probes.
const claimWaveMultiplier = 2

// runDueMonitors claims a bounded set of due monitors and dispatches probes without waiting.
func (w *MonitorWorker) runDueMonitors() {
	if w.Paused() {
		return
	}

	limit := w.claimBudget()
	if limit < 1 {
		return
	}

	now := time.Now()
	due, err := w.claimDueMonitors(now, limit)
	if err != nil {
		log.Error().Err(err).Msg("failed to load due monitor urls")
		return
	}
	if len(due) == 0 {
		return
	}

	w.dispatchChecks(due, w.checkMonitor)
}

// claimWaveLimit is the maximum number of monitors claimed in one scheduling tick.
// It returns twice the HTTP concurrency so slow probes do not starve the next claim.
func (w *MonitorWorker) claimWaveLimit() int {
	return w.checkConcurrency * claimWaveMultiplier
}

// claimBudget is how many additional monitors may be claimed given currently pending work.
// It returns zero when pending claimed checks already fill the wave limit (backpressure).
// It also limits claims to free resultJobs slots so completed probes are never dropped.
func (w *MonitorWorker) claimBudget() int {
	limit := w.claimWaveLimit()
	pending := int(w.waveDue.Load())
	if pending >= limit {
		return 0
	}
	budget := limit - pending

	// Reserve persist-queue slots for probes already claimed; they will enqueue next.
	free := cap(w.resultJobs) - len(w.resultJobs) - pending
	if free < 1 {
		return 0
	}
	if budget > free {
		budget = free
	}
	return budget
}

// monitorScheduleUpdate is one claimed monitor's next scheduled check time.
type monitorScheduleUpdate struct {
	// ID is the monitor_urls.id row that was claimed for the current wave.
	ID uint
	// NextCheckAt is the provisional time when the monitor may be checked again.
	NextCheckAt time.Time
}

// claimDueMonitors locks currently due monitors and postpones their next check before probing.
// now is the worker's current clock value used to select due rows and compute provisional schedules.
// limit caps how many due rows are claimed in this transaction; values below 1 claim nothing.
func (w *MonitorWorker) claimDueMonitors(now time.Time, limit int) ([]models.MonitorURL, error) {
	if limit < 1 {
		return nil, nil
	}

	var due []models.MonitorURL
	err := w.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("next_check_at IS NULL OR next_check_at <= ?", now).
			Order("next_check_at asc NULLS FIRST, id asc").
			Limit(limit).
			Find(&due).Error
		if err != nil {
			return err
		}
		if len(due) == 0 {
			return nil
		}

		globalInterval := models.GetCheckIntervalSeconds(tx)
		updates := make([]monitorScheduleUpdate, 0, len(due))
		for _, monitor := range due {
			intervalSec := models.MonitorCheckIntervalSeconds(monitor, globalInterval)
			updates = append(updates, monitorScheduleUpdate{
				ID:          monitor.ID,
				NextCheckAt: now.Add(claimLeaseDuration(intervalSec)),
			})
		}
		return updateClaimedMonitorSchedules(tx, updates)
	})
	if err != nil {
		return nil, err
	}
	return due, nil
}

// claimLeaseDuration is how long a claimed monitor stays off the due queue before probing finishes.
// intervalSeconds is the monitor's configured check interval.
// The lease covers a full claim wave (claimWaveMultiplier probe rounds) plus a short flush buffer so
// overlapping waves cannot reclaim a monitor while it waits for a slot, probes, or awaits batch persist.
func claimLeaseDuration(intervalSeconds int) time.Duration {
	interval := time.Duration(intervalSeconds) * time.Second
	// Worst case: last monitor in a 2x-concurrency wave waits one full probe round, then probes,
	// then waits up to the batch flush interval before next_check_at is rewritten.
	lease := urlcheck.RequestTimeout*time.Duration(claimWaveMultiplier) + 2*time.Second
	if interval > lease {
		return interval
	}
	return lease
}

// updateClaimedMonitorSchedules writes provisional next_check_at values in one SQL update.
// tx is the transaction that already locked the due monitor rows; updates are the claimed schedules.
func updateClaimedMonitorSchedules(tx *gorm.DB, updates []monitorScheduleUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	ids := make([]uint, 0, len(updates))
	for _, update := range updates {
		ids = append(ids, update.ID)
	}

	caseSQL, args := buildNextCheckAtCaseExpression(updates)
	result := tx.Model(&models.MonitorURL{}).
		Where("id IN ?", ids).
		Update("next_check_at", gorm.Expr(caseSQL, args...))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != int64(len(updates)) {
		return fmt.Errorf("claimed %d monitor schedules, updated %d", len(updates), result.RowsAffected)
	}
	return nil
}

// buildNextCheckAtCaseExpression builds the CASE expression used for bulk schedule updates.
// updates contains monitor ids and their corresponding provisional next_check_at values.
// THEN branches cast to timestamptz so PostgreSQL accepts the CASE result for next_check_at.
func buildNextCheckAtCaseExpression(updates []monitorScheduleUpdate) (string, []interface{}) {
	var b strings.Builder
	args := make([]interface{}, 0, len(updates)*2)
	b.WriteString("CASE id")
	for _, update := range updates {
		b.WriteString(" WHEN ? THEN ?::timestamptz")
		args = append(args, update.ID, update.NextCheckAt.UTC())
	}
	b.WriteString(" END")
	return b.String(), args
}

// dispatchChecks starts probe goroutines for monitors and returns without waiting.
// monitors is the claimed set to probe; checkFn runs one probe and must be concurrency-safe.
// A shared semaphore caps total in-flight HTTP work across overlapping waves.
// The concurrency slot is released before the result is enqueued so a slow DB flush cannot
// pin HTTP slots while resultJobs is full.
func (w *MonitorWorker) dispatchChecks(monitors []models.MonitorURL, checkFn func(models.MonitorURL) checkResult) {
	if len(monitors) == 0 {
		return
	}

	w.waveDue.Add(int64(len(monitors)))
	for _, monitor := range monitors {
		m := monitor
		w.wavesWG.Add(1)
		go func() {
			defer w.wavesWG.Done()
			defer w.waveDue.Add(-1)

			w.checkSem <- struct{}{}
			w.waveStarted.Add(1)
			w.inFlight.Add(1)
			res := checkFn(m)
			<-w.checkSem
			w.inFlight.Add(-1)
			w.waveStarted.Add(-1)

			w.enqueueCheckResult(res)
		}()
	}
}

// enqueueCheckResult puts one probe outcome onto resultJobs without holding a check slot.
// res is the completed probe result to persist.
// The send blocks when the queue is full so results are never dropped; HTTP slots are
// already released, and claimBudget stops new claims while the persist queue is backed up.
func (w *MonitorWorker) enqueueCheckResult(res checkResult) {
	w.resultJobs <- res
}

// IsMonitorDue reports whether a monitor should be checked at now based on its last check time.
// lastCheckedAt is nil when the monitor has never been checked.
// interval is the effective check interval for the monitor.
// now is the current time used for the due calculation.
func IsMonitorDue(lastCheckedAt *time.Time, interval time.Duration, now time.Time) bool {
	if lastCheckedAt == nil {
		return true
	}
	return now.Sub(*lastCheckedAt) >= interval
}

// checkMonitor probes one monitor and returns the result for batched persistence.
// monitor is the claimed monitor_urls row to probe.
func (w *MonitorWorker) checkMonitor(monitor models.MonitorURL) checkResult {
	ctx, cancel := context.WithTimeout(context.Background(), urlcheck.RequestTimeout)
	defer cancel()

	displayName := models.MonitorDisplayName(monitor)
	result := urlcheck.Probe(ctx, w.client, monitor.URL)
	elapsed := int(result.DurationMs)

	if !result.Up {
		w.recordMonitorRequest(displayName, monitor.URL, result.StatusCode, result.DurationMs, false, result.ErrMsg)
		return checkResult{monitor: monitor, isUp: false, errMsg: result.ErrMsg, elapsed: intPtr(elapsed)}
	}

	w.recordMonitorRequest(displayName, monitor.URL, result.StatusCode, result.DurationMs, true, "")
	return checkResult{monitor: monitor, isUp: true, errMsg: "", elapsed: intPtr(elapsed)}
}

// recordMonitorRequest stores one probe outcome in the in-memory request log ring.
// monitorName is the display name; url is the probed address; statusCode and responseTimeMs
// describe the HTTP outcome; isUp is availability; errMsg explains failures.
func (w *MonitorWorker) recordMonitorRequest(monitorName, url string, statusCode int, responseTimeMs int64, isUp bool, errMsg string) {
	applog.AddMonitorRequest(monitorName, url, statusCode, responseTimeMs, isUp, errMsg)
}

// intPtr returns a pointer to v for optional integer fields.
// v is the integer value to box.
func intPtr(v int) *int {
	return &v
}
