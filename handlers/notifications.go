package handlers

import (
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
func (h *Handler) bindMonitorNotificationFlags(c *gin.Context, input *models.MonitorURLInput) error {
	settings, err := models.LoadNotificationSettings(h.DB)
	if err != nil {
		return err
	}

	input.NotifyTelegram = false
	input.NotifySMTP = false
	if settings.TelegramConfigured() {
		input.NotifyTelegram = c.PostForm("notify_telegram") == "on"
	}
	if settings.SMTPConfigured() {
		input.NotifySMTP = c.PostForm("notify_smtp") == "on"
	}
	return nil
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
