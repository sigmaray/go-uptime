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
