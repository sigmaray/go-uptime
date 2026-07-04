package handlers

import (
	"net/http"

	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// IncidentsList отображает историю инцидентов.
func (h *Handler) IncidentsList(c *gin.Context) {
	var incidents []models.Incident
	h.DB.Preload("MonitorURL").Order("started_at desc").Limit(200).Find(&incidents)

	h.renderPage(c, http.StatusOK, "admin/incidents/index.html", gin.H{
		"Incidents": incidents,
	}, PageOptions{Title: "Incidents", ActiveNav: "incidents"})
}
