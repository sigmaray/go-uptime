package handlers

import (
	"fmt"
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// SettingsPage отображает страницу настроек мониторинга.
func (h *Handler) SettingsPage(c *gin.Context) {
	interval := models.GetCheckIntervalSeconds(h.DB, h.Config.CheckIntervalSeconds)
	notifications, err := models.LoadNotificationSettings(h.DB)
	if err != nil {
		applog.AddError("failed to load notification settings", err.Error())
		notifications = models.NotificationSettings{SMTPPort: 587}
	}

	h.renderPage(c, http.StatusOK, "app/settings/index.html", h.settingsPageData(interval, notifications), PageOptions{Title: "Settings", ActiveNav: "settings"})
}

// UpdateSettings сохраняет настройки мониторинга.
func (h *Handler) UpdateSettings(c *gin.Context) {
	var input models.SettingsInput
	if err := c.ShouldBind(&input); err != nil {
		interval := models.GetCheckIntervalSeconds(h.DB, h.Config.CheckIntervalSeconds)
		notifications, _ := models.LoadNotificationSettings(h.DB)
		data := h.settingsPageData(interval, notifications)
		data["Error"] = "Invalid form data"
		h.renderPage(c, http.StatusBadRequest, "app/settings/index.html", data, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}
	if input.SMTPPort == 0 {
		input.SMTPPort = 587
	}
	if err := input.Validate(); err != nil {
		interval := models.GetCheckIntervalSeconds(h.DB, h.Config.CheckIntervalSeconds)
		data := h.settingsPageData(interval, notificationSettingsFromInput(input))
		data["Error"] = models.FormatValidationError(err)
		h.renderPage(c, http.StatusBadRequest, "app/settings/index.html", data, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}

	if err := models.SetCheckIntervalSeconds(h.DB, input.CheckIntervalSeconds); err != nil {
		interval := models.GetCheckIntervalSeconds(h.DB, h.Config.CheckIntervalSeconds)
		data := h.settingsPageData(interval, notificationSettingsFromInput(input))
		data["Error"] = "Failed to save settings"
		h.renderPage(c, http.StatusInternalServerError, "app/settings/index.html", data, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}

	notifications := notificationSettingsFromInput(input)
	if err := models.SaveNotificationSettings(h.DB, notifications, true); err != nil {
		data := h.settingsPageData(input.CheckIntervalSeconds, notifications)
		data["Error"] = "Failed to save notification settings"
		h.renderPage(c, http.StatusInternalServerError, "app/settings/index.html", data, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}

	applog.AddEvent("settings", fmt.Sprintf("Check interval set to %d seconds", input.CheckIntervalSeconds))
	c.Redirect(http.StatusFound, "/app/settings?saved=1")
}

// notificationSettingsFromInput преобразует данные формы в структуру настроек уведомлений.
func notificationSettingsFromInput(input models.SettingsInput) models.NotificationSettings {
	return models.NotificationSettings{
		TelegramURL:  input.TelegramURL,
		SMTPHost:     input.SMTPHost,
		SMTPPort:     input.SMTPPort,
		SMTPUser:     input.SMTPUser,
		SMTPPassword: input.SMTPPassword,
		SMTPFrom:     input.SMTPFrom,
		SMTPTo:       input.SMTPTo,
	}
}
