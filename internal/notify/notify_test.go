package notify

import (
	"testing"

	"go-uptime/models"
)

func TestFormatMonitorMessage(t *testing.T) {
	// Табличный тест formatMonitorMessage для Telegram/SMTP уведомлений.
	tests := []struct {
		name   string
		change MonitorStateChange
		want   string
	}{
		{
			name: "up",
			change: MonitorStateChange{
				DisplayName: "Example",
				URL:         "https://example.com",
				IsUp:        true,
			},
			want: `Monitor "Example" (https://example.com) is UP`,
		},
		{
			name: "down with error",
			change: MonitorStateChange{
				DisplayName: "Example",
				URL:         "https://example.com",
				IsUp:        false,
				Error:       "timeout",
			},
			want: `Monitor "Example" (https://example.com) is DOWN: timeout`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act + Assert: UP без suffix; DOWN с ErrMsg после двоеточия.
			if got := formatMonitorMessage(tt.change); got != tt.want {
				t.Fatalf("formatMonitorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSendTestTelegramRequiresConfiguration(t *testing.T) {
	// Act: SendTestTelegram с пустыми NotificationSettings.
	// Assert: должен вернуть error — нельзя «тестировать» без telegram URL/token.
	if err := SendTestTelegram(models.NotificationSettings{}); err == nil {
		t.Fatal("SendTestTelegram() with empty settings should error")
	}
}

func TestSendTestSMTPRequiresConfiguration(t *testing.T) {
	// Act: SendTestSMTP без SMTP host/credentials.
	// Assert: аналогично Telegram — конфигурация обязательна.
	if err := SendTestSMTP(models.NotificationSettings{}); err == nil {
		t.Fatal("SendTestSMTP() with empty settings should error")
	}
}

func TestSendMonitorStateChangeSkipsUnconfiguredChannels(t *testing.T) {
	// Arrange: monitor хочет notify telegram+SMTP, но settings пустые.
	change := MonitorStateChange{DisplayName: "Example", URL: "https://example.com", IsUp: true}
	// Act: SendMonitorStateChange не должен падать — просто skip unconfigured channels.
	if err := SendMonitorStateChange(
		models.NotificationSettings{},
		models.MonitorURL{NotifyTelegram: true, NotifySMTP: true},
		change,
	); err != nil {
		t.Fatalf("SendMonitorStateChange() with unconfigured channels should not error, got %v", err)
	}
}
