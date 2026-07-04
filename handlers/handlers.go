package handlers

import (
	"html/template"

	"go-uptime/config"

	"gorm.io/gorm"
)

// Handler содержит зависимости HTTP-обработчиков.
type Handler struct {
	DB        *gorm.DB
	Templates *template.Template
	Config    *config.Config
}

// NewHandler создаёт новый экземпляр Handler.
func NewHandler(db *gorm.DB, tmpl *template.Template, cfg *config.Config) *Handler {
	return &Handler{DB: db, Templates: tmpl, Config: cfg}
}
