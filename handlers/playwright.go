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

// Деструктивные REST-эндпоинты только для e2e (Playwright): SQL, очистка таблиц, создание пользователей.
// Регистрируются в server.go только при GO_UPTIME_ENABLE_PLAYWRIGHT_API=true и Environment=development.
// Не использовать в production — нет подтверждения и нет ограничений dev tools.

// PlaywrightSQLRequest — тело запроса для выполнения SQL в тестах.
type PlaywrightSQLRequest struct {
	Query string        `json:"query" binding:"required"`
	Args  []interface{} `json:"args"`
}

// PlaywrightClearTableRequest — тело запроса для очистки таблицы в тестах.
type PlaywrightClearTableRequest struct {
	Table string `json:"table" binding:"required"`
}

// PlaywrightCreateUserRequest — тело запроса для создания пользователя в тестах.
type PlaywrightCreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// PlaywrightExecuteSQL выполняет произвольный SQL без подтверждения — только для автотестов.
func (h *Handler) PlaywrightExecuteSQL(c *gin.Context) {
	var req PlaywrightSQLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Без таймаута и без подтверждения — e2e контролирует запросы сами.
	columns, rows, affected, err := database.ExecuteSQL(h.DB, req.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if columns != nil {
		// Результат SELECT.
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"columns": columns,
			"rows":    rows,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "rows_affected": affected})
}

// PlaywrightClearTable безвозвратно очищает таблицу — деструктивно, только e2e.
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

// PlaywrightCreateUser создаёт пользователя с заданным паролем — обход интерактивного CLI, только e2e.
func (h *Handler) PlaywrightCreateUser(c *gin.Context) {
	var req PlaywrightCreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := forms.CreateUserInput{
		Username:        req.Username,
		Password:        req.Password,
		ConfirmPassword: req.Password, // e2e передаёт один пароль — дублируем для Validate.
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

// PlaywrightSeedApplogRequest — тело запроса для заполнения in-memory applog-буферов в тестах.
type PlaywrightSeedApplogRequest struct {
	Kind  string `json:"kind" binding:"required"`
	Count int    `json:"count" binding:"required,min=1,max=200"`
}

// PlaywrightClearApplog очищает in-memory applog-буферы — только e2e, данные теряются без восстановления.
func (h *Handler) PlaywrightClearApplog(c *gin.Context) {
	applog.ClearAll() // in-memory буферы; перезапуск процесса тоже очищает
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// PlaywrightSeedApplog наполняет in-memory applog синтетическими записями для тестов пагинации — только e2e.
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
			// Неизвестный kind — отклоняем весь batch, ничего не пишем в буфер.
			c.JSON(http.StatusBadRequest, gin.H{"error": "kind must be events, errors, or requests"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
