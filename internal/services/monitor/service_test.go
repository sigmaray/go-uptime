package monitor

import (
	"testing"

	"go-uptime/internal/urlcheck"
)

func TestMonitorURLExistsMessage(t *testing.T) {
	// Табличный тест URLExistsMessage: generic vs список duplicate URL в flash/error.
	tests := []struct {
		name string
		urls []string
		want string
	}{
		{
			name: "no urls",
			urls: nil,
			want: ErrMonitorURLExists,
		},
		{
			name: "blank urls",
			urls: []string{"", "  "},
			want: ErrMonitorURLExists,
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
			// Act + Assert: nil/blank → константа ErrMonitorURLExists; иначе — перечисление URL.
			got := URLExistsMessage(tt.urls...)
			if got != tt.want {
				t.Fatalf("URLExistsMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMonitorUnavailableMessage(t *testing.T) {
	// Табличный тест UnavailableMessage для bulk create когда probe failed.
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
			// Act + Assert: singular/plural «Site»/«Sites» и формат «url (err)».
			got := UnavailableMessage(tt.failures)
			if got != tt.want {
				t.Fatalf("UnavailableMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExcludeURLs(t *testing.T) {
	// Табличный тест ExcludeURLs: set subtraction с сохранением порядка.
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
			// Act.
			got := ExcludeURLs(tt.urls, tt.exclude)
			// Assert: длина и поэлементное совпадение (nil vs []{} различать не нужно для len).
			if len(got) != len(tt.want) {
				t.Fatalf("ExcludeURLs() len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("ExcludeURLs() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
