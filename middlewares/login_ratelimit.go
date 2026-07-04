package middlewares

import (
	"os"
	"sync"
	"time"
)

const (
	loginRateLimitMaxAttempts = 10
	loginRateLimitWindow      = time.Minute
)

type loginAttemptTracker struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

// newLoginAttemptTracker creates an in-memory tracker for login attempts per client IP.
func newLoginAttemptTracker() *loginAttemptTracker {
	return &loginAttemptTracker{
		attempts: make(map[string][]time.Time),
	}
}

// allow reports whether another login attempt is allowed for the given client IP.
func (t *loginAttemptTracker) allow(clientIP string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := now.Add(-loginRateLimitWindow)
	recent := t.attempts[clientIP][:0]
	for _, ts := range t.attempts[clientIP] {
		if ts.After(cutoff) {
			recent = append(recent, ts)
		}
	}

	if len(recent) >= loginRateLimitMaxAttempts {
		t.attempts[clientIP] = recent
		return false
	}

	t.attempts[clientIP] = append(recent, now)
	return true
}

var defaultLoginAttemptTracker = newLoginAttemptTracker()

// AllowLoginAttempt reports whether another login attempt is allowed for clientIP.
func AllowLoginAttempt(clientIP string) bool {
	if os.Getenv("GO_UPTIME_ENABLE_PLAYWRIGHT_API") == "true" {
		return true
	}
	return defaultLoginAttemptTracker.allow(clientIP, time.Now())
}
