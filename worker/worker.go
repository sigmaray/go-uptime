// Package worker performs background HTTP checks of monitored URLs.
package worker

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go-uptime/config"
	"go-uptime/internal/urlcheck"
	"go-uptime/models"

	"gorm.io/gorm"
)

// DefaultCheckConcurrency is used when config omits or sets an invalid concurrency value.
const DefaultCheckConcurrency = 150

// notifyQueueSize is the buffered capacity for async status-change alerts.
const notifyQueueSize = 256

// Stats is a point-in-time snapshot of live monitor-check and notify-queue metrics.
type Stats struct {
	// DueThisWave is how many monitors were selected for the current check wave.
	DueThisWave int
	// InFlight is how many HTTP checks are executing right now.
	InFlight int
	// WaitingForSlot is how many due monitors still wait for a concurrency slot in this wave.
	WaitingForSlot int
	// MaxConcurrency is the configured concurrent HTTP check limit.
	MaxConcurrency int
	// NotifyQueued is how many status-change alerts sit in the notify channel.
	NotifyQueued int
	// NotifyCapacity is the notify channel buffer size.
	NotifyCapacity int
}

// checkResult represents the outcome of a single HTTP check that will be batched into the DB.
type checkResult struct {
	monitor models.MonitorURL
	isUp    bool
	errMsg  string
	elapsed *int
}

// MonitorWorker periodically checks URLs from the database.
type MonitorWorker struct {
	db               *gorm.DB
	cfg              *config.Config
	client           *http.Client
	checkConcurrency int
	// notifyJobs buffers status-change alerts to be sent via external channels (e.g. Telegram/SMTP).
	notifyJobs chan notifyJob
	// resultJobs buffers completed HTTP checks waiting to be persisted to the database in a batch.
	resultJobs chan checkResult
	// notifySender delivers one alert; nil means the default Shoutrrr path.
	notifySender func(monitor models.MonitorURL, isUp bool, errMsg string)

	stop       chan struct{}
	loopDone   chan struct{}
	notifyDone chan struct{}
	// batchDone is closed when the batchResultsLoop goroutine fully exits.
	batchDone chan struct{}
	started   atomic.Bool
	stopOnce  sync.Once
	// paused skips due-monitor checks and maintenance while leaving Running() true.
	// Used by the Playwright test API so e2e clears do not race in-flight HTTP checks.
	paused atomic.Bool

	// waveDue is the size of the current check wave; zero when idle between waves.
	waveDue atomic.Int64
	// waveStarted counts monitors that have acquired a concurrency slot in this wave.
	waveStarted atomic.Int64
	// inFlight counts HTTP checks currently executing.
	inFlight atomic.Int64

	lastMaintenanceAt time.Time
}

// New creates a new background monitoring worker instance.
// db is the GORM handle used to load monitors and persist check results.
// cfg supplies retention settings and the maximum number of concurrent HTTP checks.
func New(db *gorm.DB, cfg *config.Config) *MonitorWorker {
	concurrency := DefaultCheckConcurrency
	if cfg != nil && cfg.CheckConcurrency > 0 {
		concurrency = cfg.CheckConcurrency
	}

	return &MonitorWorker{
		db:               db,
		cfg:              cfg,
		checkConcurrency: concurrency,
		client:           urlcheck.NewClient(concurrency),
		notifyJobs:       make(chan notifyJob, notifyQueueSize),
		resultJobs:       make(chan checkResult, 2048),
		stop:             make(chan struct{}),
		loopDone:         make(chan struct{}),
		notifyDone:       make(chan struct{}),
		batchDone:        make(chan struct{}),
	}
}

// Running reports whether the monitor check loop is active and has not been stopped.
// It returns false for a nil worker, before Start, and after Stop begins shutting down.
// A paused worker still reports running so /health stays healthy during e2e.
func (w *MonitorWorker) Running() bool {
	if w == nil || !w.started.Load() {
		return false
	}
	select {
	case <-w.stop:
		return false
	default:
		return true
	}
}

// Pause stops scheduling new monitor checks and maintenance waves.
// In-flight checks from a wave that already started are not cancelled.
func (w *MonitorWorker) Pause() {
	if w == nil {
		return
	}
	w.paused.Store(true)
}

// Resume allows the check loop to schedule monitor checks again after Pause.
func (w *MonitorWorker) Resume() {
	if w == nil {
		return
	}
	w.paused.Store(false)
}

// Paused reports whether new check waves are currently suppressed.
func (w *MonitorWorker) Paused() bool {
	return w != nil && w.paused.Load()
}

// Stats returns live check-wave and notification-queue counters for ops pages.
func (w *MonitorWorker) Stats() Stats {
	if w == nil {
		return Stats{}
	}

	due := int(w.waveDue.Load())
	started := int(w.waveStarted.Load())
	inFlight := int(w.inFlight.Load())
	waiting := due - started
	if waiting < 0 {
		waiting = 0
	}

	return Stats{
		DueThisWave:    due,
		InFlight:       inFlight,
		WaitingForSlot: waiting,
		MaxConcurrency: w.checkConcurrency,
		NotifyQueued:   len(w.notifyJobs),
		NotifyCapacity: notifyQueueSize,
	}
}

// Start runs the check loop and async notification sender in separate goroutines.
func (w *MonitorWorker) Start() {
	if !w.started.CompareAndSwap(false, true) {
		return
	}
	go w.backfillUptimeStatsIfNeeded()
	go w.notifyLoop()
	go w.batchResultsLoop()
	go w.loop()
}

// Stop stops the check loop, drains queued notifications, then returns.
func (w *MonitorWorker) Stop() {
	w.stopOnce.Do(func() {
		if !w.started.Load() {
			return
		}
		close(w.stop)
		<-w.loopDone
		close(w.resultJobs)
		<-w.batchDone
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
