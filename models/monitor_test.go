package models

import (
	"testing"
)

func TestParseCheckIntervalSeconds(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *int
		wantErr bool
	}{
		{
			name:  "empty inherits global",
			input: "",
			want:  nil,
		},
		{
			name:  "whitespace inherits global",
			input: "   ",
			want:  nil,
		},
		{
			name:  "valid interval",
			input: "120",
			want:  intPtr(120),
		},
		{
			name:    "too low",
			input:   "9",
			wantErr: true,
		},
		{
			name:    "too high",
			input:   "86401",
			wantErr: true,
		},
		{
			name:    "not a number",
			input:   "abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MonitorURLInput{CheckIntervalSeconds: tt.input}.ParseCheckIntervalSeconds()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMonitorCheckIntervalSeconds(t *testing.T) {
	global := 60
	custom := 300

	tests := []struct {
		name    string
		monitor MonitorURL
		want    int
	}{
		{
			name:    "uses global when unset",
			monitor: MonitorURL{},
			want:    60,
		},
		{
			name:    "uses monitor override",
			monitor: MonitorURL{CheckIntervalSeconds: &custom},
			want:    300,
		},
		{
			name:    "ignores invalid override",
			monitor: MonitorURL{CheckIntervalSeconds: intPtr(5)},
			want:    60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MonitorCheckIntervalSeconds(tt.monitor, global); got != tt.want {
				t.Fatalf("MonitorCheckIntervalSeconds() = %d, want %d", got, tt.want)
			}
		})
	}
}

func intPtr(v int) *int {
	return &v
}
