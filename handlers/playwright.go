package handlers

import (
	"fmt"
	"net/http"

	"go-uptime/database"
	"go-uptime/internal/applog"
	"go-uptime/internal/forms"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// PlaywrightSQLRequest is the request body for executing SQL in tests.
type PlaywrightSQLRequest struct {
	Query string        `json:"query" binding:"required"`
	Args  []interface{} `json:"args"`
}

// PlaywrightClearTableRequest is the request body for clearing a table in tests.
type PlaywrightClearTableRequest struct {
	Table string `json:"table" binding:"required"`
}

// PlaywrightCreateUserRequest is the request body for creating a user in tests.
type PlaywrightCreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// PlaywrightExecuteSQL executes SQL for Playwright tests.
func (h *Handler) PlaywrightExecuteSQL(c *gin.Context) {
	var req PlaywrightSQLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	columns, rows, affected, err := database.ExecuteSQL(h.DB, req.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if columns != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"columns": columns,
			"rows":    rows,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "rows_affected": affected})
}

// PlaywrightClearTable clears a table for Playwright tests.
func (h *Handler) PlaywrightClearTable(c *gin.Context) {
	var req PlaywrightClearTableRequest
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

// PlaywrightCreateUser creates a user for Playwright tests.
func (h *Handler) PlaywrightCreateUser(c *gin.Context) {
	var req PlaywrightCreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := forms.CreateUserInput{
		Username:        req.Username,
		Password:        req.Password,
		ConfirmPassword: req.Password,
	}
	if err := input.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": forms.FormatValidationError(err)})
		return
	}

	user, err := models.CreateUser(h.DB, input.Username, input.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
		},
	})
}

// PlaywrightSeedApplogRequest is the request body for seeding in-memory applog buffers in tests.
type PlaywrightSeedApplogRequest struct {
	Kind  string `json:"kind" binding:"required"`
	Count int    `json:"count" binding:"required,min=1,max=200"`
}

// PlaywrightClearApplog clears in-memory applog buffers for Playwright tests.
func (h *Handler) PlaywrightClearApplog(c *gin.Context) {
	applog.ClearAll()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// PlaywrightSeedApplog adds entries to in-memory applog buffers for Playwright tests.
func (h *Handler) PlaywrightSeedApplog(c *gin.Context) {
	var req PlaywrightSeedApplogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for i := 1; i <= req.Count; i++ {
		switch req.Kind {
		case "events":
			applog.AddEvent("test", fmt.Sprintf("pagination event %d", i))
		case "errors":
			applog.AddError(fmt.Sprintf("pagination error %d", i), "test")
		case "requests":
			applog.AddMonitorRequest("Pagination Test", "https://pagination.example.com", 200, int64(i), true, "")
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "kind must be events, errors, or requests"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
