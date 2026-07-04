package handlers

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// PageOptions — параметры отображения страницы в layout.
type PageOptions struct {
	Title     string
	ActiveNav string
	HideNav   bool
	BodyClass string
}

// renderPage рендерит страницу через общий layout.
func (h *Handler) renderPage(c *gin.Context, status int, contentTmpl string, data gin.H, opts PageOptions) {
	if data == nil {
		data = gin.H{}
	}

	var errMsg string
	if v, ok := data["Error"]; ok {
		if s, ok := v.(string); ok {
			errMsg = s
		}
		delete(data, "Error")
	}

	session := sessions.Default(c)
	currentUser, _ := session.Get("user").(string)

	var contentBuf bytes.Buffer
	if err := h.Templates.ExecuteTemplate(&contentBuf, contentTmpl, data); err != nil {
		log.Error().Err(err).Str("template", contentTmpl).Msg("failed to render page content")
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	c.HTML(status, "admin/layout.html", gin.H{
		"Title":       opts.Title,
		"ActiveNav":   opts.ActiveNav,
		"HideNav":     opts.HideNav,
		"BodyClass":   opts.BodyClass,
		"Error":       errMsg,
		"Content":     template.HTML(contentBuf.String()),
		"CurrentUser": currentUser,
		"IsDev":       h.Config.IsDevelopment(),
	})
}
