package middlewares

import (
	"time"

	"go-uptime/internal/applog"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ZerologLogger logs HTTP requests through zerolog.
func ZerologLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		event := log.Info()
		if len(c.Errors) > 0 {
			event = log.Error().Err(c.Errors.Last())
		} else if c.Writer.Status() >= 400 && c.Writer.Status() < 500 {
			event = log.Warn()
		} else if c.Writer.Status() >= 500 {
			event = log.Error()
		}

		event.
			Int("status", c.Writer.Status()).
			Str("method", c.Request.Method).
			Str("path", path).
			Str("query", query).
			Str("ip", c.ClientIP()).
			Dur("latency", latency).
			Msg(c.Request.Method + " " + path)
	}
}

// ErrorCapture stores Gin errors in application memory.
func ErrorCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		for _, err := range c.Errors {
			applog.AddError(err.Error(), c.Request.URL.Path)
		}
	}
}
