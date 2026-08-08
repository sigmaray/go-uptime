package handlers

import (
	"go-uptime/internal/forms"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// monitorNotificationContext собирает данные о доступности каналов уведомлений для шаблонов.
func (h *Handler) monitorNotificationContext() (models.NotificationSettings, gin.H, error) {
	settings, err := models.LoadNotificationSettings(h.DB)
	if err != nil {
		return models.NotificationSettings{}, nil, err
	}

	// Шаблоны показывают чекбоксы notify_* только если канал настроен глобально.
	return settings, gin.H{
		"TelegramConfigured": settings.TelegramConfigured(),
		"SMTPConfigured":     settings.SMTPConfigured(),
	}, nil
}

// bindMonitorNotificationFlags читает флаги notify_* из формы с учётом системных настроек.
// c — контекст Gin-запроса с POST-формой.
// input получает NotifyTelegram и NotifySMTP, когда соответствующий канал настроен.
func (h *Handler) bindMonitorNotificationFlags(c *gin.Context, input *forms.MonitorURLInput) error {
	telegram, smtp, err := h.readMonitorNotificationFlags(c)
	if err != nil {
		return err
	}
	input.NotifyTelegram = telegram
	input.NotifySMTP = smtp
	return nil
}

// bindBulkMonitorNotificationFlags читает флаги notify_* для массового создания мониторов.
// c — контекст Gin-запроса с POST-формой.
// input получает NotifyTelegram и NotifySMTP, когда соответствующий канал настроен.
func (h *Handler) bindBulkMonitorNotificationFlags(c *gin.Context, input *forms.MonitorURLBulkInput) error {
	telegram, smtp, err := h.readMonitorNotificationFlags(c)
	if err != nil {
		return err
	}
	input.NotifyTelegram = telegram
	input.NotifySMTP = smtp
	return nil
}

// readMonitorNotificationFlags читает POST-поля notify_*, когда каналы настроены в Settings.
// c — контекст Gin-запроса с POST-формой.
// Возвращает флаги telegram и smtp (false, если канал не настроен).
func (h *Handler) readMonitorNotificationFlags(c *gin.Context) (notifyTelegram, notifySMTP bool, err error) {
	settings, err := models.LoadNotificationSettings(h.DB)
	if err != nil {
		return false, false, err
	}

	if settings.TelegramConfigured() {
		notifyTelegram = c.PostForm("notify_telegram") == "on" // HTML checkbox
	}
	if settings.SMTPConfigured() {
		notifySMTP = c.PostForm("notify_smtp") == "on"
	}
	return notifyTelegram, notifySMTP, nil
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
