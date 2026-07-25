package urlcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsUpStatus(t *testing.T) {
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
		if got := IsUpStatus(tt.code); got != tt.want {
			t.Fatalf("IsUpStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestProbeUp(t *testing.T) {
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

	result := Probe(ctx, client, server.URL)
	if !result.Up {
		t.Fatalf("Probe() Up=false, ErrMsg=%q", result.ErrMsg)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode=%d, want 200", result.StatusCode)
	}
}

func TestProbeDownStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(1)
	result := Probe(context.Background(), client, server.URL)
	if result.Up {
		t.Fatal("expected Probe() Up=false for 503")
	}
	if result.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("StatusCode=%d, want 503", result.StatusCode)
	}
	if result.ErrMsg == "" {
		t.Fatal("expected ErrMsg for down status")
	}
}

func TestProbeAllAndUnavailableURLs(t *testing.T) {
	upServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upServer.Close()

	downServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer downServer.Close()

	urls := []string{upServer.URL, downServer.URL}
	results := ProbeAll(context.Background(), NewClient(2), urls, 2)
	if len(results) != 2 {
		t.Fatalf("len(results)=%d, want 2", len(results))
	}
	if !results[0].Up || results[1].Up {
		t.Fatalf("unexpected up flags: %+v", results)
	}

	failures := UnavailableURLs(results)
	if len(failures) != 1 || failures[0].URL != downServer.URL {
		t.Fatalf("UnavailableURLs() = %+v", failures)
	}
}

func TestSetBrowserLikeHeaders(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	SetBrowserLikeHeaders(req)
	if got := req.Header.Get("User-Agent"); got != browserLikeUserAgent {
		t.Fatalf("User-Agent=%q, want %q", got, browserLikeUserAgent)
	}
	if got := req.Header.Get("Sec-Fetch-Mode"); got != "navigate" {
		t.Fatalf("Sec-Fetch-Mode=%q, want navigate", got)
	}
}
