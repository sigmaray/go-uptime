// Package notify sends monitor status-change notifications via Shoutrrr.
package notify

import (
	"fmt"
	"strings"

	"go-uptime/models"

	"github.com/nicholas-fedor/shoutrrr"
)

// MonitorStateChange describes a monitor availability change for notification.
type MonitorStateChange struct {
	DisplayName string
	URL         string
	IsUp        bool
	Error       string
}

const testNotificationMessage = "Go Uptime test notification from Dev Tools."

// SendTestTelegram sends a test message to Telegram.
// settings holds system settings with the Shoutrrr URL.
func SendTestTelegram(settings models.NotificationSettings) error {
	if !settings.TelegramConfigured() {
		return fmt.Errorf("telegram is not configured")
	}
	return shoutrrr.Send(settings.TelegramURL, testNotificationMessage)
}

// SendTestSMTP sends a test email via SMTP.
// settings holds system SMTP settings.
func SendTestSMTP(settings models.NotificationSettings) error {
	if !settings.SMTPConfigured() {
		return fmt.Errorf("smtp is not configured")
	}
	smtpURL, err := models.BuildSMTPShoutrrrURL(settings, "Go Uptime test notification")
	if err != nil {
		return err
	}
	return shoutrrr.Send(smtpURL, testNotificationMessage)
}

// SendMonitorStateChange sends notifications through enabled channels.
// settings holds system channel settings; monitor is the monitor with notify_* flags;
// change describes the event.
func SendMonitorStateChange(settings models.NotificationSettings, monitor models.MonitorURL, change MonitorStateChange) error {
	message := formatMonitorMessage(change)
	var errs []error

	if monitor.NotifyTelegram && settings.TelegramConfigured() {
		if err := shoutrrr.Send(settings.TelegramURL, message); err != nil {
			errs = append(errs, fmt.Errorf("telegram: %w", err))
		}
	}

	if monitor.NotifySMTP && settings.SMTPConfigured() {
		smtpURL, err := models.BuildSMTPShoutrrrURL(settings, monitorStateSubject(change))
		if err != nil {
			errs = append(errs, err)
		} else if err := shoutrrr.Send(smtpURL, message); err != nil {
			errs = append(errs, fmt.Errorf("smtp: %w", err))
		}
	}

	return joinErrors(errs)
}

// formatMonitorMessage builds the notification text for a monitor status change.
func formatMonitorMessage(change MonitorStateChange) string {
	name := strings.TrimSpace(change.DisplayName)
	if name == "" {
		name = change.URL
	}
	if change.IsUp {
		return fmt.Sprintf("Monitor %q (%s) is UP", name, change.URL)
	}
	if strings.TrimSpace(change.Error) == "" {
		return fmt.Sprintf("Monitor %q (%s) is DOWN", name, change.URL)
	}
	return fmt.Sprintf("Monitor %q (%s) is DOWN: %s", name, change.URL, change.Error)
}

// monitorStateSubject returns the email subject for an SMTP notification.
func monitorStateSubject(change MonitorStateChange) string {
	name := strings.TrimSpace(change.DisplayName)
	if name == "" {
		name = change.URL
	}
	if change.IsUp {
		return fmt.Sprintf("Monitor UP: %s", name)
	}
	return fmt.Sprintf("Monitor DOWN: %s", name)
}

// joinErrors combines multiple send errors into one.
func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}
