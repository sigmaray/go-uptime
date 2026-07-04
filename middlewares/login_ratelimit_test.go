package middlewares

import (
	"testing"
	"time"
)

func TestLoginAttemptTrackerAllow(t *testing.T) {
	tracker := newLoginAttemptTracker()
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	ip := "203.0.113.10"

	for i := 0; i < loginRateLimitMaxAttempts; i++ {
		if !tracker.allow(ip, now) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}

	if tracker.allow(ip, now) {
		t.Fatal("expected rate limit to block further attempts in the same window")
	}

	later := now.Add(loginRateLimitWindow + time.Second)
	if !tracker.allow(ip, later) {
		t.Fatal("expected attempts to be allowed after the window expires")
	}
}

func TestLoginAttemptTrackerIsolatesIPs(t *testing.T) {
	tracker := newLoginAttemptTracker()
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	for i := 0; i < loginRateLimitMaxAttempts; i++ {
		if !tracker.allow("203.0.113.1", now) {
			t.Fatalf("first IP attempt %d should be allowed", i+1)
		}
	}

	if !tracker.allow("203.0.113.2", now) {
		t.Fatal("expected separate IP to have its own attempt budget")
	}
}
