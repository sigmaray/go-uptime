package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminDashboard отображает главную страницу админ-панели.
func (h *Handler) AdminDashboard(c *gin.Context) {
	// Статическая landing-страница админки без данных из БД.
	h.renderPage(c, http.StatusOK, "admin/dashboard/index.html", gin.H{}, PageOptions{
		Title:     "Dashboard",
		ActiveNav: "dashboard",
	})
}
