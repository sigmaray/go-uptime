package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// UsersList displays the user list.
func (h *Handler) UsersList(c *gin.Context) {
	var users []models.User
	h.DB.Order("created_at desc").Find(&users)

	h.renderPage(c, http.StatusOK, "admin/users/index.html", gin.H{
		"Users": users,
	}, PageOptions{Title: "Users", ActiveNav: "users"})
}

// NewUserPage displays the user creation form.
func (h *Handler) NewUserPage(c *gin.Context) {
	h.renderPage(c, http.StatusOK, "admin/users/new.html", gin.H{}, PageOptions{
		Title:     "Create User",
		ActiveNav: "users",
	})
}

// CreateUser handles user creation.
func (h *Handler) CreateUser(c *gin.Context) {
	var input models.CreateUserInput
	if err := c.ShouldBind(&input); err != nil {
		h.renderPage(c, http.StatusBadRequest, "admin/users/new.html", gin.H{
			"Error":    "Invalid form data",
			"Username": input.Username,
		}, PageOptions{Title: "Create User", ActiveNav: "users"})
		return
	}
	if err := input.Validate(); err != nil {
		h.renderPage(c, http.StatusBadRequest, "admin/users/new.html", gin.H{
			"Error":    models.FormatValidationError(err),
			"Username": input.Username,
		}, PageOptions{Title: "Create User", ActiveNav: "users"})
		return
	}

	_, err := models.CreateUser(h.DB, input.Username, input.Password)
	if err != nil {
		h.renderPage(c, http.StatusInternalServerError, "admin/users/new.html", gin.H{
			"Error":    "Failed to create user (maybe username already exists)",
			"Username": input.Username,
		}, PageOptions{Title: "Create User", ActiveNav: "users"})
		return
	}

	applog.AddEvent("user", fmt.Sprintf("Created user %q", input.Username))
	redirectWithFlash(c, "/admin/users", flashSavedMessage)
}

// EditUserPage displays the user edit form.
func (h *Handler) EditUserPage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var user models.User
	if err := h.DB.First(&user, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	h.renderPage(c, http.StatusOK, "admin/users/edit.html", gin.H{
		"User": user,
	}, PageOptions{Title: "Edit User", ActiveNav: "users"})
}

// UpdateUser handles user updates.
func (h *Handler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var user models.User
	if err := h.DB.First(&user, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var input models.UpdateUserInput
	if err := c.ShouldBind(&input); err != nil {
		h.renderPage(c, http.StatusBadRequest, "admin/users/edit.html", gin.H{
			"Error": "Invalid form data",
			"User":  user,
		}, PageOptions{Title: "Edit User", ActiveNav: "users"})
		return
	}
	if err := input.Validate(); err != nil {
		user.Username = input.Username
		h.renderPage(c, http.StatusBadRequest, "admin/users/edit.html", gin.H{
			"Error": models.FormatValidationError(err),
			"User":  user,
		}, PageOptions{Title: "Edit User", ActiveNav: "users"})
		return
	}

	user.Username = input.Username
	if input.Password != "" {
		hash, err := models.HashPassword(input.Password)
		if err != nil {
			h.renderPage(c, http.StatusInternalServerError, "admin/users/edit.html", gin.H{
				"Error": "Failed to hash password",
				"User":  user,
			}, PageOptions{Title: "Edit User", ActiveNav: "users"})
			return
		}
		user.PasswordHash = hash
	}

	if err := h.DB.Save(&user).Error; err != nil {
		h.renderPage(c, http.StatusInternalServerError, "admin/users/edit.html", gin.H{
			"Error": "Failed to update user",
			"User":  user,
		}, PageOptions{Title: "Edit User", ActiveNav: "users"})
		return
	}

	applog.AddEvent("user", fmt.Sprintf("Updated user %q", user.Username))
	redirectWithFlash(c, "/admin/users", flashSavedMessage)
}

// DeleteUser deletes a user.
func (h *Handler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var user models.User
	if err := h.DB.First(&user, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	if err := h.DB.Delete(&models.User{}, id).Error; err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	applog.AddEvent("user", fmt.Sprintf("Deleted user %q", user.Username))
	redirectWithFlash(c, "/admin/users", flashDeletedMessage)
}
