package worker

import (
	"testing"
	"time"
)

func TestShouldNotifyStateChange(t *testing.T) {
	up := true
	down := false

	tests := []struct {
		name     string
		previous *bool
		nowUp    bool
		want     bool
	}{
		{
			name:     "first check up",
			previous: nil,
			nowUp:    true,
			want:     false,
		},
		{
			name:     "first check down",
			previous: nil,
			nowUp:    false,
			want:     false,
		},
		{
			name:     "down to up",
			previous: &down,
			nowUp:    true,
			want:     true,
		},
		{
			name:     "up to down",
			previous: &up,
			nowUp:    false,
			want:     true,
		},
		{
			name:     "still up",
			previous: &up,
			nowUp:    true,
			want:     false,
		},
		{
			name:     "still down",
			previous: &down,
			nowUp:    false,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldNotifyStateChange(tt.previous, tt.nowUp); got != tt.want {
				t.Fatalf("shouldNotifyStateChange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsMonitorDue(t *testing.T) {
	interval := time.Minute
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	t.Run("never checked", func(t *testing.T) {
		if !isMonitorDue(nil, interval, now) {
			t.Fatal("expected first check to be due")
		}
	})

	t.Run("interval not elapsed", func(t *testing.T) {
		last := now.Add(-30 * time.Second)
		if isMonitorDue(&last, interval, now) {
			t.Fatal("expected monitor to be skipped")
		}
	})

	t.Run("interval elapsed", func(t *testing.T) {
		last := now.Add(-time.Minute)
		if !isMonitorDue(&last, interval, now) {
			t.Fatal("expected monitor to be due")
		}
	})
}
