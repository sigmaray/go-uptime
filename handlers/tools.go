package handlers

import (
	"context"
	"net/http"
	"time"

	"go-uptime/database"
	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// ToolsPage отображает страницу инструментов разработчика.
func (h *Handler) ToolsPage(c *gin.Context) {
	tables, err := database.ListTables(h.DB)
	if err != nil {
		applog.AddError("failed to list tables", err.Error())
		tables = nil
	}

	h.renderPage(c, http.StatusOK, "admin/tools/index.html", gin.H{
		"Tables": tables,
	}, PageOptions{Title: "Dev Tools", ActiveNav: "tools"})
}

type toolsClearTableRequest struct {
	Table string `json:"table" binding:"required"`
}

type toolsSQLRequest struct {
	Query string `json:"query" binding:"required"`
}

// ToolsClearTable очищает выбранную таблицу (только development).
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

// ToolsExecuteSQL выполняет SQL с таймаутом 1 минуту и возвращает результат JSON.
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

// ToolsSeedMonitors создаёт демонстрационные URL для мониторинга.
func (h *Handler) ToolsSeedMonitors(c *gin.Context) {
	created, err := models.SeedMonitorURLs(h.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"created": created})
}
