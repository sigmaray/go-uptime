package middlewares

import (
	"time"

	"go-uptime/internal/applog"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
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
		status := c.Writer.Status()
		switch {
		case len(c.Errors) > 0:
			dispatchRequestLog(log.Error().Err(c.Errors.Last()), c, path, query, latency)
		case status >= 500:
			dispatchRequestLog(log.Error(), c, path, query, latency)
		case status >= 400:
			dispatchRequestLog(log.Warn(), c, path, query, latency)
		default:
			dispatchRequestLog(log.Info(), c, path, query, latency)
		}
	}
}

// dispatchRequestLog writes the shared HTTP request fields and finishes the zerolog event.
// e is the level-specific event already started by the caller.
// c is the Gin context for status, method, and client IP.
// path and query are the request URL parts captured before handlers ran.
// latency is how long the request took.
func dispatchRequestLog(e *zerolog.Event, c *gin.Context, path, query string, latency time.Duration) {
	e.Int("status", c.Writer.Status()).
		Str("method", c.Request.Method).
		Str("path", path).
		Str("query", query).
		Str("ip", c.ClientIP()).
		Dur("latency", latency).
		Msg(c.Request.Method + " " + path)
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
