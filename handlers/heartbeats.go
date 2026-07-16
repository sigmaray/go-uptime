package handlers

import (
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// HeartbeatsList renders the global heartbeat history page.
func (h *Handler) HeartbeatsList(c *gin.Context) {
	page := parseQueryPage(c.Query("page"))
	perPage := models.AdminListPageSize

	total, err := models.CountMonitorChecks(h.DB)
	if err != nil {
		applog.AddError("failed to count heartbeats", err.Error())
		total = 0
	}
	page = models.ClampPage(page, total, perPage)

	checks, err := models.LoadAllMonitorChecksPage(h.DB, page, perPage)
	if err != nil {
		applog.AddError("failed to load heartbeats", err.Error())
		checks = nil
	}

	pagination := buildPaginationView(total, page, perPage, "Heartbeats", func(p int) string {
		return buildAdminListURL("/admin/heartbeats", p)
	})

	h.renderPage(c, http.StatusOK, "admin/heartbeats/index.html", gin.H{
		"Heartbeats": checks,
		"Pagination": pagination,
	}, PageOptions{Title: "Heartbeats", ActiveNav: "heartbeats"})
}
