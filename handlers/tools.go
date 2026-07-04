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

// ToolsPage displays the developer tools page.
func (h *Handler) ToolsPage(c *gin.Context) {
	tables, err := database.ListTables(h.DB)
	if err != nil {
		applog.AddError("failed to list tables", err.Error())
		tables = nil
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

// ToolsClearTable clears the selected table (development only).
func (h *Handler) ToolsClearTable(c *gin.Context) {
	var req toolsClearTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := database.ClearTable(h.DB, req.Table); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ToolsExecuteSQL runs SQL with a 1-minute timeout and returns the result as JSON.
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
	go func() {
		cols, rows, affected, err := database.ExecuteSQL(h.DB.WithContext(ctx), req.Query)
		ch <- result{columns: cols, rows: rows, affected: affected, err: err}
	}()

	select {
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "query timeout exceeded"})
		return
	case res := <-ch:
		if res.err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": res.err.Error()})
			return
		}
		if res.columns != nil {
			c.JSON(http.StatusOK, gin.H{
				"columns": res.columns,
				"rows":    res.rows,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"rows_affected": res.affected,
		})
	}
}

// ToolsSeedMonitors creates demonstration URLs for monitoring.
func (h *Handler) ToolsSeedMonitors(c *gin.Context) {
	created, err := models.SeedMonitorURLs(h.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"created": created})
}

// ToolsTestTelegram sends a test Telegram notification (development only).
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

// ToolsTestSMTP sends a test email notification (development only).
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

// ToolsTestError adds a test error record to memory (development only).
func (h *Handler) ToolsTestError(c *gin.Context) {
	applog.AddError("test error from dev tools", "manual trigger")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ToolsTestLog adds a test application event to memory (development only).
func (h *Handler) ToolsTestLog(c *gin.Context) {
	message := fmt.Sprintf("test event from dev tools at %s", time.Now().Format(time.RFC3339))
	applog.AddEvent("dev-tools", message)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": message})
}
