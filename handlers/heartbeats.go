package handlers

import (
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// HeartbeatsList renders the global heartbeat history page.
func (h *Handler) HeartbeatsList(c *gin.Context) {
	checks, err := models.LoadRecentMonitorChecks(h.DB, models.MaxRecentHeartbeatsList)
	if err != nil {
		applog.AddError("failed to load heartbeats", err.Error())
		checks = nil
	}

	h.renderPage(c, http.StatusOK, "admin/heartbeats/index.html", gin.H{
		"Heartbeats": checks,
	}, PageOptions{Title: "Heartbeats", ActiveNav: "heartbeats"})
}
