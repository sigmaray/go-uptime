package handlers

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Одноразовые flash-сообщения после redirect (Post/Redirect/Get):
// успех сохранения/удаления показывается на следующей странице без дублирования при F5.

const (
	flashSavedMessage   = "Saved successfully."
	flashDeletedMessage = "Deleted successfully."
)

// setFlash сохраняет одноразовое сообщение в сессии для следующей отрисованной страницы.
// Паттерн PRG: после POST редирект несёт сообщение, consumeFlash снимает его при первом GET.
func setFlash(c *gin.Context, message string) {
	session := sessions.Default(c)
	session.AddFlash(message) // gin-contrib/sessions кладёт в cookie-сессию
	_ = session.Save()
}

// consumeFlash возвращает и очищает первое flash-сообщение из сессии.
func consumeFlash(c *gin.Context) string {
	session := sessions.Default(c)
	flashes := session.Flashes() // чтение уже удаляет flash из сессии
	if len(flashes) == 0 {
		return ""
	}
	if err := session.Save(); err != nil {
		log.Error().Err(err).Msg("failed to save session after consuming flash message")
	}
	if message, ok := flashes[0].(string); ok {
		return message
	}
	return "" // неожиданный тип во flash — игнорируем
}

// redirectWithFlash перенаправляет на url после сохранения flash-сообщения в сессии.
func redirectWithFlash(c *gin.Context, url, message string) {
	setFlash(c, message)
	c.Redirect(http.StatusFound, url) // 302 — классический PRG после POST
}
