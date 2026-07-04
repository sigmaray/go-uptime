package handlers

import (
	"fmt"
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// LoginPage отображает форму входа.
func (h *Handler) LoginPage(c *gin.Context) {
	session := sessions.Default(c)
	if session.Get("user") != nil {
		c.Redirect(http.StatusFound, "/admin/")
		return
	}
	h.renderPage(c, http.StatusOK, "admin/login/index.html", gin.H{}, PageOptions{
		Title:     "Login",
		HideNav:   true,
		BodyClass: "bg-light",
	})
}

// Login обрабатывает отправку формы входа.
func (h *Handler) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	user, err := models.FindUserByUsername(h.DB, username)
	if err != nil || user == nil || !models.CheckPassword(user.PasswordHash, password) {
		h.renderPage(c, http.StatusOK, "admin/login/index.html", gin.H{
			"Error": "Invalid username or password",
		}, PageOptions{
			Title:     "Login",
			HideNav:   true,
			BodyClass: "bg-light",
		})
		return
	}

	session := sessions.Default(c)
	session.Set("user", user.Username)
	_ = session.Save()
	applog.AddEvent("auth", fmt.Sprintf("User %q logged in", user.Username))
	c.Redirect(http.StatusFound, "/admin/")
}

// Logout завершает сессию пользователя.
func (h *Handler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	username, _ := session.Get("user").(string)
	session.Clear()
	_ = session.Save()
	if username != "" {
		applog.AddEvent("auth", fmt.Sprintf("User %q logged out", username))
	}
	c.Redirect(http.StatusFound, "/")
}
