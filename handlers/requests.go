package handlers

import (
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// RequestsPage renders the last in-memory HTTP requests to monitored sites.
func (h *Handler) RequestsPage(c *gin.Context) {
	page := parseQueryPage(c.Query("page"))
	perPage := models.AdminListPageSize
	total := applog.CountMonitorRequests()
	page = models.ClampPage(page, total, perPage)

	pagination := buildPaginationView(total, page, perPage, "Monitor Requests", func(p int) string {
		return buildAdminListURL("/admin/requests", p)
	})

	h.renderPage(c, http.StatusOK, "admin/requests/index.html", gin.H{
		"Entries":    applog.MonitorRequestsPage(page, perPage),
		"Pagination": pagination,
	}, PageOptions{Title: "Requests", ActiveNav: "requests"})
}
