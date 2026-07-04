package worker

import "testing"

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
