package notify

import (
	"testing"

	"go-uptime/models"
)

func TestFormatMonitorMessage(t *testing.T) {
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
			if got := formatMonitorMessage(tt.change); got != tt.want {
				t.Fatalf("formatMonitorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSendTestTelegramRequiresConfiguration(t *testing.T) {
	if err := SendTestTelegram(models.NotificationSettings{}); err == nil {
		t.Fatal("SendTestTelegram() with empty settings should error")
	}
}

func TestSendTestSMTPRequiresConfiguration(t *testing.T) {
	if err := SendTestSMTP(models.NotificationSettings{}); err == nil {
		t.Fatal("SendTestSMTP() with empty settings should error")
	}
}

func TestSendMonitorStateChangeSkipsUnconfiguredChannels(t *testing.T) {
	change := MonitorStateChange{DisplayName: "Example", URL: "https://example.com", IsUp: true}
	if err := SendMonitorStateChange(
		models.NotificationSettings{},
		models.MonitorURL{NotifyTelegram: true, NotifySMTP: true},
		change,
	); err != nil {
		t.Fatalf("SendMonitorStateChange() with unconfigured channels should not error, got %v", err)
	}
}
