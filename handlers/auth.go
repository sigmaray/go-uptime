package handlers

import (
	"fmt"
	"net/http"

	"go-uptime/internal/applog"
	"go-uptime/internal/repository"
	"go-uptime/middlewares"
	"go-uptime/models"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// LoginPage отображает форму входа.
func (h *Handler) LoginPage(c *gin.Context) {
	session := sessions.Default(c)
	if session.Get("user") != nil {
		// Уже авторизован — не показываем форму входа повторно.
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
	// Сначала rate limit по IP — до обращения к БД и проверки пароля.
	if !middlewares.AllowLoginAttempt(c.ClientIP()) {
		h.renderPage(c, http.StatusTooManyRequests, "admin/login/index.html", gin.H{
			"Error": "Too many login attempts. Please try again later.",
		}, PageOptions{
			Title:     "Login",
			HideNav:   true,
			BodyClass: "bg-light",
		})
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")

	repo := repository.NewUserRepository(h.DB)
	user, err := repo.FindUserByUsername(username)
	// Одинаковая ошибка для «пользователь не найден» и «неверный пароль» — без перечисления имён (user enumeration).
	if err != nil || user == nil || !models.CheckPassword(user.PasswordHash, password) {
		// 200 OK + текст ошибки — не раскрываем, существует ли username.
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
	// В cookie-сессии храним username; отдельного server-side session store нет.
	session.Set("user", user.Username)
	_ = session.Save()
	applog.AddEvent("auth", fmt.Sprintf("User %q logged in", user.Username))
	c.Redirect(http.StatusFound, "/admin/")
}

// Logout завершает текущую сессию пользователя.
func (h *Handler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	username, _ := session.Get("user").(string)
	// Очищаем cookie-сессию; инвалидировать на сервере нечего — хранилища сессий нет.
	session.Clear()
	_ = session.Save()
	if username != "" {
		applog.AddEvent("auth", fmt.Sprintf("User %q logged out", username))
	}
	c.Redirect(http.StatusFound, "/")
}
