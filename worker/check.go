package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/internal/urlcheck"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// runDueMonitors checks monitors whose individual interval has elapsed and runs periodic maintenance.
func (w *MonitorWorker) runDueMonitors() {
	if w.Paused() {
		return
	}

	now := time.Now()
	due, err := w.claimDueMonitors(now)
	if err != nil {
		log.Error().Err(err).Msg("failed to load due monitor urls")
		return
	}

	runChecksConcurrently(due, w.checkConcurrency, w.checkMonitor, &checkWaveCounters{
		due:      &w.waveDue,
		started:  &w.waveStarted,
		inFlight: &w.inFlight,
	})

	w.runMaintenanceIfDue(now)
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
func (w *MonitorWorker) claimDueMonitors(now time.Time) ([]models.MonitorURL, error) {
	var due []models.MonitorURL
	err := w.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("next_check_at IS NULL OR next_check_at <= ?", now).
			Order("next_check_at asc NULLS FIRST, id asc").
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
			nextAt := now.Add(time.Duration(models.MonitorCheckIntervalSeconds(monitor, globalInterval)) * time.Second)
			updates = append(updates, monitorScheduleUpdate{ID: monitor.ID, NextCheckAt: nextAt})
		}
		return updateClaimedMonitorSchedules(tx, updates)
	})
	if err != nil {
		return nil, err
	}
	return due, nil
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
func buildNextCheckAtCaseExpression(updates []monitorScheduleUpdate) (string, []interface{}) {
	var b strings.Builder
	args := make([]interface{}, 0, len(updates)*2)
	b.WriteString("CASE id")
	for _, update := range updates {
		b.WriteString(" WHEN ? THEN ?")
		args = append(args, update.ID, update.NextCheckAt)
	}
	b.WriteString(" END")
	return b.String(), args
}

// checkWaveCounters holds atomics updated while a check wave runs.
// due is the wave size; started counts acquired slots; inFlight counts running checks.
type checkWaveCounters struct {
	due      *atomic.Int64
	started  *atomic.Int64
	inFlight *atomic.Int64
}

// runChecksConcurrently runs checkFn for each monitor with at most maxConcurrent goroutines.
// monitors is the list of due monitors to check in this wave.
// maxConcurrent caps how many HTTP checks may run at the same time; values below 1 become 1.
// checkFn performs one monitor check and must be safe to call concurrently.
// counters, when non-nil, receives live wave progress for Stats().
func runChecksConcurrently(monitors []models.MonitorURL, maxConcurrent int, checkFn func(models.MonitorURL), counters *checkWaveCounters) {
	if len(monitors) == 0 {
		return
	}
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}

	if counters != nil {
		counters.due.Store(int64(len(monitors)))
		counters.started.Store(0)
		counters.inFlight.Store(0)
		defer func() {
			counters.due.Store(0)
			counters.started.Store(0)
			counters.inFlight.Store(0)
		}()
	}

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, monitor := range monitors {
		wg.Add(1)
		sem <- struct{}{}
		if counters != nil {
			counters.started.Add(1)
			counters.inFlight.Add(1)
		}
		go func(m models.MonitorURL) {
			defer wg.Done()
			defer func() {
				<-sem
				if counters != nil {
					counters.inFlight.Add(-1)
				}
			}()
			checkFn(m)
		}(monitor)
	}

	wg.Wait()
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

func (w *MonitorWorker) checkMonitor(monitor models.MonitorURL) {
	ctx, cancel := context.WithTimeout(context.Background(), urlcheck.RequestTimeout)
	defer cancel()

	displayName := models.MonitorDisplayName(monitor)
	result := urlcheck.Probe(ctx, w.client, monitor.URL)
	elapsed := int(result.DurationMs)

	if !result.Up {
		w.recordMonitorRequest(displayName, monitor.URL, result.StatusCode, result.DurationMs, false, result.ErrMsg)
		w.resultJobs <- checkResult{monitor: monitor, isUp: false, errMsg: result.ErrMsg, elapsed: intPtr(elapsed)}
		return
	}

	w.recordMonitorRequest(displayName, monitor.URL, result.StatusCode, result.DurationMs, true, "")
	w.resultJobs <- checkResult{monitor: monitor, isUp: true, errMsg: "", elapsed: intPtr(elapsed)}
}

func (w *MonitorWorker) recordMonitorRequest(monitorName, url string, statusCode int, responseTimeMs int64, isUp bool, errMsg string) {
	applog.AddMonitorRequest(monitorName, url, statusCode, responseTimeMs, isUp, errMsg)
}

func intPtr(v int) *int {
	return &v
}
