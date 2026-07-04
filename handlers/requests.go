package handlers

import (
	"net/http"

	"go-uptime/internal/applog"

	"github.com/gin-gonic/gin"
)

// RequestsPage renders the last in-memory HTTP requests to monitored sites.
func (h *Handler) RequestsPage(c *gin.Context) {
	h.renderPage(c, http.StatusOK, "admin/requests/index.html", gin.H{
		"Entries": applog.RecentMonitorRequests(),
	}, PageOptions{Title: "Requests", ActiveNav: "requests"})
}
