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

	h.renderPage(c, http.StatusOK, "app/settings/index.html", gin.H{
		"CheckIntervalSeconds": interval,
	}, PageOptions{Title: "Settings", ActiveNav: "settings"})
}

// UpdateSettings сохраняет настройки мониторинга.
func (h *Handler) UpdateSettings(c *gin.Context) {
	var input models.SettingsInput
	if err := c.ShouldBind(&input); err != nil {
		h.renderPage(c, http.StatusBadRequest, "app/settings/index.html", gin.H{
			"Error":                "Invalid form data",
			"CheckIntervalSeconds": input.CheckIntervalSeconds,
		}, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}
	if err := input.Validate(); err != nil {
		h.renderPage(c, http.StatusBadRequest, "app/settings/index.html", gin.H{
			"Error":                models.FormatValidationError(err),
			"CheckIntervalSeconds": input.CheckIntervalSeconds,
		}, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}

	if err := models.SetCheckIntervalSeconds(h.DB, input.CheckIntervalSeconds); err != nil {
		h.renderPage(c, http.StatusInternalServerError, "app/settings/index.html", gin.H{
			"Error":                "Failed to save settings",
			"CheckIntervalSeconds": input.CheckIntervalSeconds,
		}, PageOptions{Title: "Settings", ActiveNav: "settings"})
		return
	}

	applog.AddEvent("settings", fmt.Sprintf("Check interval set to %d seconds", input.CheckIntervalSeconds))
	c.Redirect(http.StatusFound, "/app/settings?saved=1")
}
