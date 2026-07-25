package handlers

import (
	"testing"
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
