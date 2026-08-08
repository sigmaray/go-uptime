// Package middlewares предоставляет Gin middleware для auth, логирования и rate limits.
package middlewares

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// AuthRequired перенаправляет неаутентифицированных пользователей на страницу входа.
// Без return URL в query — после login пользователь попадает на dashboard по умолчанию.
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("user") == nil {
			// Нет user в session — редирект на /login, handler не вызываем.
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}
