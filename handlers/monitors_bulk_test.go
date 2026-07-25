package handlers

import (
	"testing"

	"go-uptime/internal/urlcheck"
)

func TestMonitorURLExistsMessage(t *testing.T) {
	tests := []struct {
		name string
		urls []string
		want string
	}{
		{
			name: "no urls",
			urls: nil,
			want: errMonitorURLExists,
		},
		{
			name: "blank urls",
			urls: []string{"", "  "},
			want: errMonitorURLExists,
		},
		{
			name: "one url",
			urls: []string{"https://dup.example.com"},
			want: "A monitor with this URL already exists: https://dup.example.com",
		},
		{
			name: "multiple urls",
			urls: []string{"https://a.example.com", "https://b.example.com"},
			want: "A monitor with this URL already exists: https://a.example.com, https://b.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitorURLExistsMessage(tt.urls...)
			if got != tt.want {
				t.Fatalf("monitorURLExistsMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMonitorUnavailableMessage(t *testing.T) {
	tests := []struct {
		name     string
		failures []urlcheck.Result
		want     string
	}{
		{
			name:     "empty",
			failures: nil,
			want:     "Site is unavailable and was not created",
		},
		{
			name: "one failure",
			failures: []urlcheck.Result{
				{URL: "https://down.example.com", ErrMsg: "connection refused"},
			},
			want: "Site is unavailable and was not created: https://down.example.com (connection refused)",
		},
		{
			name: "multiple failures",
			failures: []urlcheck.Result{
				{URL: "https://a.example.com", ErrMsg: "unexpected status code: 503"},
				{URL: "https://b.example.com", ErrMsg: "timeout"},
			},
			want: "Sites are unavailable and were not created: https://a.example.com (unexpected status code: 503); https://b.example.com (timeout)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitorUnavailableMessage(tt.failures)
			if got != tt.want {
				t.Fatalf("monitorUnavailableMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExcludeURLs(t *testing.T) {
	tests := []struct {
		name    string
		urls    []string
		exclude []string
		want    []string
	}{
		{
			name:    "nil inputs",
			urls:    nil,
			exclude: nil,
			want:    nil,
		},
		{
			name:    "empty exclude keeps urls",
			urls:    []string{"https://a.example.com", "https://b.example.com"},
			exclude: nil,
			want:    []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name:    "removes matching urls and preserves order",
			urls:    []string{"https://a.example.com", "https://b.example.com", "https://c.example.com"},
			exclude: []string{"https://b.example.com"},
			want:    []string{"https://a.example.com", "https://c.example.com"},
		},
		{
			name:    "all excluded yields empty slice",
			urls:    []string{"https://a.example.com"},
			exclude: []string{"https://a.example.com"},
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := excludeURLs(tt.urls, tt.exclude)
			if len(got) != len(tt.want) {
				t.Fatalf("excludeURLs() len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("excludeURLs() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
