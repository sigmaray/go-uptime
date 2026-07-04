package handlers

import (
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// monitorNotificationContext собирает данные о доступности каналов оповещения для шаблонов.
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

// bindMonitorNotificationFlags читает флаги notify_* из формы с учётом системных настроек.
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

// settingsPageData собирает данные для страницы настроек.
func (h *Handler) settingsPageData(interval int, notifications models.NotificationSettings) gin.H {
	return gin.H{
		"CheckIntervalSeconds": interval,
		"NotificationSettings": notifications,
		"TelegramConfigured":   notifications.TelegramConfigured(),
		"SMTPConfigured":       notifications.SMTPConfigured(),
	}
}
