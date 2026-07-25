package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/internal/urlcheck"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
)

// runDueMonitors checks monitors whose individual interval has elapsed and runs periodic maintenance.
func (w *MonitorWorker) runDueMonitors() {
	if w.Paused() {
		return
	}

	globalIntervalSeconds := models.GetCheckIntervalSeconds(w.db)

	var due []models.MonitorURL
	if err := w.db.Where("last_checked_at IS NULL OR last_checked_at <= NOW() - make_interval(secs := COALESCE(check_interval_seconds, ?))", globalIntervalSeconds).Find(&due).Error; err != nil {
		log.Error().Err(err).Msg("failed to load due monitor urls")
		applog.AddError("failed to load due monitor urls", err.Error())
		return
	}

	now := time.Now()
	runChecksConcurrently(due, w.checkConcurrency, w.checkMonitor, &checkWaveCounters{
		due:      &w.waveDue,
		started:  &w.waveStarted,
		inFlight: &w.inFlight,
	})

	w.runMaintenanceIfDue(now)
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
		w.markDown(monitor, result.ErrMsg, intPtr(elapsed))
		return
	}

	w.recordMonitorRequest(displayName, monitor.URL, result.StatusCode, result.DurationMs, true, "")
	w.markUp(monitor, time.Now(), intPtr(elapsed))
}

func (w *MonitorWorker) recordMonitorRequest(monitorName, url string, statusCode int, responseTimeMs int64, isUp bool, errMsg string) {
	applog.AddMonitorRequest(monitorName, url, statusCode, responseTimeMs, isUp, errMsg)
}

func intPtr(v int) *int {
	return &v
}
