package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminDashboard displays the admin panel home page.
func (h *Handler) AdminDashboard(c *gin.Context) {
	h.renderPage(c, http.StatusOK, "admin/dashboard/index.html", gin.H{}, PageOptions{
		Title:     "Dashboard",
		ActiveNav: "dashboard",
	})
}

// Health returns server health status.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
