// Package handlers реализует Gin HTTP-обработчики для админ UI и API.
package handlers

import (
	"html/template"

	"go-uptime/config"
	"go-uptime/worker"

	"gorm.io/gorm"
)

// Handler содержит зависимости HTTP-обработчиков.
type Handler struct {
	DB        *gorm.DB
	Templates *template.Template
	Config    *config.Config
	// Worker предоставляет live check-wave stats для страницы info админки; может быть nil в тестах.
	Worker *worker.MonitorWorker
}

// NewHandler создаёт новый экземпляр Handler.
// db — GORM handle для обработчиков запросов.
// tmpl — набор распарсенных HTML-шаблонов.
// cfg — конфигурация приложения.
// monitorWorker — фоновый checker для live queue metrics; может быть nil.
func NewHandler(db *gorm.DB, tmpl *template.Template, cfg *config.Config, monitorWorker *worker.MonitorWorker) *Handler {
	return &Handler{DB: db, Templates: tmpl, Config: cfg, Worker: monitorWorker}
}
