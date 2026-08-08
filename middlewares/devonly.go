package middlewares

import (
	"net/http"

	"go-uptime/config"

	"github.com/gin-gonic/gin"
)

// DevelopmentOnly разрешает доступ только в режиме development.
func DevelopmentOnly(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.IsDevelopment() {
			// 404, не 403 — скрываем существование dev-only маршрутов в production.
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Next()
	}
}
