package handlers

import (
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// ErrorsPage displays recent application errors from memory.
func (h *Handler) ErrorsPage(c *gin.Context) {
	page := parseQueryPage(c.Query("page"))
	perPage := models.AdminListPageSize
	total := applog.CountErrors()
	page = models.ClampPage(page, total, perPage)

	pagination := buildPaginationView(total, page, perPage, "Application Errors", func(p int) string {
		return buildAdminListURL("/admin/errors", p)
	})

	h.renderPage(c, http.StatusOK, "admin/errors/index.html", gin.H{
		"Entries":    applog.ErrorsPage(page, perPage),
		"Pagination": pagination,
	}, PageOptions{Title: "Errors", ActiveNav: "errors"})
}

// LogsPage renders significant application events kept in memory.
func (h *Handler) LogsPage(c *gin.Context) {
	page := parseQueryPage(c.Query("page"))
	perPage := models.AdminListPageSize
	total := applog.CountEvents()
	page = models.ClampPage(page, total, perPage)

	pagination := buildPaginationView(total, page, perPage, "Application Logs", func(p int) string {
		return buildAdminListURL("/admin/logs", p)
	})

	h.renderPage(c, http.StatusOK, "admin/logs/index.html", gin.H{
		"Entries":    applog.EventsPage(page, perPage),
		"Pagination": pagination,
	}, PageOptions{Title: "Logs", ActiveNav: "logs"})
}
