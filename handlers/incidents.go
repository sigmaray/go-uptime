package handlers

import (
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// IncidentsList отображает историю инцидентов.
func (h *Handler) IncidentsList(c *gin.Context) {
	page := parseQueryPage(c.Query("page"))
	perPage := models.AdminListPageSize

	total, err := models.CountIncidents(h.DB)
	if err != nil {
		applog.AddError("failed to count incidents", err.Error())
		total = 0
	}
	page = models.ClampPage(page, total, perPage)

	// LoadIncidentsPage подгружает MonitorURL для отображения имени в списке.
	incidents, err := models.LoadIncidentsPage(h.DB, page, perPage)
	if err != nil {
		applog.AddError("failed to load incidents", err.Error())
		incidents = nil
	}

	pagination := buildPaginationView(total, page, perPage, "Incidents", func(p int) string {
		return buildAdminListURL("/admin/incidents", p)
	})

	h.renderPage(c, http.StatusOK, "admin/incidents/index.html", gin.H{
		"Incidents":  incidents,
		"Pagination": pagination,
	}, PageOptions{Title: "Incidents", ActiveNav: "incidents"})
}
