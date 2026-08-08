package middlewares

import (
	"time"

	"go-uptime/internal/applog"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ZerologLogger логирует HTTP-запросы через zerolog.
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
			// Handler явно добавил ошибку через c.Error().
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

// dispatchRequestLog записывает общие поля HTTP-запроса и завершает событие zerolog.
// e — событие нужного уровня, уже начатое вызывающим кодом.
// c — контекст Gin для status, method и IP клиента.
// path и query — части URL запроса, зафиксированные до выполнения handlers.
// latency — длительность обработки запроса.
func dispatchRequestLog(e *zerolog.Event, c *gin.Context, path, query string, latency time.Duration) {
	e.Int("status", c.Writer.Status()).
		Str("method", c.Request.Method).
		Str("path", path).
		Str("query", query).
		Str("ip", c.ClientIP()).
		Dur("latency", latency).
		Msg(c.Request.Method + " " + path)
}

// ErrorCapture сохраняет ошибки Gin в памяти приложения.
// Только c.Errors из c.Error() в handlers; паники перехватывает gin.Recovery(), сюда не попадают.
func ErrorCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		for _, err := range c.Errors {
			// Путь запроса — контекст для страницы /admin/errors.
			applog.AddError(err.Error(), c.Request.URL.Path)
		}
	}
}
