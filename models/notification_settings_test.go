package models

import "testing"

func TestNotificationSettingsConfigured(t *testing.T) {
	tests := []struct {
		name     string
		settings NotificationSettings
		telegram bool
		smtp     bool
	}{
		{
			name:     "empty",
			settings: NotificationSettings{},
		},
		{
			name: "telegram only",
			settings: NotificationSettings{
				TelegramURL: "telegram://token@telegram?channels=123",
			},
			telegram: true,
		},
		{
			name: "smtp only",
			settings: NotificationSettings{
				SMTPHost: "smtp.example.com",
				SMTPPort: 587,
				SMTPTo:   "ops@example.com",
			},
			smtp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.settings.TelegramConfigured(); got != tt.telegram {
				t.Fatalf("TelegramConfigured() = %v, want %v", got, tt.telegram)
			}
			if got := tt.settings.SMTPConfigured(); got != tt.smtp {
				t.Fatalf("SMTPConfigured() = %v, want %v", got, tt.smtp)
			}
		})
	}
}

func TestBuildSMTPShoutrrrURL(t *testing.T) {
	settings := NotificationSettings{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUser:     "user",
		SMTPPassword: "secret",
		SMTPFrom:     "alerts@example.com",
		SMTPTo:       "ops@example.com",
	}

	got, err := BuildSMTPShoutrrrURL(settings, "Monitor DOWN")
	if err != nil {
		t.Fatalf("BuildSMTPShoutrrrURL() error = %v", err)
	}
	if got == "" {
		t.Fatal("BuildSMTPShoutrrrURL() returned empty URL")
	}
}

func TestBuildSMTPShoutrrrURLIncomplete(t *testing.T) {
	_, err := BuildSMTPShoutrrrURL(NotificationSettings{SMTPHost: "smtp.example.com"}, "")
	if err == nil {
		t.Fatal("expected error for incomplete smtp settings")
	}
}
