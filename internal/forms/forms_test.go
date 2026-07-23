package forms

import "testing"

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

func TestValidateTelegramShoutrrrURL(t *testing.T) {
	input := SettingsInput{
		CheckIntervalSeconds: 60,
		TelegramURL:          "https://example.com",
	}
	if err := input.Validate(); err == nil {
		t.Fatal("expected validation error for non-telegram URL")
	}

	input.TelegramURL = "telegram://token@telegram?channels=123"
	if err := input.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func intPtr(v int) *int {
	return &v
}
