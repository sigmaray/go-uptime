package worker

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-uptime/config"
	"go-uptime/models"
)

func TestSetBrowserLikeHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	setBrowserLikeHeaders(req)

	want := map[string]string{
		"User-Agent":                browserLikeUserAgent,
		"Accept":                    browserLikeAccept,
		"Accept-Language":           browserLikeAcceptLanguage,
		"Upgrade-Insecure-Requests": "1",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
	}
	for name, expected := range want {
		if got := req.Header.Get(name); got != expected {
			t.Fatalf("%s = %q, want %q", name, got, expected)
		}
	}
}

func TestShouldNotifyStateChange(t *testing.T) {
	up := true
	down := false

	tests := []struct {
		name     string
		previous *bool
		nowUp    bool
		want     bool
	}{
		{
			name:     "first check up",
			previous: nil,
			nowUp:    true,
			want:     false,
		},
		{
			name:     "first check down",
			previous: nil,
			nowUp:    false,
			want:     false,
		},
		{
			name:     "down to up",
			previous: &down,
			nowUp:    true,
			want:     true,
		},
		{
			name:     "up to down",
			previous: &up,
			nowUp:    false,
			want:     true,
		},
		{
			name:     "still up",
			previous: &up,
			nowUp:    true,
			want:     false,
		},
		{
			name:     "still down",
			previous: &down,
			nowUp:    false,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldNotifyStateChange(tt.previous, tt.nowUp); got != tt.want {
				t.Fatalf("shouldNotifyStateChange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsMonitorDue(t *testing.T) {
	interval := time.Minute
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	t.Run("never checked", func(t *testing.T) {
		if !IsMonitorDue(nil, interval, now) {
			t.Fatal("expected first check to be due")
		}
	})

	t.Run("interval not elapsed", func(t *testing.T) {
		last := now.Add(-30 * time.Second)
		if IsMonitorDue(&last, interval, now) {
			t.Fatal("expected monitor to be skipped")
		}
	})

	t.Run("interval elapsed", func(t *testing.T) {
		last := now.Add(-time.Minute)
		if !IsMonitorDue(&last, interval, now) {
			t.Fatal("expected monitor to be due")
		}
	})
}

func TestRunChecksConcurrentlyRespectsLimit(t *testing.T) {
	const (
		monitorCount  = 20
		maxConcurrent = 3
		checkDuration = 30 * time.Millisecond
	)

	monitors := make([]models.MonitorURL, monitorCount)
	for i := range monitors {
		monitors[i].ID = uint(i + 1)
	}

	var (
		inFlight atomic.Int32
		peak     atomic.Int32
		checked  atomic.Int32
		mu       sync.Mutex
		seen     = make(map[uint]bool)
	)

	start := time.Now()
	runChecksConcurrently(monitors, maxConcurrent, func(m models.MonitorURL) {
		current := inFlight.Add(1)
		for {
			prev := peak.Load()
			if current <= prev || peak.CompareAndSwap(prev, current) {
				break
			}
		}

		time.Sleep(checkDuration)

		inFlight.Add(-1)
		checked.Add(1)

		mu.Lock()
		seen[m.ID] = true
		mu.Unlock()
	}, nil)
	elapsed := time.Since(start)

	if got := int(checked.Load()); got != monitorCount {
		t.Fatalf("checked %d monitors, want %d", got, monitorCount)
	}
	if got := int(peak.Load()); got > maxConcurrent {
		t.Fatalf("peak concurrency %d exceeds limit %d", got, maxConcurrent)
	}
	if len(seen) != monitorCount {
		t.Fatalf("saw %d unique monitors, want %d", len(seen), monitorCount)
	}

	// Serial would take monitorCount * checkDuration; with concurrency 3 it should be much faster.
	serialFloor := time.Duration(monitorCount) * checkDuration
	if elapsed >= serialFloor {
		t.Fatalf("elapsed %v looks serial (serial floor %v)", elapsed, serialFloor)
	}
}

func TestRunChecksConcurrentlyEmptyAndInvalidLimit(t *testing.T) {
	runChecksConcurrently(nil, 5, func(models.MonitorURL) {
		t.Fatal("checkFn must not run for empty list")
	}, nil)

	var ran atomic.Int32
	runChecksConcurrently([]models.MonitorURL{{ID: 1}}, 0, func(models.MonitorURL) {
		ran.Add(1)
	}, nil)
	if ran.Load() != 1 {
		t.Fatalf("expected single check with invalid limit, got %d", ran.Load())
	}
}

func TestStatsTracksWaveProgress(t *testing.T) {
	w := New(nil, &config.Config{CheckConcurrency: 2})

	release := make(chan struct{})
	started := make(chan struct{}, 4)
	monitors := []models.MonitorURL{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runChecksConcurrently(monitors, w.checkConcurrency, func(models.MonitorURL) {
			started <- struct{}{}
			<-release
		}, &checkWaveCounters{
			due:      &w.waveDue,
			started:  &w.waveStarted,
			inFlight: &w.inFlight,
		})
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for in-flight checks")
		}
	}

	stats := w.Stats()
	if stats.DueThisWave != 4 {
		t.Fatalf("DueThisWave = %d, want 4", stats.DueThisWave)
	}
	if stats.InFlight != 2 {
		t.Fatalf("InFlight = %d, want 2", stats.InFlight)
	}
	if stats.WaitingForSlot != 2 {
		t.Fatalf("WaitingForSlot = %d, want 2", stats.WaitingForSlot)
	}
	if stats.MaxConcurrency != 2 {
		t.Fatalf("MaxConcurrency = %d, want 2", stats.MaxConcurrency)
	}
	if stats.NotifyCapacity != notifyQueueSize {
		t.Fatalf("NotifyCapacity = %d, want %d", stats.NotifyCapacity, notifyQueueSize)
	}
	if stats.NotifyQueued != 0 {
		t.Fatalf("NotifyQueued = %d, want 0", stats.NotifyQueued)
	}

	close(release)
	wg.Wait()

	stats = w.Stats()
	if stats.DueThisWave != 0 || stats.InFlight != 0 || stats.WaitingForSlot != 0 {
		t.Fatalf("expected idle stats after wave, got %+v", stats)
	}
}

func TestStatsReportsNotifyQueueDepth(t *testing.T) {
	w := New(nil, &config.Config{})
	w.enqueueNotification(models.MonitorURL{ID: 1, NotifySMTP: true}, false, "down")
	stats := w.Stats()
	if stats.NotifyQueued != 1 {
		t.Fatalf("NotifyQueued = %d, want 1", stats.NotifyQueued)
	}
}

func TestNewUsesConfiguredCheckConcurrency(t *testing.T) {
	cfg := &config.Config{CheckConcurrency: 7}
	w := New(nil, cfg)
	if w.checkConcurrency != 7 {
		t.Fatalf("checkConcurrency = %d, want 7", w.checkConcurrency)
	}

	w = New(nil, &config.Config{CheckConcurrency: 0})
	if w.checkConcurrency != defaultCheckConcurrency {
		t.Fatalf("checkConcurrency = %d, want default %d", w.checkConcurrency, defaultCheckConcurrency)
	}
}

func TestEnqueueNotificationSkipsDisabledChannels(t *testing.T) {
	w := New(nil, &config.Config{})
	w.enqueueNotification(models.MonitorURL{ID: 1}, false, "down")
	if len(w.notifyJobs) != 0 {
		t.Fatalf("expected empty queue, got %d jobs", len(w.notifyJobs))
	}
}

func TestEnqueueNotificationDoesNotBlockWhenQueueFull(t *testing.T) {
	w := New(nil, &config.Config{})
	for i := 0; i < notifyQueueSize; i++ {
		w.notifyJobs <- notifyJob{monitor: models.MonitorURL{ID: uint(i)}}
	}

	start := time.Now()
	w.enqueueNotification(models.MonitorURL{ID: 999, NotifySMTP: true}, false, "timeout")
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("enqueue blocked for %v on full queue", elapsed)
	}
	if len(w.notifyJobs) != notifyQueueSize {
		t.Fatalf("queue length = %d, want %d", len(w.notifyJobs), notifyQueueSize)
	}
}

func TestNotifyLoopDeliversQueuedJobs(t *testing.T) {
	w := New(nil, &config.Config{CheckConcurrency: 1})
	delivered := make(chan notifyJob, 1)
	w.notifySender = func(monitor models.MonitorURL, isUp bool, errMsg string) {
		delivered <- notifyJob{monitor: monitor, isUp: isUp, errMsg: errMsg}
	}
	go w.notifyLoop()

	monitor := models.MonitorURL{
		ID:             42,
		URL:            "https://example.com",
		NotifyTelegram: true,
	}
	w.enqueueNotification(monitor, false, "timeout")

	select {
	case job := <-delivered:
		if job.monitor.ID != 42 || job.isUp || job.errMsg != "timeout" {
			t.Fatalf("unexpected job: %+v", job)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification delivery")
	}

	close(w.notifyJobs)
	select {
	case <-w.notifyDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notifyLoop to exit")
	}
}

func TestEnqueueNotificationDoesNotWaitForSlowSender(t *testing.T) {
	w := New(nil, &config.Config{CheckConcurrency: 1})
	var calls atomic.Int32
	started := make(chan struct{}, 2)
	blockFirst := make(chan struct{})
	w.notifySender = func(models.MonitorURL, bool, string) {
		n := calls.Add(1)
		started <- struct{}{}
		if n == 1 {
			<-blockFirst
		}
	}
	go w.notifyLoop()

	monitor := models.MonitorURL{ID: 1, NotifyTelegram: true}
	w.enqueueNotification(monitor, false, "first")

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("sender did not start")
	}

	start := time.Now()
	w.enqueueNotification(monitor, true, "")
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("enqueue waited %v for busy sender", elapsed)
	}

	close(blockFirst)
	close(w.notifyJobs)
	select {
	case <-w.notifyDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notifyLoop to exit")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("delivered %d jobs, want 2", got)
	}
}
