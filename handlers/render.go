package handlers

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// PageOptions содержит параметры отрисовки layout для HTML-страницы.
type PageOptions struct {
	Title     string
	ActiveNav string
	HideNav   bool
	BodyClass string
	Success   string
}

// renderPage отрисовывает страницу через общий layout админки.
// Двухфазный рендер: сначала contentTmpl в буфер, затем layout с Content как доверенным HTML.
// Error извлекается из data и передаётся layout — flash/ошибки рисует только оболочка.
// Success берётся из opts.Success или из session flash после redirect.
func (h *Handler) renderPage(c *gin.Context, status int, contentTmpl string, data gin.H, opts PageOptions) {
	if data == nil {
		data = gin.H{}
	}

	// Error не должен попадать в шаблон контента — layout централизует блок ошибок.
	var errMsg string
	if v, ok := data["Error"]; ok {
		if s, ok := v.(string); ok {
			errMsg = s
		}
		delete(data, "Error")
	}

	successMsg := opts.Success
	if successMsg == "" {
		successMsg = consumeFlash(c) // одноразовое сообщение после PRG-redirect
	}

	session := sessions.Default(c)
	currentUser, _ := session.Get("user").(string)

	// Фаза 1: тело страницы в буфер (ещё не HTTP-ответ).
	var contentBuf bytes.Buffer
	if err := h.Templates.ExecuteTemplate(&contentBuf, contentTmpl, data); err != nil {
		log.Error().Err(err).Str("template", contentTmpl).Msg("failed to render page content")
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	// Фаза 2: layout вставляет готовый HTML контента (template.HTML — доверенный вывод шаблонов).
	c.HTML(status, "admin/layout.html", gin.H{
		"Title":     opts.Title,
		"ActiveNav": opts.ActiveNav,
		"HideNav":   opts.HideNav,
		"BodyClass": opts.BodyClass,
		"Error":     errMsg,
		"Success":   successMsg,
		// Content отрисовывается из доверенных серверных шаблонов, а не из сырого пользовательского ввода.
		"Content":     template.HTML(contentBuf.String()), //nolint:gosec // G203: trusted template output
		"CurrentUser": currentUser,
		"IsDev":       h.Config.IsDevelopment(), // показывает ссылку на dev tools в nav
	})
}
