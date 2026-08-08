package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// /health для оркестраторов (Docker, k8s) и балансировщиков: 200 = процесс готов принимать трафик,
// 503 = зависимость недоступна — балансировщик не должен слать пользователей на этот инстанс.

// healthCheckResult — результат проверок зависимостей /health.
type healthCheckResult struct {
	// OK — true, когда все обязательные зависимости здоровы.
	OK bool
	// Database — "ok" или "unavailable".
	Database string
	// Worker — "ok" или "not running".
	Worker string
}

// evaluateHealth преобразует результаты проверок в стабильный payload /health.
// dbErr — ошибка ping базы данных или nil, если база ответила.
// workerRunning — true, когда фоновый monitor worker активен.
func evaluateHealth(dbErr error, workerRunning bool) healthCheckResult {
	result := healthCheckResult{
		OK:       true,
		Database: "ok",
		Worker:   "ok",
	}
	if dbErr != nil {
		result.OK = false
		result.Database = "unavailable" // ping PostgreSQL не прошёл
	}
	if !workerRunning {
		result.OK = false
		result.Worker = "not running" // worker не стартовал или уже остановлен
	}
	return result
}

// Health сообщает о готовности процесса для оркестраторов и reverse proxy.
// 200 OK — PostgreSQL отвечает на ping и monitor worker запущен: приложение может проверять URL и слать уведомления.
// 503 Service Unavailable — хотя бы одна проверка провалена; тело JSON описывает, что именно (без текста внутренних ошибок БД).
func (h *Handler) Health(c *gin.Context) {
	// Короткий таймаут — балансировщик не должен ждать долго.
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
		c.JSON(http.StatusServiceUnavailable, body) // 503 — инстанс снять с балансировки
		return
	}
	c.JSON(http.StatusOK, body)
}

// pingDatabase проверяет, что приложение может достичь PostgreSQL.
// ctx ограничивает время ожидания ping до сбоя.
func (h *Handler) pingDatabase(ctx context.Context) error {
	if h.DB == nil {
		return errHealthDatabaseMissing
	}
	sqlDB, err := h.DB.DB() // *sql.DB из пула GORM
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// errHealthDatabaseMissing возвращается, когда у обработчика нет GORM handle.
var errHealthDatabaseMissing = errors.New("database handle is not configured")
