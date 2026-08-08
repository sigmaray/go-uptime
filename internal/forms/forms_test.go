package forms

import "testing"

func TestParseCheckIntervalSeconds(t *testing.T) {
	// Табличный тест ParseCheckIntervalSeconds: nil = inherit global, bounds 10..86400.
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
			// Act: парсим строку формы через MonitorURLInput.
			got, err := MonitorURLInput{CheckIntervalSeconds: tt.input}.ParseCheckIntervalSeconds()
			if tt.wantErr {
				// Assert: невалидные значения — error path.
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want == nil {
				// Assert: пустой input → nil pointer (наследование глобального интервала).
				if got != nil {
					t.Fatalf("got %v, want nil", *got)
				}
				return
			}
			// Assert: валидное число → *int с ожидаемым значением.
			if got == nil || *got != *tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateTelegramShoutrrrURL(t *testing.T) {
	// Arrange: валидный интервал, но TelegramURL — обычный HTTPS (не shoutrrr scheme).
	input := SettingsInput{
		CheckIntervalSeconds: 60,
		TelegramURL:          "https://example.com",
	}
	// Assert: Validate отклоняет non-telegram URL.
	if err := input.Validate(); err == nil {
		t.Fatal("expected validation error for non-telegram URL")
	}

	// Act: корректный shoutrrr telegram:// URL.
	input.TelegramURL = "telegram://token@telegram?channels=123"
	// Assert: проходит полную Validate settings form.
	if err := input.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

// intPtr — test helper: литерал *int без локальной переменной в table-driven case.
func intPtr(v int) *int {
	return &v
}
