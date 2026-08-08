package urlcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsUpStatus(t *testing.T) {
	// Табличный тест: HTTP status codes 200..399 считаются «up» для мониторинга.
	tests := []struct {
		code int
		want bool
	}{
		{200, true},
		{201, true},
		{301, true},
		{399, true},
		{199, false},
		{400, false},
		{404, false},
		{500, false},
		{0, false},
	}
	for _, tt := range tests {
		// Act + Assert: границы диапазона и «нет ответа» (code=0).
		if got := IsUpStatus(tt.code); got != tt.want {
			t.Fatalf("IsUpStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestProbeUp(t *testing.T) {
	// Arrange: mock HTTP server, возвращающий 200 и проверяющий User-Agent.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected browser-like User-Agent")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewClient(2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Act: проба живого URL.
	result := Probe(ctx, client, server.URL)
	// Assert: Up=true, StatusCode=200.
	if !result.Up {
		t.Fatalf("Probe() Up=false, ErrMsg=%q", result.ErrMsg)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode=%d, want 200", result.StatusCode)
	}
}

func TestProbeCapsResponseBodyDrain(t *testing.T) {
	// Arrange: сервер отдаёт тело больше MaxProbeBodyBytes — drain должен обрезаться.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, MaxProbeBodyBytes+4096))
	}))
	defer server.Close()

	client := NewClient(1)
	start := time.Now()
	// Act: Probe не должен читать весь multi-MB body.
	result := Probe(context.Background(), client, server.URL)
	// Assert: статус всё равно up (200).
	if !result.Up {
		t.Fatalf("Probe() Up=false, ErrMsg=%q", result.ErrMsg)
	}
	// Assert: завершение быстрое — cap на drain работает.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Probe took %v; body drain should be capped", elapsed)
	}
}

func TestProbeDownStatus(t *testing.T) {
	// Arrange: сервер возвращает 503 Service Unavailable.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(1)
	// Act.
	result := Probe(context.Background(), client, server.URL)
	// Assert: 503 → Up=false.
	if result.Up {
		t.Fatal("expected Probe() Up=false for 503")
	}
	if result.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("StatusCode=%d, want 503", result.StatusCode)
	}
	// Assert: ErrMsg заполнен для UI/уведомлений.
	if result.ErrMsg == "" {
		t.Fatal("expected ErrMsg for down status")
	}
}

func TestProbeAllAndUnavailableURLs(t *testing.T) {
	// Arrange: два сервера — один 200, другой 404.
	upServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upServer.Close()

	downServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer downServer.Close()

	urls := []string{upServer.URL, downServer.URL}
	// Act: параллельный ProbeAll с concurrency=2.
	results := ProbeAll(context.Background(), NewClient(2), urls, 2)
	// Assert: по одному результату на URL.
	if len(results) != 2 {
		t.Fatalf("len(results)=%d, want 2", len(results))
	}
	// Assert: первый up, второй down.
	if !results[0].Up || results[1].Up {
		t.Fatalf("unexpected up flags: %+v", results)
	}

	// Act: фильтр только недоступных URL для bulk-create validation.
	failures := UnavailableURLs(results)
	// Assert: ровно один failure — down server.
	if len(failures) != 1 || failures[0].URL != downServer.URL {
		t.Fatalf("UnavailableURLs() = %+v", failures)
	}
}

func TestSetBrowserLikeHeaders(t *testing.T) {
	// Arrange: пустой GET request.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	// Act: подставляем browser-like headers для обхода простых bot-filter.
	SetBrowserLikeHeaders(req)
	// Assert: фиксированный User-Agent из пакета.
	if got := req.Header.Get("User-Agent"); got != browserLikeUserAgent {
		t.Fatalf("User-Agent=%q, want %q", got, browserLikeUserAgent)
	}
	// Assert: Sec-Fetch-Mode=navigate — как у настоящего браузера.
	if got := req.Header.Get("Sec-Fetch-Mode"); got != "navigate" {
		t.Fatalf("Sec-Fetch-Mode=%q, want navigate", got)
	}
}
