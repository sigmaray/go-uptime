package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/internal/forms"
	monitorsvc "go-uptime/internal/services/monitor"
	"go-uptime/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MonitorListItem — строка монитора с недавней историей uptime-проверок для списка админки.
type MonitorListItem struct {
	models.MonitorURL
	HistoryBars []models.UptimeHistoryBar
	Uptime      models.MonitorUptime
}

// MonitorsList отображает список мониторимых URL со статусом.
func (h *Handler) MonitorsList(c *gin.Context) {
	// Параметры списка: страница, фильтр, сортировка.
	page := parseQueryPage(c.Query("page"))
	perPage := models.AdminListPageSize
	filter := parseMonitorsListFilter(c)
	sort := ParseListSort(
		"/admin/monitors",
		models.MonitorURL{},
		"monitor_urls.created_at desc, monitor_urls.id asc",
		c.Query("sort"),
		c.Query("order"),
		"ID", "URL", "IsUp", "LastCheckedAt", "LastError",
	)
	sort.ExtraQuery = filter.QueryValues() // фильтры сохраняются в ссылках сортировки/пагинации
	now := time.Now()

	baseQuery := filter.Apply(h.DB.Model(&models.MonitorURL{}))

	// Сначала COUNT для пагинации; при ошибке показываем пустой список, не 500.
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		applog.AddError("failed to count monitors", err.Error())
		total = 0
	}
	page = models.ClampPage(page, total, perPage)

	var monitors []models.MonitorURL
	// GORM переиспользует один и тот же *Statement: после Count на baseQuery в нём
	// остаётся SELECT count(*). Пересобираем цепочку filter→sort→Find с нуля.
	query := sort.Apply(filter.Apply(h.DB.Model(&models.MonitorURL{})))
	if err := query.
		Offset(models.PageOffset(page, perPage)).
		Limit(perPage).
		Find(&monitors).Error; err != nil {
		applog.AddError("failed to load monitors", err.Error())
		monitors = nil
	}

	// ID и CreatedAt нужны для batch-загрузки истории uptime и статистики.
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

	// Собираем view model: монитор + полоски истории + агрегированный uptime.
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
		"ReturnTo":   sort.PageURL(page),
	}, PageOptions{Title: "Monitors", ActiveNav: "monitors"})
}

// NewMonitorPage отображает форму создания URL.
func (h *Handler) NewMonitorPage(c *gin.Context) {
	// Шаблону нужно знать, какие каналы уведомлений настроены в Settings.
	_, notifyData, err := h.monitorNotificationContext()
	if err != nil {
		applog.AddError("failed to load notification settings", err.Error())
		notifyData = gin.H{
			"TelegramConfigured": false,
			"SMTPConfigured":     false,
		}
	}

	data := gin.H{
		"Input": forms.MonitorURLInput{}, // пустая форма
	}
	for key, value := range notifyData {
		data[key] = value
	}

	h.renderPage(c, http.StatusOK, "admin/monitors/new.html", data, PageOptions{
		Title:     "Add Monitor URL",
		ActiveNav: "monitors",
	})
}

// CreateMonitor обрабатывает создание URL.
func (h *Handler) CreateMonitor(c *gin.Context) {
	var input forms.MonitorURLInput
	if err := c.ShouldBind(&input); err != nil {
		// Ошибка bind — показываем форму с сообщением, без redirect.
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
		// Валидация URL, интервала и т.д. — форма с текстом ошибки.
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

	if input.VerifyBeforeCreate {
		// Опциональная проверка доступности URL до INSERT — пользователь включил галочку.
		failures := monitorsvc.VerifyReachable(c.Request.Context(), []string{input.URL})
		if len(failures) > 0 {
			_, notifyData, _ := h.monitorNotificationContext()
			h.renderPage(c, http.StatusUnprocessableEntity, "admin/monitors/new.html", gin.H{
				"Error":              monitorsvc.UnavailableMessage(failures),
				"Input":              input,
				"NotifyTelegram":     input.NotifyTelegram,
				"NotifySMTP":         input.NotifySMTP,
				"TelegramConfigured": notifyData["TelegramConfigured"],
				"SMTPConfigured":     notifyData["SMTPConfigured"],
			}, PageOptions{Title: "Add Monitor URL", ActiveNav: "monitors"})
			return
		}
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
		status := http.StatusInternalServerError
		errMsg := "Failed to create monitor URL"
		if models.IsUniqueViolation(err) {
			// URL уже есть в monitor_urls — конфликт, не 500.
			status = http.StatusConflict
			errMsg = monitorsvc.ErrMonitorURLExists
		}
		_, notifyData, _ := h.monitorNotificationContext()
		h.renderPage(c, status, "admin/monitors/new.html", gin.H{
			"Error":              errMsg,
			"Input":              input,
			"NotifyTelegram":     input.NotifyTelegram,
			"NotifySMTP":         input.NotifySMTP,
			"TelegramConfigured": notifyData["TelegramConfigured"],
			"SMTPConfigured":     notifyData["SMTPConfigured"],
		}, PageOptions{Title: "Add Monitor URL", ActiveNav: "monitors"})
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Created monitor %q (%s)", monitor.Name, monitor.URL))
	redirectWithFlash(c, "/admin/monitors", flashSavedMessage)
}

// BulkNewMonitorPage отображает форму для одновременного создания нескольких мониторимых URL.
// c — контекст Gin-запроса для аутентифицированной сессии админки.
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

// bulkMonitorFormData формирует данные шаблона для формы массового создания, включая контекст уведомлений.
// input — отправленная (или пустая) bulk-форма.
// errMsg — пользовательская строка ошибки; пусто, если ошибки нет.
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

// BulkCreateMonitors создаёт несколько мониторов из списка URL, разделённых запятыми или переводами строк.
// Флаги уведомлений и необязательный интервал проверки применяются к каждому созданному монитору.
// Name каждого монитора устанавливается равным его URL.
// c — контекст Gin-запроса с телом POST-формы.
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
	urls := input.ParsedURLs() // разбор textarea: запятые и переводы строк

	svc := monitorsvc.NewService(h.DB)
	if conflicts, err := svc.ExistingURLs(urls); err != nil {
		applog.AddError("failed to check existing monitor URLs", err.Error())
		h.renderPage(c, http.StatusInternalServerError, "admin/monitors/bulk_new.html",
			h.bulkMonitorFormData(input, "Failed to create monitor URLs"),
			PageOptions{Title: "Add multiple Monitor URLs", ActiveNav: "monitors"})
		return
	} else if len(conflicts) > 0 {
		// SkipExisting: пропустить уже существующие URL и создать только новые.
		if !input.SkipExisting {
			h.renderPage(c, http.StatusConflict, "admin/monitors/bulk_new.html",
				h.bulkMonitorFormData(input, monitorsvc.URLExistsMessage(conflicts...)),
				PageOptions{Title: "Add multiple Monitor URLs", ActiveNav: "monitors"})
			return
		}
		urls = monitorsvc.ExcludeURLs(urls, conflicts)
		if len(urls) == 0 {
			redirectWithFlash(c, "/admin/monitors", flashSavedMessage)
			return
		}
	}

	// VerifyBeforeCreate: «всё или ничего» — один недоступный URL отклоняет весь batch.
	if input.VerifyBeforeCreate {
		failures := monitorsvc.VerifyReachable(c.Request.Context(), urls)
		if len(failures) > 0 {
			h.renderPage(c, http.StatusUnprocessableEntity, "admin/monitors/bulk_new.html",
				h.bulkMonitorFormData(input, monitorsvc.UnavailableMessage(failures)),
				PageOptions{Title: "Add multiple Monitor URLs", ActiveNav: "monitors"})
			return
		}
	}

	failedURL := ""
	// Вся пачка в одной транзакции: при ошибке на одном URL откатываются все INSERT.
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, rawURL := range urls {
			// Name=URL: у bulk-создания нет отдельного поля имени для каждой строки.
			monitor := models.MonitorURL{
				Name:                 rawURL,
				URL:                  rawURL,
				CheckIntervalSeconds: checkInterval,
				NotifyTelegram:       input.NotifyTelegram,
				NotifySMTP:           input.NotifySMTP,
			}
			if err := tx.Create(&monitor).Error; err != nil {
				failedURL = rawURL
				return err
			}
		}
		return nil
	})
	if err != nil {
		status := http.StatusInternalServerError
		errMsg := "Failed to create monitor URLs"
		if models.IsUniqueViolation(err) {
			status = http.StatusConflict
			errMsg = monitorsvc.URLExistsMessage(failedURL)
		}
		h.renderPage(c, status, "admin/monitors/bulk_new.html",
			h.bulkMonitorFormData(input, errMsg),
			PageOptions{Title: "Add multiple Monitor URLs", ActiveNav: "monitors"})
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Created %d monitors in bulk", len(urls)))
	redirectWithFlash(c, "/admin/monitors", flashSavedMessage)
}

// MonitorShowPage отрисовывает детали монитора и историю heartbeat.
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

	// На странице монитора две независимые пагинации: инциденты и heartbeats.
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

	// Uptime-полоски и процент для одного монитора (не batch, как в списке).
	historyBars, err := models.BuildUptimeHistoryBars(h.DB, uint(id), monitor.CreatedAt, now)
	if err != nil {
		applog.AddError("failed to load monitor uptime history", err.Error())
		historyBars = nil
	}

	uptime, err := models.LoadMonitorUptime(h.DB, uint(id), monitor.CreatedAt, now)
	if err != nil {
		applog.AddError("failed to load monitor uptime stats", err.Error())
	}

	// URL пагинации сохраняет оба query-параметра при переключении страниц.
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

	_, notifyData, notifyErr := h.monitorNotificationContext()
	if notifyErr != nil {
		applog.AddError("failed to load notification settings", notifyErr.Error())
		notifyData = gin.H{
			"TelegramConfigured": false,
			"SMTPConfigured":     false,
		}
	}

	data := gin.H{
		"Monitor":  monitor,
		"ReturnTo": monitorsListReturnURL(c), // скрытое поле формы для PRG после update/delete
	}
	for key, value := range notifyData {
		data[key] = value
	}
	data["NotifyTelegram"] = monitor.NotifyTelegram
	data["NotifySMTP"] = monitor.NotifySMTP

	h.renderPage(c, http.StatusOK, "admin/monitors/edit.html", data, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
}

// UpdateMonitor обрабатывает обновление URL.
func (h *Handler) UpdateMonitor(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	// Куда вернуться после сохранения — whitelist return_to из формы/query.
	returnTo := monitorsListReturnURL(c)

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
			"ReturnTo":           returnTo,
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
		// Подставляем введённые значения в monitor для повторного показа формы.
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
			"ReturnTo":           returnTo,
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
		status := http.StatusInternalServerError
		errMsg := "Failed to update monitor URL"
		if models.IsUniqueViolation(err) {
			status = http.StatusConflict
			errMsg = monitorsvc.ErrMonitorURLExists
		}
		_, notifyData, _ := h.monitorNotificationContext()
		h.renderPage(c, status, "admin/monitors/edit.html", gin.H{
			"Error":              errMsg,
			"Monitor":            monitor,
			"ReturnTo":           returnTo,
			"NotifyTelegram":     input.NotifyTelegram,
			"NotifySMTP":         input.NotifySMTP,
			"TelegramConfigured": notifyData["TelegramConfigured"],
			"SMTPConfigured":     notifyData["SMTPConfigured"],
		}, PageOptions{Title: "Edit Monitor URL", ActiveNav: "monitors"})
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Updated monitor %q (%s)", monitor.Name, monitor.URL))
	redirectWithFlash(c, returnTo, flashSavedMessage)
}

// DeleteMonitor безвозвратно удаляет мониторимый URL и каскадно связанные строки.
// c — контекст Gin-запроса с path-параметром id монитора.
func (h *Handler) DeleteMonitor(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	returnTo := monitorsListReturnURL(c)

	var monitor models.MonitorURL
	if err := h.DB.First(&monitor, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	// CASCADE в БД удалит связанные checks и incidents.
	if err := h.DB.Delete(&models.MonitorURL{}, id).Error; err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	applog.AddEvent("monitor", fmt.Sprintf("Deleted monitor %q (%s)", models.MonitorDisplayName(monitor), monitor.URL))
	redirectWithFlash(c, returnTo, flashDeletedMessage)
}
