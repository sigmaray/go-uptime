package handlers

import (
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// HeartbeatsList отрисовывает глобальную страницу истории heartbeat.
func (h *Handler) HeartbeatsList(c *gin.Context) {
	page := parseQueryPage(c.Query("page"))
	perPage := models.AdminListPageSize
	sort := ParseListSort(
		"/admin/heartbeats",
		models.MonitorCheck{},
		"monitor_checks.checked_at desc, monitor_checks.id asc",
		c.Query("sort"),
		c.Query("order"),
		"MonitorURL", "CheckedAt", "ResponseTimeMs", "IsUp",
	)

	total, err := models.CountMonitorChecks(h.DB)
	if err != nil {
		applog.AddError("failed to count heartbeats", err.Error())
		total = 0
	}
	page = models.ClampPage(page, total, perPage)

	var checks []models.MonitorCheck
	// Preload MonitorURL — сортировка по MonitorURL требует JOIN в sort.Apply.
	query := sort.Apply(h.DB.Model(&models.MonitorCheck{}).Preload("MonitorURL"))
	if err := query.
		Offset(models.PageOffset(page, perPage)).
		Limit(perPage).
		Find(&checks).Error; err != nil {
		applog.AddError("failed to load heartbeats", err.Error())
		checks = nil
	}

	pagination := buildPaginationView(total, page, perPage, "Heartbeats", sort.PageURL)

	h.renderPage(c, http.StatusOK, "admin/heartbeats/index.html", gin.H{
		"Heartbeats": checks,
		"Pagination": pagination,
		"Sort":       sort,
	}, PageOptions{Title: "Heartbeats", ActiveNav: "heartbeats"})
}
