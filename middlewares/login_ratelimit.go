package middlewares

import (
	"os"
	"sync"
	"time"
)

// Скользящее окно: не больше 10 попыток входа в минуту на IP клиента.
const (
	loginRateLimitMaxAttempts = 10
	loginRateLimitWindow      = time.Minute
)

// loginAttemptTracker хранит timestamps попыток входа по IP для sliding window (10/min).
type loginAttemptTracker struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

// newLoginAttemptTracker создаёт in-memory трекер попыток входа по IP клиента.
func newLoginAttemptTracker() *loginAttemptTracker {
	return &loginAttemptTracker{
		attempts: make(map[string][]time.Time),
	}
}

// allow сообщает, разрешена ли ещё одна попытка входа для данного IP клиента.
func (t *loginAttemptTracker) allow(clientIP string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Граница окна: всё, что старше cutoff, больше не учитывается.
	cutoff := now.Add(-loginRateLimitWindow)
	// [:0] переиспользует backing array слайса: фильтруем устаревшие метки без новой аллокации.
	recent := t.attempts[clientIP][:0]
	for _, ts := range t.attempts[clientIP] {
		if ts.After(cutoff) {
			recent = append(recent, ts)
		}
	}

	if len(recent) >= loginRateLimitMaxAttempts {
		// Лимит исчерпан: сохраняем только «живые» попытки и отклоняем новую.
		t.attempts[clientIP] = recent
		return false
	}

	// Укладываемся в лимит — фиксируем текущую попытку в окне.
	t.attempts[clientIP] = append(recent, now)
	return true
}

var defaultLoginAttemptTracker = newLoginAttemptTracker()

// AllowLoginAttempt сообщает, разрешена ли ещё одна попытка входа для clientIP.
func AllowLoginAttempt(clientIP string) bool {
	// Обход rate limit для e2e/Playwright API — автотесты не должны упираться в лимит 10/min.
	if os.Getenv("GO_UPTIME_ENABLE_PLAYWRIGHT_API") == "true" {
		return true
	}
	return defaultLoginAttemptTracker.allow(clientIP, time.Now())
}
