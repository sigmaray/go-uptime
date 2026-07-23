package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// healthCheckResult is the outcome of /health dependency probes.
type healthCheckResult struct {
	// OK is true when every required dependency is healthy.
	OK bool
	// Database is "ok" or "unavailable".
	Database string
	// Worker is "ok" or "not running".
	Worker string
}

// evaluateHealth maps probe outcomes into a stable /health payload.
// dbErr is the database ping error, or nil when the database responded.
// workerRunning is true when the background monitor worker is active.
func evaluateHealth(dbErr error, workerRunning bool) healthCheckResult {
	result := healthCheckResult{
		OK:       true,
		Database: "ok",
		Worker:   "ok",
	}
	if dbErr != nil {
		result.OK = false
		result.Database = "unavailable"
	}
	if !workerRunning {
		result.OK = false
		result.Worker = "not running"
	}
	return result
}

// Health reports process readiness for orchestrators and reverse proxies.
// It returns 200 when PostgreSQL accepts a ping and the monitor worker is running;
// otherwise it returns 503 with per-check details (without internal error text).
func (h *Handler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	dbErr := h.pingDatabase(ctx)
	workerRunning := h.Worker != nil && h.Worker.Running()
	result := evaluateHealth(dbErr, workerRunning)

	if dbErr != nil {
		log.Error().Err(dbErr).Msg("health check: database unavailable")
	}
	if !workerRunning {
		log.Error().Msg("health check: monitor worker not running")
	}

	body := gin.H{
		"status": "ok",
		"checks": gin.H{
			"database": result.Database,
			"worker":   result.Worker,
		},
	}
	if !result.OK {
		body["status"] = "unhealthy"
		c.JSON(http.StatusServiceUnavailable, body)
		return
	}
	c.JSON(http.StatusOK, body)
}

// pingDatabase verifies that the application can reach PostgreSQL.
// ctx bounds how long the ping may wait before failing.
func (h *Handler) pingDatabase(ctx context.Context) error {
	if h.DB == nil {
		return errHealthDatabaseMissing
	}
	sqlDB, err := h.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// errHealthDatabaseMissing is returned when the handler has no GORM handle.
var errHealthDatabaseMissing = errors.New("database handle is not configured")
