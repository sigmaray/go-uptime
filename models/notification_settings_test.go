package models

import "testing"

func TestNotificationSettingsConfigured(t *testing.T) {
	// Arrange: наборы настроек с разной степенью заполненности каналов.
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
			// Act + Assert: TelegramConfigured отражает наличие telegram URL.
			if got := tt.settings.TelegramConfigured(); got != tt.telegram {
				t.Fatalf("TelegramConfigured() = %v, want %v", got, tt.telegram)
			}
			// Act + Assert: SMTPConfigured требует host, port и получателя.
			if got := tt.settings.SMTPConfigured(); got != tt.smtp {
				t.Fatalf("SMTPConfigured() = %v, want %v", got, tt.smtp)
			}
		})
	}
}

func TestBuildSMTPShoutrrrURL(t *testing.T) {
	// Arrange: полный набор SMTP-полей для shoutrrr URL.
	settings := NotificationSettings{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUser:     "user",
		SMTPPassword: "secret",
		SMTPFrom:     "alerts@example.com",
		SMTPTo:       "ops@example.com",
	}

	// Act: собираем shoutrrr URL с subject из аргумента.
	got, err := BuildSMTPShoutrrrURL(settings, "Monitor DOWN")
	if err != nil {
		t.Fatalf("BuildSMTPShoutrrrURL() error = %v", err)
	}
	// Assert: URL не пустой при валидных настройках.
	if got == "" {
		t.Fatal("BuildSMTPShoutrrrURL() returned empty URL")
	}
}

func TestBuildSMTPShoutrrrURLIncomplete(t *testing.T) {
	// Act: только host без port/to — настройки неполные.
	_, err := BuildSMTPShoutrrrURL(NotificationSettings{SMTPHost: "smtp.example.com"}, "")
	// Assert: ожидаем ошибку валидации, а не пустой URL.
	if err == nil {
		t.Fatal("expected error for incomplete smtp settings")
	}
}
