package handlers

import (
	"html/template"

	"go-uptime/config"

	"gorm.io/gorm"
)

// Handler holds HTTP handler dependencies.
type Handler struct {
	DB        *gorm.DB
	Templates *template.Template
	Config    *config.Config
}

// NewHandler creates a new Handler instance.
func NewHandler(db *gorm.DB, tmpl *template.Template, cfg *config.Config) *Handler {
	return &Handler{DB: db, Templates: tmpl, Config: cfg}
}
