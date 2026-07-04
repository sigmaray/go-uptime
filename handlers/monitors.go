package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
)

// MonitorListItem is a monitor row with recent uptime check history for the admin list.
type MonitorListItem struct {
	models.MonitorURL
	RecentChecks []models.MonitorCheck
}

// MonitorsList отображает список мониторируемых URL со статусом.
func (h *Handler) MonitorsList(c *gin.Context) {
	var monitors []models.MonitorURL
	h.DB.Order("created_at desc").Find(&monitors)

	since := time.Now().Add(-30 * time.Minute)
	checksByMonitor, err := models.LoadMonitorChecksSince(h.DB, since)
	if err != nil {
		applog.AddError("failed to load monitor check history", err.Error())
		checksByMonitor = map[uint][]models.MonitorCheck{}
	}

	items := make([]MonitorListItem, 0, len(monitors))
	for _, monitor := range monitors {
		items = append(items, MonitorListItem{
			MonitorURL:   monitor,
			RecentChecks: checksByMonitor[monitor.ID],
		})
	}

	h.renderPage(c, http.StatusOK, "admin/monitors/index.html", gin.H{
		"Monitors": items,
	}, PageOptions{Title: "Monitors", ActiveNav: "monitors"})
}

// NewMonitorPage отображает форму создания URL.
func (h *Handler) NewMonitorPage(c *gin.Context) {
	h.renderPage(c, http.StatusOK, "admin/monitors/new.html", gin.H{}, PageOptions{
		Title:     "Add Monitor URL",
		ActiveNav: "monitors",
	})
}

// CreateMonitor обрабатывает создание URL.
func (h *Handler) CreateMonitor(c *gin.Context) {
	var input models.MonitorURLInput
	if err := c.ShouldBind(&input); err != nil {
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/new.html", gin.H{
			"Error": "Invalid form data",
			"Input": input,
		}, PageOptions{Title: "Add Monitor URL", ActiveNav: "monitors"})
		return
	}
	if err := input.Validate(); err != nil {
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/new.html", gin.H{
			"Error": models.FormatValidationError(err),
			"Input": input,
		}, PageOptions{Title: "Add Monitor URL", ActiveNav: "monitors"})
		return
	}

	name := models.ResolveMonitorName(input.Name, input.URL)
	monitor := models.MonitorURL{Name: name, URL: input.URL}
	if err := h.DB.Create(&monitor).Error; err != nil {
		h.renderPage(c, http.StatusInternalServerError, "admin/monitors/new.html", gin.H{
			"Error": "Failed to create monitor URL",
			"Input": input,
		}, PageOptions{Title: "Add Monitor URL", ActiveNav: "monitors"})
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Created monitor %q (%s)", monitor.Name, monitor.URL))
	c.Redirect(http.StatusFound, "/admin/monitors")
}

// MonitorShowPage renders monitor details and heartbeat history.
func (h *Handler) MonitorShowPage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var monitor models.MonitorURL
	if err := h.DB.First(&monitor, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	heartbeats, err := models.LoadMonitorChecks(h.DB, uint(id), models.MaxMonitorDetailHeartbeats)
	if err != nil {
		applog.AddError("failed to load monitor heartbeats", err.Error())
		heartbeats = nil
	}

	since := time.Now().Add(-30 * time.Minute)
	checksByMonitor, err := models.LoadMonitorChecksSince(h.DB, since)
	if err != nil {
		applog.AddError("failed to load monitor check history", err.Error())
	}
	recentChecks := checksByMonitor[uint(id)]

	displayName := models.MonitorDisplayName(monitor)
	h.renderPage(c, http.StatusOK, "admin/monitors/show.html", gin.H{
		"Monitor":      monitor,
		"DisplayName":  displayName,
		"Heartbeats":   heartbeats,
		"RecentChecks": recentChecks,
	}, PageOptions{Title: displayName, ActiveNav: "monitors"})
}

// EditMonitorPage отображает форму редактирования URL.
func (h *Handler) EditMonitorPage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var monitor models.MonitorURL
	if err := h.DB.First(&monitor, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	h.renderPage(c, http.StatusOK, "admin/monitors/edit.html", gin.H{
		"Monitor": monitor,
	}, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
}

// UpdateMonitor обрабатывает обновление URL.
func (h *Handler) UpdateMonitor(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var monitor models.MonitorURL
	if err := h.DB.First(&monitor, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var input models.MonitorURLInput
	if err := c.ShouldBind(&input); err != nil {
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/edit.html", gin.H{
			"Error":   "Invalid form data",
			"Monitor": monitor,
		}, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
		return
	}
	if err := input.Validate(); err != nil {
		monitor.Name = input.Name
		monitor.URL = input.URL
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/edit.html", gin.H{
			"Error":   models.FormatValidationError(err),
			"Monitor": monitor,
		}, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
		return
	}

	monitor.Name = models.ResolveMonitorName(input.Name, input.URL)
	monitor.URL = input.URL
	if err := h.DB.Save(&monitor).Error; err != nil {
		h.renderPage(c, http.StatusInternalServerError, "admin/monitors/edit.html", gin.H{
			"Error":   "Failed to update monitor URL",
			"Monitor": monitor,
		}, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Updated monitor %q (%s)", monitor.Name, monitor.URL))
	c.Redirect(http.StatusFound, "/admin/monitors")
}

// DeleteMonitor удаляет URL из мониторинга.
func (h *Handler) DeleteMonitor(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var monitor models.MonitorURL
	if err := h.DB.First(&monitor, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	if err := h.DB.Delete(&models.MonitorURL{}, id).Error; err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Deleted monitor %q (%s)", models.MonitorDisplayName(monitor), monitor.URL))
	c.Redirect(http.StatusFound, "/admin/monitors")
}
