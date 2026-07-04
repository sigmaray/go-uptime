package handlers

import (
	"net/http"

	"go-uptime/internal/applog"

	"github.com/gin-gonic/gin"
)

// ErrorsPage отображает последние ошибки приложения из памяти.
func (h *Handler) ErrorsPage(c *gin.Context) {
	h.renderPage(c, http.StatusOK, "admin/errors/index.html", gin.H{
		"Entries": applog.RecentErrors(),
	}, PageOptions{Title: "Errors", ActiveNav: "errors"})
}

// LogsPage renders significant application events kept in memory.
func (h *Handler) LogsPage(c *gin.Context) {
	h.renderPage(c, http.StatusOK, "admin/logs/index.html", gin.H{
		"Entries": applog.RecentEvents(),
	}, PageOptions{Title: "Logs", ActiveNav: "logs"})
}
