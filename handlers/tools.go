package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go-uptime/database"
	"go-uptime/internal/applog"
	"go-uptime/internal/notify"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// ToolsPage отображает страницу инструментов разработчика.
func (h *Handler) ToolsPage(c *gin.Context) {
	tables, err := database.ListTables(h.DB)
	if err != nil {
		applog.AddError("failed to list tables", err.Error())
		tables = nil // страница откроется, но без списка таблиц
	}

	notifications, notifyErr := models.LoadNotificationSettings(h.DB)
	if notifyErr != nil {
		applog.AddError("failed to load notification settings", notifyErr.Error())
	}

	h.renderPage(c, http.StatusOK, "admin/tools/index.html", gin.H{
		"Tables":             tables,
		"TelegramConfigured": notifications.TelegramConfigured(),
		"SMTPConfigured":     notifications.SMTPConfigured(),
	}, PageOptions{Title: "Dev Tools", ActiveNav: "tools"})
}

type toolsClearTableRequest struct {
	Table string `json:"table" binding:"required"`
}

type toolsSQLRequest struct {
	Query string `json:"query" binding:"required"`
}

// ToolsClearTable очищает выбранную таблицу (только для разработки).
func (h *Handler) ToolsClearTable(c *gin.Context) {
	var req toolsClearTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// TRUNCATE/CASCADE — только whitelist таблиц в database.ClearTable.
	if err := database.ClearTable(h.DB, req.Table); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ToolsExecuteSQL выполняет SQL с таймаутом 1 минута и возвращает результат как JSON.
// Долгий запрос уходит в goroutine; select ждёт результат или ctx.Done(), чтобы HTTP handler
// не зависал бесконечно, если БД не отвечает.
func (h *Handler) ToolsExecuteSQL(c *gin.Context) {
	var req toolsSQLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()

	type result struct {
		columns  []string
		rows     [][]string
		affected int64
		err      error
	}
	ch := make(chan result, 1)
	// SQL выполняется в отдельной goroutine — select ниже ограничивает время ожидания handler'а.
	go func() {
		cols, rows, affected, err := database.ExecuteSQL(h.DB.WithContext(ctx), req.Query)
		ch <- result{columns: cols, rows: rows, affected: affected, err: err}
	}()

	select {
	case <-ctx.Done():
		// Таймаут сработал раньше, чем goroutine вернула результат.
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "query timeout exceeded"})
		return
	case res := <-ch:
		if res.err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": res.err.Error()})
			return
		}
		if res.columns != nil {
			// SELECT — возвращаем колонки и строки.
			c.JSON(http.StatusOK, gin.H{
				"columns": res.columns,
				"rows":    res.rows,
			})
			return
		}
		// INSERT/UPDATE/DELETE — только rows_affected.
		c.JSON(http.StatusOK, gin.H{
			"rows_affected": res.affected,
		})
	}
}

// ToolsSeedMonitors создаёт демонстрационные URL для мониторинга.
func (h *Handler) ToolsSeedMonitors(c *gin.Context) {
	created, err := models.SeedMonitorURLs(h.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"created": created})
}

// ToolsTestTelegram отправляет тестовое Telegram-уведомление (только для разработки).
func (h *Handler) ToolsTestTelegram(c *gin.Context) {
	settings, err := models.LoadNotificationSettings(h.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := notify.SendTestTelegram(settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ToolsTestSMTP отправляет тестовое email-уведомление (только для разработки).
func (h *Handler) ToolsTestSMTP(c *gin.Context) {
	settings, err := models.LoadNotificationSettings(h.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := notify.SendTestSMTP(settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ToolsTestError добавляет тестовую запись об ошибке в память (только для разработки).
func (h *Handler) ToolsTestError(c *gin.Context) {
	applog.AddError("test error from dev tools", "manual trigger")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ToolsTestLog добавляет тестовое событие приложения в память (только для разработки).
func (h *Handler) ToolsTestLog(c *gin.Context) {
	message := fmt.Sprintf("test event from dev tools at %s", time.Now().Format(time.RFC3339))
	applog.AddEvent("dev-tools", message)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": message})
}
