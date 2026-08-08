package middlewares

import (
	"testing"
	"time"
)

func TestLoginAttemptTrackerAllow(t *testing.T) {
	// Arrange: свежий tracker, фиксированное время и IP — без гонок и зависимости от «сейчас».
	tracker := newLoginAttemptTracker()
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	ip := "203.0.113.10"

	// Act + Assert: первые loginRateLimitMaxAttempts попыток в одном окне должны проходить.
	for i := 0; i < loginRateLimitMaxAttempts; i++ {
		if !tracker.allow(ip, now) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}

	// Assert: (loginRateLimitMaxAttempts+1)-я попытка в том же окне блокируется.
	if tracker.allow(ip, now) {
		t.Fatal("expected rate limit to block further attempts in the same window")
	}

	// Act: сдвигаем время за пределы окна rate limit.
	later := now.Add(loginRateLimitWindow + time.Second)
	// Assert: после истечения окна счётчик для IP сбрасывается и попытка снова разрешена.
	if !tracker.allow(ip, later) {
		t.Fatal("expected attempts to be allowed after the window expires")
	}
}

func TestLoginAttemptTrackerIsolatesIPs(t *testing.T) {
	// Arrange: один tracker, два разных IP; лимит одного не должен влиять на другой.
	tracker := newLoginAttemptTracker()
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	// Act: исчерпываем лимит для первого IP.
	for i := 0; i < loginRateLimitMaxAttempts; i++ {
		if !tracker.allow("203.0.113.1", now) {
			t.Fatalf("first IP attempt %d should be allowed", i+1)
		}
	}

	// Assert: второй IP имеет отдельный бюджет попыток в том же временном окне.
	if !tracker.allow("203.0.113.2", now) {
		t.Fatal("expected separate IP to have its own attempt budget")
	}
}
