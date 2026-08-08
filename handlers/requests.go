package handlers

import (
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// RequestsPage отрисовывает последние HTTP-запросы к мониторимым сайтам из памяти.
func (h *Handler) RequestsPage(c *gin.Context) {
	page := parseQueryPage(c.Query("page"))
	perPage := models.AdminListPageSize
	total := applog.CountMonitorRequests() // HTTP probe log из worker, in-memory
	page = models.ClampPage(page, total, perPage)

	pagination := buildPaginationView(total, page, perPage, "Monitor Requests", func(p int) string {
		return buildAdminListURL("/admin/requests", p)
	})

	h.renderPage(c, http.StatusOK, "admin/requests/index.html", gin.H{
		"Entries":    applog.MonitorRequestsPage(page, perPage),
		"Pagination": pagination,
	}, PageOptions{Title: "Requests", ActiveNav: "requests"})
}
