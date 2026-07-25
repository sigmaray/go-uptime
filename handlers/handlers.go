// Package handlers implements Gin HTTP handlers for the admin UI and APIs.
package handlers

import (
	"html/template"

	"go-uptime/config"
	"go-uptime/worker"

	"gorm.io/gorm"
)

// Handler holds HTTP handler dependencies.
type Handler struct {
	DB        *gorm.DB
	Templates *template.Template
	Config    *config.Config
	// Worker supplies live check-wave stats for the admin info page; may be nil in tests.
	Worker *worker.MonitorWorker
}

// NewHandler creates a new Handler instance.
// db is the GORM handle for request handlers.
// tmpl is the parsed HTML template set.
// cfg is application configuration.
// monitorWorker is the background checker used for live queue metrics; may be nil.
func NewHandler(db *gorm.DB, tmpl *template.Template, cfg *config.Config, monitorWorker *worker.MonitorWorker) *Handler {
	return &Handler{DB: db, Templates: tmpl, Config: cfg, Worker: monitorWorker}
}
