package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"go-uptime/models"
	"go-uptime/worker"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// overdueMonitorView содержит поля представления для самого просроченного монитора.
type overdueMonitorView struct {
	ID          uint
	Name        string
	URL         string
	LastChecked string
	OverdueBy   string
	HasOverdue  bool
}

// monitorBacklog суммирует due/waiting мониторы для страницы info админки.
type monitorBacklog struct {
	Total        int
	DueWaiting   int
	NeverChecked int
	MostOverdue  overdueMonitorView
}

// utilizationGauge — один Bootstrap progress bar для live worker capacity.
type utilizationGauge struct {
	// Label — краткое имя метрики над полосой.
	Label string
	// Value — текущее абсолютное значение.
	Value int
	// Max — знаменатель для gauge; ноль означает пустую неактивную полосу.
	Max int
	// Percent — Value/Max, ограниченное 0–100 для ширины CSS.
	Percent int
	// Detail — текст "value / max" рядом с полосой.
	Detail string
}

// compositionSegment — один взаимоисключающий сегмент stacked composition chart.
type compositionSegment struct {
	// Label — человекочитаемое имя сегмента (например "Up").
	Label string
	// Count — сколько мониторов попадает в этот сегмент.
	Count int
	// Percent — Count/Total, ограниченное 0–100 для ширины CSS.
	Percent int
	// Modifier — суффикс BEM-модификатора, например "up" или "due".
	Modifier string
}

// compositionChart группирует stacked segments для разбивки fleet или backlog.
type compositionChart struct {
	// Total — сумма count сегментов (обычно все мониторы).
	Total int
	// Segments — упорядоченные сегменты для stacked bar и легенды.
	Segments []compositionSegment
}

// heartbeatMinuteBar — одна минутная колонка в heartbeat chart за прошлый час.
type heartbeatMinuteBar struct {
	// Label — краткое время на циферблате в tooltips (HH:MM в локальном времени).
	Label string
	// Title — полный accessible tooltip для колонки.
	Title string
	// Success — сколько heartbeat успешно завершилось в эту минуту.
	Success int
	// Failed — сколько heartbeat завершилось с ошибкой в эту минуту.
	Failed int
	// Total — Success + Failed.
	Total int
	// HeightPercent — высота колонки относительно самой загруженной минуты (0–100).
	HeightPercent int
	// SuccessPercent — доля успеха в высоте колонки (0–100 от HeightPercent stack).
	SuccessPercent int
	// FailedPercent — доля ошибок в высоте колонки.
	FailedPercent int
}

// heartbeatHourChart — поминутная разбивка success/failure за прошлый час.
type heartbeatHourChart struct {
	// Bars — ровно models.HeartbeatHourMinutes колонок, сначала самые старые.
	Bars []heartbeatMinuteBar
	// MaxPerMinute — наибольший Total среди Bars (масштаб chart).
	MaxPerMinute int
	// TotalSuccess — успешные heartbeat за весь час.
	TotalSuccess int
	// TotalFailed — неуспешные heartbeat за весь час.
	TotalFailed int
	// Total — Success + Failed.
	Total int
	// StartLabel — метка самой старой минуты на оси X.
	StartLabel string
	// EndLabel — метка самой новой минуты на оси X.
	EndLabel string
}

// heartbeatHourChartPayload — JSON-форма, потребляемая Chart.js на странице info.
type heartbeatHourChartPayload struct {
	// Labels — минутные метки времени (HH:MM), сначала самые старые.
	Labels []string `json:"labels"`
	// Success — количество успешных heartbeat, выровненное с Labels.
	Success []int `json:"success"`
	// Failed — количество неуспешных heartbeat, выровненное с Labels.
	Failed []int `json:"failed"`
}

// tableRowCount — одна PostgreSQL-таблица приложения с текущим row count и размером на диске.
type tableRowCount struct {
	// Name — имя PostgreSQL-таблицы (например "monitor_urls").
	Name string
	// Count — число строк, сейчас хранящихся в таблице.
	Count int64
	// TotalBytes — pg_total_relation_size для таблицы (включая indexes).
	TotalBytes int64
}

// applicationTableModels перечисляет каждую GORM-модель, чью PostgreSQL-таблицу использует приложение.
// Order — по алфавиту имени таблицы, чтобы страница info была стабильной при перезагрузках.
var applicationTableModels = []struct {
	name  string
	model any
}{
	{name: "app_settings", model: &models.AppSetting{}},
	{name: "incidents", model: &models.Incident{}},
	{name: "monitor_checks", model: &models.MonitorCheck{}},
	{name: "monitor_urls", model: &models.MonitorURL{}},
	{name: "stat_daily", model: &models.StatDaily{}},
	{name: "stat_hourly", model: &models.StatHourly{}},
	{name: "stat_minutely", model: &models.StatMinutely{}},
	{name: "users", model: &models.User{}},
}

// InfoPage показывает backlog мониторов, live worker metrics и размеры PostgreSQL-таблиц.
func (h *Handler) InfoPage(c *gin.Context) {
	// Базовый набор данных: все мониторы для backlog и fleet composition.
	var monitors []models.MonitorURL
	if err := h.DB.Find(&monitors).Error; err != nil {
		log.Error().Err(err).Msg("failed to load monitors for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load monitors.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}

	// Размеры таблиц PostgreSQL для ops-обзора.
	tableCounts, err := loadTableRowCounts(h.DB)
	if err != nil {
		log.Error().Err(err).Msg("failed to load table row counts for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load table row counts.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}

	now := time.Now()
	// Поминутная статистика heartbeat за последний час → HTML-график и Chart.js JSON.
	minuteCounts, err := models.CountHeartbeatsByMinute(h.DB, now)
	if err != nil {
		log.Error().Err(err).Msg("failed to load heartbeat minute counts for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load heartbeat chart.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}
	heartbeatChart := buildHeartbeatHourChart(minuteCounts, now)
	chartJSON, err := marshalHeartbeatHourChartJSON(heartbeatChart)
	if err != nil {
		log.Error().Err(err).Msg("failed to encode heartbeat chart for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load heartbeat chart.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}

	globalIntervalSeconds := models.GetCheckIntervalSeconds(h.DB)
	backlog := computeMonitorBacklog(monitors, globalIntervalSeconds, now)

	// Лимит параллельных HTTP-проверок из конфига или дефолт worker.
	checkConcurrency := worker.DefaultCheckConcurrency
	if h.Config != nil && h.Config.CheckConcurrency > 0 {
		checkConcurrency = h.Config.CheckConcurrency
	}

	workerStats := worker.Stats{}
	if h.Worker != nil {
		workerStats = h.Worker.Stats() // моментальный снимок очередей и in-flight
	}

	incidentTotal, err := models.CountIncidents(h.DB)
	if err != nil {
		log.Error().Err(err).Msg("failed to count incidents for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load incident counts.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}
	incidentOpen, err := models.CountOpenIncidents(h.DB)
	if err != nil {
		log.Error().Err(err).Msg("failed to count open incidents for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load incident counts.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}

	environment := ""
	if h.Config != nil {
		environment = h.Config.Environment
	}
	fleetComposition := buildFleetComposition(monitors)
	// JSON blob для копирования в Cursor / внешний анализ — собираем из уже загруженных данных.
	diagnostics := buildInfoDiagnostics(
		now,
		environment,
		h.Worker,
		workerStats,
		backlog,
		fleetComposition,
		heartbeatChart,
		incidentTotal,
		incidentOpen,
		tableCounts,
	)
	diagnosticsJSON, err := marshalInfoDiagnosticsJSON(diagnostics)
	if err != nil {
		log.Error().Err(err).Msg("failed to encode diagnostics JSON for info page")
		h.renderPage(c, http.StatusInternalServerError, "admin/info/index.html", gin.H{
			"Error": "Failed to load diagnostics JSON.",
		}, PageOptions{Title: "Info", ActiveNav: "info"})
		return
	}

	h.renderPage(c, http.StatusOK, "admin/info/index.html", gin.H{
		"TotalMonitors":          backlog.Total,
		"DueWaiting":             backlog.DueWaiting,
		"NeverChecked":           backlog.NeverChecked,
		"CheckConcurrency":       checkConcurrency,
		"MostOverdue":            backlog.MostOverdue,
		"WorkerStats":            workerStats,
		"UtilizationGauges":      buildUtilizationGauges(workerStats),
		"FleetComposition":       fleetComposition,
		"BacklogComposition":     buildBacklogComposition(backlog),
		"HeartbeatHourChart":     heartbeatChart,
		"HeartbeatHourChartJSON": chartJSON,
		"TableCounts":            tableCounts,
		"DiagnosticsJSON":        diagnosticsJSON,
	}, PageOptions{Title: "Info", ActiveNav: "info"})
}

// loadTableRowCounts возвращает текущий row count и размер на диске для каждой таблицы приложения.
// db — GORM handle для выполнения COUNT и pg_total_relation_size запросов к PostgreSQL.
func loadTableRowCounts(db *gorm.DB) ([]tableRowCount, error) {
	counts := make([]tableRowCount, 0, len(applicationTableModels))
	for _, entry := range applicationTableModels {
		var count int64
		if err := db.Model(entry.model).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("count rows in %s: %w", entry.name, err)
		}
		var totalBytes int64
		// Имена таблиц берутся из hardcoded whitelist applicationTableModels.
		if err := db.Raw(
			"SELECT pg_total_relation_size(?::regclass)",
			"public."+entry.name,
		).Scan(&totalBytes).Error; err != nil {
			return nil, fmt.Errorf("relation size for %s: %w", entry.name, err)
		}
		counts = append(counts, tableRowCount{Name: entry.name, Count: count, TotalBytes: totalBytes})
	}
	return counts, nil
}

// buildUtilizationGauges преобразует live worker stats в view models progress bar.
// stats — моментальный снимок worker.Stats от monitor worker.
func buildUtilizationGauges(stats worker.Stats) []utilizationGauge {
	// Знаменатель «Waiting for slot» — max из due wave и фактически ждущих слот.
	waitingMax := stats.DueThisWave
	if waitingMax < stats.WaitingForSlot {
		waitingMax = stats.WaitingForSlot
	}
	return []utilizationGauge{
		newUtilizationGauge("Check slots", stats.InFlight, stats.MaxConcurrency),
		newUtilizationGauge("Waiting for slot", stats.WaitingForSlot, waitingMax),
		newUtilizationGauge("Result queue", stats.ResultQueued, stats.ResultCapacity),
		newUtilizationGauge("Notify queue", stats.NotifyQueued, stats.NotifyCapacity),
	}
}

// newUtilizationGauge создаёт один gauge с безопасным процентом и detail label.
// label — краткий заголовок метрики в UI.
// value — текущее абсолютное значение.
// capacity — capacity или размер wave, используемый как знаменатель полосы.
func newUtilizationGauge(label string, value, capacity int) utilizationGauge {
	return utilizationGauge{
		Label:   label,
		Value:   value,
		Max:     capacity,
		Percent: percentOf(value, capacity),
		Detail:  fmt.Sprintf("%d / %d", value, capacity),
	}
}

// buildFleetComposition считает мониторы по текущему статусу для stacked chart.
// monitors — полный список мониторов, загруженный для страницы info.
func buildFleetComposition(monitors []models.MonitorURL) compositionChart {
	up, down, unknown := 0, 0, 0
	for i := range monitors {
		monitor := monitors[i]
		if monitor.LastCheckedAt == nil {
			unknown++ // ещё не было ни одной проверки
			continue
		}
		if monitor.IsUp != nil && *monitor.IsUp {
			up++
			continue
		}
		down++ // проверен, но не up (включая явный false и NULL is_up)
	}
	total := len(monitors)
	return compositionChart{
		Total: total,
		Segments: []compositionSegment{
			newCompositionSegment("Up", up, total, "up"),
			newCompositionSegment("Down", down, total, "down"),
			newCompositionSegment("Unknown", unknown, total, "unknown"),
		},
	}
}

// buildBacklogComposition преобразует backlog counts во взаимоисключающие сегменты.
// backlog — сводка due/waiting, уже вычисленная для страницы info.
// Never-checked мониторы всегда due, поэтому "Due" здесь означает ранее проверенные и просроченные.
func buildBacklogComposition(backlog monitorBacklog) compositionChart {
	// Due среди уже проверенных = все due минус never-checked (они всегда due).
	dueChecked := backlog.DueWaiting - backlog.NeverChecked
	if dueChecked < 0 {
		dueChecked = 0
	}
	onSchedule := backlog.Total - backlog.DueWaiting
	if onSchedule < 0 {
		onSchedule = 0
	}
	return compositionChart{
		Total: backlog.Total,
		Segments: []compositionSegment{
			newCompositionSegment("Due", dueChecked, backlog.Total, "due"),
			newCompositionSegment("Never checked", backlog.NeverChecked, backlog.Total, "never"),
			newCompositionSegment("On schedule", onSchedule, backlog.Total, "schedule"),
		},
	}
}

// newCompositionSegment создаёт один сегмент stacked bar с CSS-safe процентом.
// label — текст легенды для сегмента.
// count — сколько элементов принадлежит сегменту.
// total — знаменатель chart, используемый для Percent.
// modifier — суффикс BEM-модификатора, применяемый в шаблоне.
func newCompositionSegment(label string, count, total int, modifier string) compositionSegment {
	return compositionSegment{
		Label:    label,
		Count:    count,
		Percent:  percentOf(count, total),
		Modifier: modifier,
	}
}

// buildHeartbeatHourChart преобразует разреженные поминутные counts в фиксированный 60-колоночный chart.
// counts — непустые минутные buckets из models.CountHeartbeatsByMinute.
// now — тот же reference clock, использованный при загрузке этих counts.
func buildHeartbeatHourChart(counts []models.HeartbeatMinuteCount, now time.Time) heartbeatHourChart {
	// Окно: 60 минут, выровненных по UTC; метки на оси — в локальной TZ пользователя.
	windowEnd := now.UTC().Truncate(time.Minute)
	windowStart := windowEnd.Add(-time.Duration(models.HeartbeatHourMinutes-1) * time.Minute)
	loc := now.Location()

	// Разреженные buckets из SQL → map по unix-минуте для O(1) lookup.
	byMinute := make(map[int64]models.HeartbeatMinuteCount, len(counts))
	for _, count := range counts {
		byMinute[count.BucketAt.UTC().Truncate(time.Minute).Unix()] = count
	}

	bars := make([]heartbeatMinuteBar, 0, models.HeartbeatHourMinutes)
	maxPerMinute := 0
	totalSuccess := 0
	totalFailed := 0

	// Заполняем все 60 слотов — пустые минуты остаются с нулями.
	for i := 0; i < models.HeartbeatHourMinutes; i++ {
		bucketAt := windowStart.Add(time.Duration(i) * time.Minute)
		count := byMinute[bucketAt.Unix()]
		success := int(count.Success)
		failed := int(count.Failed)
		total := success + failed
		if total > maxPerMinute {
			maxPerMinute = total
		}
		totalSuccess += success
		totalFailed += failed

		label := bucketAt.In(loc).Format("15:04")
		bars = append(bars, heartbeatMinuteBar{
			Label:   label,
			Title:   fmt.Sprintf("%s — %d successful, %d failed (%d total)", label, success, failed, total),
			Success: success,
			Failed:  failed,
			Total:   total,
		})
	}

	// Второй проход: высота колонок относительно maxPerMinute и доли success/failed внутри столбца.
	for i := range bars {
		bar := &bars[i]
		bar.HeightPercent = percentOf(bar.Total, maxPerMinute)
		if bar.Total > 0 {
			bar.SuccessPercent = percentOf(bar.Success, bar.Total)
			bar.FailedPercent = 100 - bar.SuccessPercent
			// Крайние случаи: только успех или только ошибка — без «дробных» 99/1 из целочисленного деления.
			if bar.Failed == 0 {
				bar.FailedPercent = 0
				bar.SuccessPercent = 100
			} else if bar.Success == 0 {
				bar.SuccessPercent = 0
				bar.FailedPercent = 100
			}
		}
	}

	startLabel := ""
	endLabel := ""
	if len(bars) > 0 {
		startLabel = bars[0].Label
		endLabel = bars[len(bars)-1].Label
	}

	return heartbeatHourChart{
		Bars:         bars,
		MaxPerMinute: maxPerMinute,
		TotalSuccess: totalSuccess,
		TotalFailed:  totalFailed,
		Total:        totalSuccess + totalFailed,
		StartLabel:   startLabel,
		EndLabel:     endLabel,
	}
}

// marshalHeartbeatHourChartJSON кодирует series chart для Chart.js renderer.
// chart — view model за прошлый час, уже заполненная minute bars.
func marshalHeartbeatHourChartJSON(chart heartbeatHourChart) (template.JS, error) {
	payload := heartbeatHourChartPayload{
		Labels:  make([]string, len(chart.Bars)),
		Success: make([]int, len(chart.Bars)),
		Failed:  make([]int, len(chart.Bars)),
	}
	for i, bar := range chart.Bars {
		payload.Labels[i] = bar.Label
		payload.Success[i] = bar.Success
		payload.Failed[i] = bar.Failed
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal heartbeat hour chart: %w", err)
	}
	// JSON из encoding/json безопасно встраивать как JS literal для Chart.js.
	return template.JS(raw), nil //nolint:gosec // G203: trusted JSON, not user HTML
}

// percentOf возвращает value как целый процент от capacity, ограниченный 0–100.
// value — числитель (текущее использование или count сегмента).
// capacity — знаменатель (capacity или total); неположительный capacity даёт 0.
func percentOf(value, capacity int) int {
	if capacity <= 0 || value <= 0 {
		return 0
	}
	percent := (value * 100) / capacity
	if percent > 100 {
		return 100
	}
	return percent
}

// computeMonitorBacklog считает due мониторы и выбирает самый просроченный проверенный монитор.
// monitors — полный список мониторов из базы данных.
// globalIntervalSeconds — интервал проверки по умолчанию из настроек приложения.
// now — reference time для расчётов due и overdue.
// NeverChecked всегда due; MostOverdue — только среди уже проверенных (LastCheckedAt != nil).
func computeMonitorBacklog(monitors []models.MonitorURL, globalIntervalSeconds int, now time.Time) monitorBacklog {
	backlog := monitorBacklog{Total: len(monitors)}

	var mostOverdue *models.MonitorURL
	var mostOverdueBy time.Duration

	for i := range monitors {
		monitor := &monitors[i]
		intervalSeconds := models.MonitorCheckIntervalSeconds(*monitor, globalIntervalSeconds)
		interval := time.Duration(intervalSeconds) * time.Second

		if monitor.LastCheckedAt == nil {
			backlog.NeverChecked++
		}

		if !worker.IsMonitorDue(monitor.LastCheckedAt, interval, now) {
			continue // монитор ещё не просрочен — не попадает в DueWaiting
		}
		backlog.DueWaiting++

		// Для «самого просроченного» never-checked не сравниваем — у них нет базовой LastCheckedAt.
		if monitor.LastCheckedAt == nil {
			continue
		}
		overdueBy := now.Sub(*monitor.LastCheckedAt) - interval
		if mostOverdue == nil || overdueBy > mostOverdueBy {
			mostOverdue = monitor
			mostOverdueBy = overdueBy
		}
	}

	backlog.MostOverdue = buildOverdueMonitorView(mostOverdue, mostOverdueBy)
	return backlog
}

// buildOverdueMonitorView преобразует худший просроченный монитор в поля, удобные для шаблона.
// monitor — выбранный просроченный монитор или nil, если таких нет.
// overdueBy — насколько монитор просрочен относительно due time.
func buildOverdueMonitorView(monitor *models.MonitorURL, overdueBy time.Duration) overdueMonitorView {
	if monitor == nil {
		return overdueMonitorView{} // HasOverdue=false — шаблон скрывает блок
	}

	lastChecked := "—"
	if monitor.LastCheckedAt != nil {
		lastChecked = monitor.LastCheckedAt.Format("2006-01-02 15:04:05")
	}

	return overdueMonitorView{
		ID:          monitor.ID,
		Name:        models.MonitorDisplayName(*monitor),
		URL:         monitor.URL,
		LastChecked: lastChecked,
		OverdueBy:   formatDuration(overdueBy),
		HasOverdue:  true,
	}
}

// formatDuration форматирует неотрицательную duration в краткой человекочитаемой форме.
// d — duration для форматирования; отрицательные значения трактуются как ноль.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0 // отрицательная просрочка не показываем
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	// Часы и минуты без секунд для длинных интервалов.
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}
