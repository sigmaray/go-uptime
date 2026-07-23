package worker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
)

// browserLikeUserAgent mimics a common desktop Chrome browser so WAF / bot filters
// are less likely to reject checks solely because of an obvious monitor User-Agent.
const browserLikeUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// browserLikeAccept is a typical browser Accept header for a top-level document navigation.
const browserLikeAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"

// browserLikeAcceptLanguage prefers English and Russian so localized sites still treat the
// request as a normal browser visit from an international client.
const browserLikeAcceptLanguage = "en-US,en;q=0.9,ru;q=0.8"

// runDueMonitors checks monitors whose individual interval has elapsed and runs periodic maintenance.
func (w *MonitorWorker) runDueMonitors() {
	if w.Paused() {
		return
	}

	var monitors []models.MonitorURL
	if err := w.db.Find(&monitors).Error; err != nil {
		log.Error().Err(err).Msg("failed to load monitor urls")
		applog.AddError("failed to load monitor urls", err.Error())
		return
	}

	now := time.Now()
	globalIntervalSeconds := models.GetCheckIntervalSeconds(w.db)

	due := make([]models.MonitorURL, 0)
	for _, monitor := range monitors {
		intervalSeconds := models.MonitorCheckIntervalSeconds(monitor, globalIntervalSeconds)
		if !IsMonitorDue(monitor.LastCheckedAt, time.Duration(intervalSeconds)*time.Second, now) {
			continue
		}
		due = append(due, monitor)
	}

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
	setBrowserLikeHeaders(req)

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

// setBrowserLikeHeaders attaches request headers that resemble a normal browser GET.
// req is the outbound monitor check request that will be sent by the HTTP client.
// This reduces false downs from simple bot filters that reject custom monitor User-Agents.
// Meta properties (for example WhatsApp) return HTTP 400 for a Chrome User-Agent without
// Sec-Fetch-* navigation headers, so those are included as well.
func setBrowserLikeHeaders(req *http.Request) {
	req.Header.Set("User-Agent", browserLikeUserAgent)
	req.Header.Set("Accept", browserLikeAccept)
	req.Header.Set("Accept-Language", browserLikeAcceptLanguage)
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
}

func intPtr(v int) *int {
	return &v
}
