// Package notify отправляет уведомления об изменении статуса мониторов через Shoutrrr.
package notify

import (
	"fmt"
	"strings"

	"go-uptime/models"

	"github.com/nicholas-fedor/shoutrrr"
)

// MonitorStateChange описывает изменение доступности монитора для уведомления.
type MonitorStateChange struct {
	DisplayName string
	URL         string
	IsUp        bool
	Error       string
}

const testNotificationMessage = "Go Uptime test notification from Dev Tools."

// SendTestTelegram отправляет тестовое сообщение в Telegram.
// settings содержит системные настройки с Shoutrrr URL.
func SendTestTelegram(settings models.NotificationSettings) error {
	if !settings.TelegramConfigured() {
		return fmt.Errorf("telegram is not configured")
	}
	return shoutrrr.Send(settings.TelegramURL, testNotificationMessage)
}

// SendTestSMTP отправляет тестовое письмо через SMTP.
// settings содержит системные SMTP-настройки.
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

// SendMonitorStateChange отправляет уведомления через включённые каналы монитора.
// Канал пропускается, если флаг monitor.Notify* выключен или канал не настроен в settings
// (TelegramConfigured / SMTPConfigured) — это не ошибка, просто нет работы.
// Ошибки разных каналов накапливаются; joinErrors склеивает их в одну строку через «; »,
// чтобы worker мог залогировать частичный сбой (например, Telegram упал, SMTP прошёл).
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

	// nil если все включённые каналы успешны или ни один не был активен.
	return joinErrors(errs)
}

// formatMonitorMessage формирует текст уведомления об изменении статуса монитора.
func formatMonitorMessage(change MonitorStateChange) string {
	name := strings.TrimSpace(change.DisplayName)
	if name == "" {
		name = change.URL
	}
	if change.IsUp {
		return fmt.Sprintf("Monitor %q (%s) is UP", name, change.URL)
	}
	if strings.TrimSpace(change.Error) == "" {
		// DOWN без текста ошибки — только факт недоступности.
		return fmt.Sprintf("Monitor %q (%s) is DOWN", name, change.URL)
	}
	return fmt.Sprintf("Monitor %q (%s) is DOWN: %s", name, change.URL, change.Error)
}

// monitorStateSubject возвращает тему письма для SMTP-уведомления.
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

// joinErrors объединяет несколько ошибок отправки в одну.
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
	// Несколько каналов упали — одна строка с разделителем «; ».
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}
