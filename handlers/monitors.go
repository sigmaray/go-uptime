package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/internal/forms"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MonitorListItem is a monitor row with recent uptime check history for the admin list.
type MonitorListItem struct {
	models.MonitorURL
	HistoryBars []models.UptimeHistoryBar
	Uptime      models.MonitorUptime
}

// MonitorsList displays the list of monitored URLs with status.
func (h *Handler) MonitorsList(c *gin.Context) {
	page := parseQueryPage(c.Query("page"))
	perPage := models.AdminListPageSize
	filter := parseMonitorsListFilter(c)
	sort := ParseListSort(
		"/admin/monitors",
		models.MonitorURL{},
		"monitor_urls.created_at desc, monitor_urls.id asc",
		c.Query("sort"),
		c.Query("order"),
		"Name", "URL", "IsUp", "LastCheckedAt", "LastError",
	)
	sort.ExtraQuery = filter.QueryValues()
	now := time.Now()

	baseQuery := filter.Apply(h.DB.Model(&models.MonitorURL{}))

	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		applog.AddError("failed to count monitors", err.Error())
		total = 0
	}
	page = models.ClampPage(page, total, perPage)

	var monitors []models.MonitorURL
	// Rebuild the filtered query so Count does not leave statement state on the Find.
	query := sort.Apply(filter.Apply(h.DB.Model(&models.MonitorURL{})))
	if err := query.
		Offset(models.PageOffset(page, perPage)).
		Limit(perPage).
		Find(&monitors).Error; err != nil {
		applog.AddError("failed to load monitors", err.Error())
		monitors = nil
	}

	monitorIDs := make([]uint, 0, len(monitors))
	createdAtByID := make(map[uint]time.Time, len(monitors))
	for _, monitor := range monitors {
		monitorIDs = append(monitorIDs, monitor.ID)
		createdAtByID[monitor.ID] = monitor.CreatedAt
	}

	historyByMonitor, err := models.LoadUptimeHistoryBarsForMonitors(h.DB, monitorIDs, createdAtByID, now)
	if err != nil {
		applog.AddError("failed to load monitor uptime history", err.Error())
		historyByMonitor = map[uint][]models.UptimeHistoryBar{}
	}

	uptimeByMonitor, err := models.LoadMonitorUptimes(h.DB, monitorIDs, createdAtByID, now)
	if err != nil {
		applog.AddError("failed to load monitor uptime stats", err.Error())
		uptimeByMonitor = map[uint]models.MonitorUptime{}
	}

	items := make([]MonitorListItem, 0, len(monitors))
	for _, monitor := range monitors {
		items = append(items, MonitorListItem{
			MonitorURL:  monitor,
			HistoryBars: historyByMonitor[monitor.ID],
			Uptime:      uptimeByMonitor[monitor.ID],
		})
	}

	pagination := buildPaginationView(total, page, perPage, "Monitors", sort.PageURL)

	h.renderPage(c, http.StatusOK, "admin/monitors/index.html", gin.H{
		"Monitors":   items,
		"Pagination": pagination,
		"Sort":       sort,
		"Filter":     filter,
	}, PageOptions{Title: "Monitors", ActiveNav: "monitors"})
}

// NewMonitorPage displays the URL creation form.
func (h *Handler) NewMonitorPage(c *gin.Context) {
	_, notifyData, err := h.monitorNotificationContext()
	if err != nil {
		applog.AddError("failed to load notification settings", err.Error())
		notifyData = gin.H{
			"TelegramConfigured": false,
			"SMTPConfigured":     false,
		}
	}

	data := gin.H{}
	for key, value := range notifyData {
		data[key] = value
	}

	h.renderPage(c, http.StatusOK, "admin/monitors/new.html", data, PageOptions{
		Title:     "Add Monitor URL",
		ActiveNav: "monitors",
	})
}

// CreateMonitor handles URL creation.
func (h *Handler) CreateMonitor(c *gin.Context) {
	var input forms.MonitorURLInput
	if err := c.ShouldBind(&input); err != nil {
		_, notifyData, _ := h.monitorNotificationContext()
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/new.html", gin.H{
			"Error":              "Invalid form data",
			"Input":              input,
			"NotifyTelegram":     input.NotifyTelegram,
			"NotifySMTP":         input.NotifySMTP,
			"TelegramConfigured": notifyData["TelegramConfigured"],
			"SMTPConfigured":     notifyData["SMTPConfigured"],
		}, PageOptions{Title: "Add Monitor URL", ActiveNav: "monitors"})
		return
	}
	if err := h.bindMonitorNotificationFlags(c, &input); err != nil {
		applog.AddError("failed to load notification settings", err.Error())
	}
	if err := input.Validate(); err != nil {
		_, notifyData, _ := h.monitorNotificationContext()
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/new.html", gin.H{
			"Error":              forms.FormatValidationError(err),
			"Input":              input,
			"NotifyTelegram":     input.NotifyTelegram,
			"NotifySMTP":         input.NotifySMTP,
			"TelegramConfigured": notifyData["TelegramConfigured"],
			"SMTPConfigured":     notifyData["SMTPConfigured"],
		}, PageOptions{Title: "Add Monitor URL", ActiveNav: "monitors"})
		return
	}

	checkInterval, _ := input.ParseCheckIntervalSeconds()

	name := models.ResolveMonitorName(input.Name, input.URL)
	monitor := models.MonitorURL{
		Name:                 name,
		URL:                  input.URL,
		CheckIntervalSeconds: checkInterval,
		NotifyTelegram:       input.NotifyTelegram,
		NotifySMTP:           input.NotifySMTP,
	}
	if err := h.DB.Create(&monitor).Error; err != nil {
		h.renderPage(c, http.StatusInternalServerError, "admin/monitors/new.html", gin.H{
			"Error": "Failed to create monitor URL",
			"Input": input,
		}, PageOptions{Title: "Add Monitor URL", ActiveNav: "monitors"})
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Created monitor %q (%s)", monitor.Name, monitor.URL))
	redirectWithFlash(c, "/admin/monitors", flashSavedMessage)
}

// BulkNewMonitorPage displays the form for creating multiple monitored URLs at once.
// c is the Gin request context for the authenticated admin session.
func (h *Handler) BulkNewMonitorPage(c *gin.Context) {
	_, notifyData, err := h.monitorNotificationContext()
	if err != nil {
		applog.AddError("failed to load notification settings", err.Error())
		notifyData = gin.H{
			"TelegramConfigured": false,
			"SMTPConfigured":     false,
		}
	}

	data := gin.H{
		"Input": forms.MonitorURLBulkInput{},
	}
	for key, value := range notifyData {
		data[key] = value
	}

	h.renderPage(c, http.StatusOK, "admin/monitors/bulk_new.html", data, PageOptions{
		Title:     "Add multiple Monitor URLs",
		ActiveNav: "monitors",
	})
}

// bulkMonitorFormData builds template data for the bulk create form, including notification context.
// input is the submitted (or empty) bulk form.
// errMsg is a user-visible error string; empty when none.
func (h *Handler) bulkMonitorFormData(input forms.MonitorURLBulkInput, errMsg string) gin.H {
	_, notifyData, _ := h.monitorNotificationContext()
	return gin.H{
		"Error":              errMsg,
		"Input":              input,
		"NotifyTelegram":     input.NotifyTelegram,
		"NotifySMTP":         input.NotifySMTP,
		"TelegramConfigured": notifyData["TelegramConfigured"],
		"SMTPConfigured":     notifyData["SMTPConfigured"],
	}
}

// BulkCreateMonitors creates multiple monitors from a comma- or newline-separated URL list.
// Notification flags and optional check interval apply to every created monitor.
// Each monitor's Name is set to its URL.
// c is the Gin request context with the POST form body.
func (h *Handler) BulkCreateMonitors(c *gin.Context) {
	var input forms.MonitorURLBulkInput
	if err := c.ShouldBind(&input); err != nil {
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/bulk_new.html",
			h.bulkMonitorFormData(input, "Invalid form data"),
			PageOptions{Title: "Add multiple Monitor URLs", ActiveNav: "monitors"})
		return
	}
	if err := h.bindBulkMonitorNotificationFlags(c, &input); err != nil {
		applog.AddError("failed to load notification settings", err.Error())
	}
	if err := input.Validate(); err != nil {
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/bulk_new.html",
			h.bulkMonitorFormData(input, forms.FormatValidationError(err)),
			PageOptions{Title: "Add multiple Monitor URLs", ActiveNav: "monitors"})
		return
	}

	checkInterval, _ := input.ParseCheckIntervalSeconds()
	urls := input.ParsedURLs()

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, rawURL := range urls {
			monitor := models.MonitorURL{
				Name:                 rawURL,
				URL:                  rawURL,
				CheckIntervalSeconds: checkInterval,
				NotifyTelegram:       input.NotifyTelegram,
				NotifySMTP:           input.NotifySMTP,
			}
			if err := tx.Create(&monitor).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.renderPage(c, http.StatusInternalServerError, "admin/monitors/bulk_new.html",
			h.bulkMonitorFormData(input, "Failed to create monitor URLs"),
			PageOptions{Title: "Add multiple Monitor URLs", ActiveNav: "monitors"})
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Created %d monitors in bulk", len(urls)))
	redirectWithFlash(c, "/admin/monitors", flashSavedMessage)
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

	incidentsPage := parseQueryPage(c.Query("incidents_page"))
	heartbeatsPage := parseQueryPage(c.Query("heartbeats_page"))

	incidentTotal, err := models.CountIncidentsForMonitor(h.DB, monitor.ID)
	if err != nil {
		applog.AddError("failed to count monitor incidents", err.Error())
		incidentTotal = 0
	}
	incidentsPage = models.ClampPage(incidentsPage, incidentTotal, models.MonitorDetailListPageSize)

	heartbeatTotal, err := models.CountMonitorChecksForMonitor(h.DB, uint(id))
	if err != nil {
		applog.AddError("failed to count monitor heartbeats", err.Error())
		heartbeatTotal = 0
	}
	heartbeatsPage = models.ClampPage(heartbeatsPage, heartbeatTotal, models.MonitorDetailListPageSize)

	incidents, incidentsErr := models.LoadIncidentsForMonitorPage(h.DB, monitor.ID, incidentsPage, models.MonitorDetailListPageSize)
	if incidentsErr != nil {
		applog.AddError("failed to load monitor incidents", incidentsErr.Error())
		incidents = nil
	}

	heartbeats, err := models.LoadMonitorChecksPage(h.DB, uint(id), heartbeatsPage, models.MonitorDetailListPageSize)
	if err != nil {
		applog.AddError("failed to load monitor heartbeats", err.Error())
		heartbeats = nil
	}

	now := time.Now()

	historyBars, err := models.BuildUptimeHistoryBars(h.DB, uint(id), monitor.CreatedAt, now)
	if err != nil {
		applog.AddError("failed to load monitor uptime history", err.Error())
		historyBars = nil
	}

	uptime, err := models.LoadMonitorUptime(h.DB, uint(id), monitor.CreatedAt, now)
	if err != nil {
		applog.AddError("failed to load monitor uptime stats", err.Error())
	}

	incidentsPagination := buildPaginationView(incidentTotal, incidentsPage, models.MonitorDetailListPageSize, "Incidents", func(page int) string {
		return buildMonitorShowURL(monitor.ID, page, heartbeatsPage)
	})
	heartbeatsPagination := buildPaginationView(heartbeatTotal, heartbeatsPage, models.MonitorDetailListPageSize, "Heartbeat History", func(page int) string {
		return buildMonitorShowURL(monitor.ID, incidentsPage, page)
	})

	displayName := models.MonitorDisplayName(monitor)
	h.renderPage(c, http.StatusOK, "admin/monitors/show.html", gin.H{
		"Monitor":              monitor,
		"DisplayName":          displayName,
		"Heartbeats":           heartbeats,
		"HistoryBars":          historyBars,
		"Uptime":               uptime,
		"Incidents":            incidents,
		"IncidentsPagination":  incidentsPagination,
		"HeartbeatsPagination": heartbeatsPagination,
	}, PageOptions{Title: displayName, ActiveNav: "monitors"})
}

// EditMonitorPage displays the URL edit form.
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

	_, notifyData, notifyErr := h.monitorNotificationContext()
	if notifyErr != nil {
		applog.AddError("failed to load notification settings", notifyErr.Error())
		notifyData = gin.H{
			"TelegramConfigured": false,
			"SMTPConfigured":     false,
		}
	}

	data := gin.H{"Monitor": monitor}
	for key, value := range notifyData {
		data[key] = value
	}
	data["NotifyTelegram"] = monitor.NotifyTelegram
	data["NotifySMTP"] = monitor.NotifySMTP

	h.renderPage(c, http.StatusOK, "admin/monitors/edit.html", data, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
}

// UpdateMonitor handles URL updates.
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

	var input forms.MonitorURLInput
	if err := c.ShouldBind(&input); err != nil {
		_, notifyData, _ := h.monitorNotificationContext()
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/edit.html", gin.H{
			"Error":              "Invalid form data",
			"Monitor":            monitor,
			"NotifyTelegram":     monitor.NotifyTelegram,
			"NotifySMTP":         monitor.NotifySMTP,
			"TelegramConfigured": notifyData["TelegramConfigured"],
			"SMTPConfigured":     notifyData["SMTPConfigured"],
		}, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
		return
	}
	if err := h.bindMonitorNotificationFlags(c, &input); err != nil {
		applog.AddError("failed to load notification settings", err.Error())
	}
	if err := input.Validate(); err != nil {
		monitor.Name = input.Name
		monitor.URL = input.URL
		monitor.CheckIntervalSeconds = nil
		if parsed, parseErr := input.ParseCheckIntervalSeconds(); parseErr == nil {
			monitor.CheckIntervalSeconds = parsed
		}
		_, notifyData, _ := h.monitorNotificationContext()
		h.renderPage(c, http.StatusBadRequest, "admin/monitors/edit.html", gin.H{
			"Error":              forms.FormatValidationError(err),
			"Monitor":            monitor,
			"NotifyTelegram":     input.NotifyTelegram,
			"NotifySMTP":         input.NotifySMTP,
			"TelegramConfigured": notifyData["TelegramConfigured"],
			"SMTPConfigured":     notifyData["SMTPConfigured"],
		}, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
		return
	}

	checkInterval, _ := input.ParseCheckIntervalSeconds()

	monitor.Name = models.ResolveMonitorName(input.Name, input.URL)
	monitor.URL = input.URL
	monitor.CheckIntervalSeconds = checkInterval
	monitor.NotifyTelegram = input.NotifyTelegram
	monitor.NotifySMTP = input.NotifySMTP
	if err := h.DB.Save(&monitor).Error; err != nil {
		h.renderPage(c, http.StatusInternalServerError, "admin/monitors/edit.html", gin.H{
			"Error":   "Failed to update monitor URL",
			"Monitor": monitor,
		}, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Updated monitor %q (%s)", monitor.Name, monitor.URL))
	redirectWithFlash(c, "/admin/monitors", flashSavedMessage)
}

// DeleteMonitor permanently removes a monitored URL and cascaded related rows.
// c is the Gin request context with the monitor id path parameter.
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
	redirectWithFlash(c, "/admin/monitors", flashDeletedMessage)
}
