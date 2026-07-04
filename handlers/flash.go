package handlers

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	flashSavedMessage   = "Saved successfully."
	flashDeletedMessage = "Deleted successfully."
)

// setFlash stores a one-time message in the session for the next rendered page.
func setFlash(c *gin.Context, message string) {
	session := sessions.Default(c)
	session.AddFlash(message)
	_ = session.Save()
}

// consumeFlash returns and clears the first flash message from the session.
func consumeFlash(c *gin.Context) string {
	session := sessions.Default(c)
	flashes := session.Flashes()
	if len(flashes) == 0 {
		return ""
	}
	if message, ok := flashes[0].(string); ok {
		return message
	}
	return ""
}

// redirectWithFlash redirects to url after storing a flash message in the session.
func redirectWithFlash(c *gin.Context, url, message string) {
	setFlash(c, message)
	c.Redirect(http.StatusFound, url)
}
