package handlers

import (
	"fmt"
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/internal/forms"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// SettingsPage отрисовывает страницу настроек мониторинга.
func (h *Handler) SettingsPage(c *gin.Context) {
	interval := models.GetCheckIntervalSeconds(h.DB)
	notifications, err := models.LoadNotificationSettings(h.DB)
	if err != nil {
		applog.AddError("failed to load notification settings", err.Error())
		notifications = models.NotificationSettings{SMTPPort: 587} // дефолт порта для формы
	}

	h.renderPage(c, http.StatusOK, "admin/settings/index.html", h.settingsPageData(interval, notifications), PageOptions{Title: "Settings", ActiveNav: "settings"})
}

// UpdateSettings сохраняет настройки мониторинга из формы настроек.
func (h *Handler) UpdateSettings(c *gin.Context) {
	var input forms.SettingsInput
	if err := c.ShouldBind(&input); err != nil {
		interval := models.GetCheckIntervalSeconds(h.DB)
		notifications, _ := models.LoadNotificationSettings(h.DB)
		data := h.settingsPageData(interval, notifications)
		data["Error"] = "Invalid form data"
		h.renderPage(c, http.StatusBadRequest, "admin/settings/index.html", data, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}
	if input.SMTPPort == 0 {
		input.SMTPPort = 587 // пустое поле порта → стандартный submission port
	}
	if err := input.Validate(); err != nil {
		interval := models.GetCheckIntervalSeconds(h.DB)
		data := h.settingsPageData(interval, notificationSettingsFromInput(input))
		data["Error"] = forms.FormatValidationError(err)
		h.renderPage(c, http.StatusBadRequest, "admin/settings/index.html", data, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}

	if err := models.SetCheckIntervalSeconds(h.DB, input.CheckIntervalSeconds); err != nil {
		// Ошибка сохранения интервала — показываем форму с введёнными значениями.
		interval := models.GetCheckIntervalSeconds(h.DB)
		data := h.settingsPageData(interval, notificationSettingsFromInput(input))
		data["Error"] = "Failed to save settings"
		h.renderPage(c, http.StatusInternalServerError, "admin/settings/index.html", data, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}

	notifications := notificationSettingsFromInput(input)
	if err := models.SaveNotificationSettings(h.DB, notifications, true); err != nil {
		// Интервал уже сохранён; уведомления — отдельный шаг, при ошибке показываем оба блока формы.
		data := h.settingsPageData(input.CheckIntervalSeconds, notifications)
		data["Error"] = "Failed to save notification settings"
		h.renderPage(c, http.StatusInternalServerError, "admin/settings/index.html", data, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}

	applog.AddEvent("settings", fmt.Sprintf("Check interval set to %d seconds", input.CheckIntervalSeconds))
	redirectWithFlash(c, "/admin/settings", flashSavedMessage)
}

// notificationSettingsFromInput сопоставляет отправленные значения формы с настройками уведомлений.
func notificationSettingsFromInput(input forms.SettingsInput) models.NotificationSettings {
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
