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
