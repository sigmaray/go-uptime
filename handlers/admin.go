package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminDashboard отображает главную страницу админ-панели.
func (h *Handler) AdminDashboard(c *gin.Context) {
	h.renderPage(c, http.StatusOK, "admin/dashboard/index.html", gin.H{}, PageOptions{
		Title:     "Dashboard",
		ActiveNav: "dashboard",
	})
}

// Health возвращает статус работоспособности сервера.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
