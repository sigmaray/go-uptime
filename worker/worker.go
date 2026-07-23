// Package worker performs background HTTP checks of monitored URLs.
package worker

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go-uptime/config"
	"go-uptime/internal/applog"
	"go-uptime/internal/notify"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const requestTimeout = 15 * time.Second

// defaultCheckConcurrency is used when config omits or sets an invalid concurrency value.
const defaultCheckConcurrency = 50

// notifyQueueSize is the buffered capacity for async status-change alerts.
const notifyQueueSize = 256

// browserLikeUserAgent mimics a common desktop Chrome browser so WAF / bot filters
// are less likely to reject checks solely because of an obvious monitor User-Agent.
const browserLikeUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// browserLikeAccept is a typical browser Accept header for a top-level document navigation.
const browserLikeAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"

// browserLikeAcceptLanguage prefers English and Russian so localized sites still treat the
// request as a normal browser visit from an international client.
const browserLikeAcceptLanguage = "en-US,en;q=0.9,ru;q=0.8"

// notifyJob is one monitor status-change alert to send off the check path.
type notifyJob struct {
	monitor models.MonitorURL
	isUp    bool
	errMsg  string
}

// MonitorWorker periodically checks URLs from the database.
type MonitorWorker struct {
	db               *gorm.DB
	cfg              *config.Config
	client           *http.Client
	checkConcurrency int
	notifyJobs       chan notifyJob
	// notifySender delivers one alert; nil means the default Shoutrrr path.
	notifySender func(monitor models.MonitorURL, isUp bool, errMsg string)

	stop       chan struct{}
	loopDone   chan struct{}
	notifyDone chan struct{}
	started    atomic.Bool
	stopOnce   sync.Once

	lastMaintenanceAt time.Time
}

// New creates a new background monitoring worker instance.
// db is the GORM handle used to load monitors and persist check results.
// cfg supplies retention settings and the maximum number of concurrent HTTP checks.
func New(db *gorm.DB, cfg *config.Config) *MonitorWorker {
	concurrency := defaultCheckConcurrency
	if cfg != nil && cfg.CheckConcurrency > 0 {
		concurrency = cfg.CheckConcurrency
	}

	return &MonitorWorker{
		db:               db,
		cfg:              cfg,
		checkConcurrency: concurrency,
		client: &http.Client{
			Timeout:   requestTimeout,
			Transport: newCheckTransport(concurrency),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		notifyJobs: make(chan notifyJob, notifyQueueSize),
		stop:       make(chan struct{}),
		loopDone:   make(chan struct{}),
		notifyDone: make(chan struct{}),
	}
}

// newCheckTransport builds an HTTP transport sized for concurrent monitor checks.
// maxConcurrent is the configured check concurrency used to size connection pools.
func newCheckTransport(maxConcurrent int) *http.Transport {
	idleConns := maxConcurrent * 2
	if idleConns < 100 {
		idleConns = 100
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          idleConns,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// Start runs the check loop and async notification sender in separate goroutines.
func (w *MonitorWorker) Start() {
	if !w.started.CompareAndSwap(false, true) {
		return
	}
	go w.backfillUptimeStatsIfNeeded()
	go w.notifyLoop()
	go w.loop()
}

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

// Stop stops the check loop, drains queued notifications, then returns.
func (w *MonitorWorker) Stop() {
	w.stopOnce.Do(func() {
		if !w.started.Load() {
			return
		}
		close(w.stop)
		<-w.loopDone
		close(w.notifyJobs)
		<-w.notifyDone
	})
}

func (w *MonitorWorker) loop() {
	defer close(w.loopDone)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	w.runDueMonitors()

	for {
		select {
		case <-ticker.C:
			w.runDueMonitors()
		case <-w.stop:
			return
		}
	}
}

// notifyLoop sends queued status-change alerts so SMTP/Telegram never block HTTP checks.
func (w *MonitorWorker) notifyLoop() {
	defer close(w.notifyDone)

	for job := range w.notifyJobs {
		w.deliverNotification(job.monitor, job.isUp, job.errMsg)
	}
}

// enqueueNotification queues a status-change alert without waiting for delivery.
// monitor is the monitor that changed state; isUp is the new availability;
// errMsg is the down reason (empty when the monitor recovered).
func (w *MonitorWorker) enqueueNotification(monitor models.MonitorURL, isUp bool, errMsg string) {
	if !monitor.NotifyTelegram && !monitor.NotifySMTP {
		return
	}

	job := notifyJob{
		monitor: monitor,
		isUp:    isUp,
		errMsg:  errMsg,
	}
	select {
	case w.notifyJobs <- job:
	default:
		log.Warn().
			Uint("monitor_id", monitor.ID).
			Msg("notification queue full, dropping alert")
		applog.AddError(
			"notification queue full",
			fmt.Sprintf("monitor_id=%d dropped alert", monitor.ID),
		)
	}
}

// deliverNotification sends one queued alert via the injected sender or the default path.
// monitor is the monitor that changed state; isUp is the new availability;
// errMsg is the down reason (empty when the monitor recovered).
func (w *MonitorWorker) deliverNotification(monitor models.MonitorURL, isUp bool, errMsg string) {
	if w.notifySender != nil {
		w.notifySender(monitor, isUp, errMsg)
		return
	}
	w.sendNotifications(monitor, isUp, errMsg)
}

// runDueMonitors checks monitors whose individual interval has elapsed and runs periodic maintenance.
func (w *MonitorWorker) runDueMonitors() {
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
		if !isMonitorDue(monitor.LastCheckedAt, time.Duration(intervalSeconds)*time.Second, now) {
			continue
		}
		due = append(due, monitor)
	}

	runChecksConcurrently(due, w.checkConcurrency, w.checkMonitor)

	w.runMaintenanceIfDue(now)
}

// runChecksConcurrently runs checkFn for each monitor with at most maxConcurrent goroutines.
// monitors is the list of due monitors to check in this wave.
// maxConcurrent caps how many HTTP checks may run at the same time; values below 1 become 1.
// checkFn performs one monitor check and must be safe to call concurrently.
func runChecksConcurrently(monitors []models.MonitorURL, maxConcurrent int, checkFn func(models.MonitorURL)) {
	if len(monitors) == 0 {
		return
	}
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, monitor := range monitors {
		wg.Add(1)
		sem <- struct{}{}
		go func(m models.MonitorURL) {
			defer wg.Done()
			defer func() { <-sem }()
			checkFn(m)
		}(monitor)
	}

	wg.Wait()
}

// isMonitorDue reports whether a monitor should be checked at now based on its last check time.
// lastCheckedAt is nil when the monitor has never been checked.
// interval is the effective check interval for the monitor.
// now is the current time used for the due calculation.
func isMonitorDue(lastCheckedAt *time.Time, interval time.Duration, now time.Time) bool {
	if lastCheckedAt == nil {
		return true
	}
	return now.Sub(*lastCheckedAt) >= interval
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
func setBrowserLikeHeaders(req *http.Request) {
	req.Header.Set("User-Agent", browserLikeUserAgent)
	req.Header.Set("Accept", browserLikeAccept)
	req.Header.Set("Accept-Language", browserLikeAcceptLanguage)
}

func intPtr(v int) *int {
	return &v
}

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

// sendNotifications sends status-change notifications when channels are configured and enabled for the monitor.
// monitor is the monitor that changed; isUp is the new state; errMsg explains a down transition.
func (w *MonitorWorker) sendNotifications(monitor models.MonitorURL, isUp bool, errMsg string) {
	if !monitor.NotifyTelegram && !monitor.NotifySMTP {
		return
	}

	settings, err := models.LoadNotificationSettings(w.db)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to load notification settings")
		applog.AddError("failed to load notification settings", err.Error())
		return
	}

	change := notify.MonitorStateChange{
		DisplayName: models.MonitorDisplayName(monitor),
		URL:         monitor.URL,
		IsUp:        isUp,
		Error:       errMsg,
	}
	if err := notify.SendMonitorStateChange(settings, monitor, change); err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to send monitor notification")
		applog.AddError("failed to send monitor notification", err.Error())
	}
}
