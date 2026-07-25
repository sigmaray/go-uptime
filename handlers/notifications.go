package handlers

import (
	"go-uptime/internal/forms"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// monitorNotificationContext collects notification channel availability data for templates.
func (h *Handler) monitorNotificationContext() (models.NotificationSettings, gin.H, error) {
	settings, err := models.LoadNotificationSettings(h.DB)
	if err != nil {
		return models.NotificationSettings{}, nil, err
	}

	return settings, gin.H{
		"TelegramConfigured": settings.TelegramConfigured(),
		"SMTPConfigured":     settings.SMTPConfigured(),
	}, nil
}

// bindMonitorNotificationFlags reads notify_* flags from the form, respecting system settings.
// c is the Gin request context with the POST form.
// input receives NotifyTelegram and NotifySMTP when the corresponding channel is configured.
func (h *Handler) bindMonitorNotificationFlags(c *gin.Context, input *forms.MonitorURLInput) error {
	telegram, smtp, err := h.readMonitorNotificationFlags(c)
	if err != nil {
		return err
	}
	input.NotifyTelegram = telegram
	input.NotifySMTP = smtp
	return nil
}

// bindBulkMonitorNotificationFlags reads notify_* flags for bulk monitor creation.
// c is the Gin request context with the POST form.
// input receives NotifyTelegram and NotifySMTP when the corresponding channel is configured.
func (h *Handler) bindBulkMonitorNotificationFlags(c *gin.Context, input *forms.MonitorURLBulkInput) error {
	telegram, smtp, err := h.readMonitorNotificationFlags(c)
	if err != nil {
		return err
	}
	input.NotifyTelegram = telegram
	input.NotifySMTP = smtp
	return nil
}

// readMonitorNotificationFlags reads notify_* POST fields when channels are configured in Settings.
// c is the Gin request context with the POST form.
// It returns telegram and smtp preference flags (false when the channel is not configured).
func (h *Handler) readMonitorNotificationFlags(c *gin.Context) (notifyTelegram, notifySMTP bool, err error) {
	settings, err := models.LoadNotificationSettings(h.DB)
	if err != nil {
		return false, false, err
	}

	if settings.TelegramConfigured() {
		notifyTelegram = c.PostForm("notify_telegram") == "on"
	}
	if settings.SMTPConfigured() {
		notifySMTP = c.PostForm("notify_smtp") == "on"
	}
	return notifyTelegram, notifySMTP, nil
}

// settingsPageData collects data for the settings page.
func (h *Handler) settingsPageData(interval int, notifications models.NotificationSettings) gin.H {
	return gin.H{
		"CheckIntervalSeconds": interval,
		"NotificationSettings": notifications,
		"TelegramConfigured":   notifications.TelegramConfigured(),
		"SMTPConfigured":       notifications.SMTPConfigured(),
	}
}
