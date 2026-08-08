package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/worker"
)

// infoDiagnosticsRecentErrorsLimit ограничивает число in-memory ошибок в JSON blob.
const infoDiagnosticsRecentErrorsLimit = 50

// infoDiagnostics — Cursor-friendly ops snapshot, встроенный на страницу info админки.
type infoDiagnostics struct {
	// GeneratedAt — когда этот snapshot был собран (RFC3339 в JSON).
	GeneratedAt time.Time `json:"generated_at"`
	// Environment — не секретное значение GO_UPTIME_ENVIRONMENT.
	Environment string `json:"environment"`
	// Worker содержит live check-wave и queue depth metrics.
	Worker infoDiagnosticsWorker `json:"worker"`
	// Monitors суммирует fleet backlog и status composition.
	Monitors infoDiagnosticsMonitors `json:"monitors"`
	// HeartbeatsPastHour агрегирует успешные и неуспешные checks за последний час.
	HeartbeatsPastHour infoDiagnosticsHeartbeats `json:"heartbeats_past_hour"`
	// Incidents считает open и total incident rows.
	Incidents infoDiagnosticsIncidents `json:"incidents"`
	// Applog суммирует in-memory log buffers и недавние ошибки.
	Applog infoDiagnosticsApplog `json:"applog"`
	// Tables перечисляет таблицы приложения с row counts и on-disk sizes.
	Tables []infoDiagnosticsTable `json:"tables"`
}

// infoDiagnosticsWorker — секция worker в diagnostics JSON.
type infoDiagnosticsWorker struct {
	// Running — true, когда цикл monitor worker запущен и не остановлен.
	Running bool `json:"running"`
	// Paused — true, когда новые check waves подавлены (например во время e2e).
	Paused bool `json:"paused"`
	// DueThisWave — сколько claimed мониторов ещё не завершили probing.
	DueThisWave int `json:"due_this_wave"`
	// InFlight — сколько HTTP checks выполняется прямо сейчас.
	InFlight int `json:"in_flight"`
	// WaitingForSlot — сколько claimed мониторов всё ещё ждут concurrency slot.
	WaitingForSlot int `json:"waiting_for_slot"`
	// MaxConcurrency — настроенный лимит concurrent HTTP checks.
	MaxConcurrency int `json:"max_concurrency"`
	// ResultQueued — завершённые checks, ожидающие persist.
	ResultQueued int `json:"result_queued"`
	// ResultCapacity — размер буфера persist channel.
	ResultCapacity int `json:"result_capacity"`
	// NotifyQueued — сколько status-change alerts находится в notify channel.
	NotifyQueued int `json:"notify_queued"`
	// NotifyCapacity — размер буфера notify channel.
	NotifyCapacity int `json:"notify_capacity"`
}

// infoDiagnosticsMonitors — секция monitors в diagnostics JSON.
type infoDiagnosticsMonitors struct {
	// Total — сколько мониторов существует.
	Total int `json:"total"`
	// DueWaiting — сколько мониторов уже due для check.
	DueWaiting int `json:"due_waiting"`
	// NeverChecked — сколько мониторов никогда не probed.
	NeverChecked int `json:"never_checked"`
	// Fleet разбивает мониторы по текущему статусу.
	Fleet infoDiagnosticsFleet `json:"fleet"`
	// MostOverdue — худший просроченный монитор или null, если таких нет.
	MostOverdue *infoDiagnosticsOverdue `json:"most_overdue"`
}

// infoDiagnosticsFleet считает мониторы по статусу up/down/unknown.
type infoDiagnosticsFleet struct {
	// Up — мониторы, последний раз seen healthy.
	Up int `json:"up"`
	// Down — мониторы, последний раз seen unhealthy.
	Down int `json:"down"`
	// Unknown — мониторы, которые никогда не checked.
	Unknown int `json:"unknown"`
}

// infoDiagnosticsOverdue описывает единственный самый просроченный монитор для diagnostics.
type infoDiagnosticsOverdue struct {
	// ID — primary key monitor_urls.
	ID uint `json:"id"`
	// Name — отображаемое имя в admin UI.
	Name string `json:"name"`
	// URL — мониторимый URL.
	URL string `json:"url"`
	// LastChecked — отформатированный timestamp последней проверки.
	LastChecked string `json:"last_checked"`
	// OverdueBy — краткая человекочитаемая overdue duration.
	OverdueBy string `json:"overdue_by"`
}

// infoDiagnosticsHeartbeats суммирует totals heartbeat за прошлый час.
type infoDiagnosticsHeartbeats struct {
	// Total — успешные плюс неуспешные heartbeat в окне.
	Total int `json:"total"`
	// Success — успешные heartbeat в окне.
	Success int `json:"success"`
	// Failed — неуспешные heartbeat в окне.
	Failed int `json:"failed"`
}

// infoDiagnosticsIncidents суммирует counts incident rows.
type infoDiagnosticsIncidents struct {
	// Total — все incident rows.
	Total int64 `json:"total"`
	// Open — incidents, которые ещё не resolved.
	Open int64 `json:"open"`
}

// infoDiagnosticsApplog суммирует in-memory applog buffers для Cursor analysis.
type infoDiagnosticsApplog struct {
	// ErrorsStored — сколько error records сейчас в буфере.
	ErrorsStored int64 `json:"errors_stored"`
	// EventsStored — сколько application events сейчас в буфере.
	EventsStored int64 `json:"events_stored"`
	// RequestsStored — сколько monitor HTTP request records в буфере.
	RequestsStored int64 `json:"requests_stored"`
	// RecentErrors — новейшие error entries, ограниченные для размера payload.
	RecentErrors []applog.Entry `json:"recent_errors"`
}

// infoDiagnosticsTable — одна PostgreSQL-таблица приложения с size metrics.
type infoDiagnosticsTable struct {
	// Name — имя PostgreSQL-таблицы.
	Name string `json:"name"`
	// RowCount — число строк, сейчас хранящихся в таблице.
	RowCount int64 `json:"row_count"`
	// TotalBytes — pg_total_relation_size для таблицы (включая indexes).
	TotalBytes int64 `json:"total_bytes"`
}

// buildInfoDiagnostics собирает Cursor-friendly diagnostics snapshot.
// now — timestamp snapshot, записываемый в generated_at.
// environment — не секретное имя environment приложения.
// w — live monitor worker (может быть nil в тестах).
// workerStats — моментальный Stats, уже загруженный для страницы info.
// backlog — сводка due/waiting, уже вычисленная для страницы info.
// fleet — up/down/unknown composition, уже вычисленная для страницы info.
// heartbeat — chart за прошлый час, уже вычисленный для страницы info.
// incidentTotal — общее число incident rows.
// incidentOpen — сколько incidents всё ещё unresolved.
// tableCounts — таблицы приложения с row counts и disk sizes.
func buildInfoDiagnostics(
	now time.Time,
	environment string,
	w *worker.MonitorWorker,
	workerStats worker.Stats,
	backlog monitorBacklog,
	fleet compositionChart,
	heartbeat heartbeatHourChart,
	incidentTotal, incidentOpen int64,
	tableCounts []tableRowCount,
) infoDiagnostics {
	running := false
	paused := false
	if w != nil {
		running = w.Running()
		paused = w.Paused()
	}

	// Ограничиваем размер JSON — только последние N ошибок из in-memory буфера.
	recentErrors := applog.RecentErrors()
	if len(recentErrors) > infoDiagnosticsRecentErrorsLimit {
		recentErrors = recentErrors[:infoDiagnosticsRecentErrorsLimit]
	}
	if recentErrors == nil {
		recentErrors = []applog.Entry{} // JSON null → пустой массив
	}

	tables := make([]infoDiagnosticsTable, 0, len(tableCounts))
	for _, table := range tableCounts {
		tables = append(tables, infoDiagnosticsTable{
			Name:       table.Name,
			RowCount:   table.Count,
			TotalBytes: table.TotalBytes,
		})
	}

	return infoDiagnostics{
		GeneratedAt: now.UTC(),
		Environment: environment,
		Worker: infoDiagnosticsWorker{
			Running:        running,
			Paused:         paused,
			DueThisWave:    workerStats.DueThisWave,
			InFlight:       workerStats.InFlight,
			WaitingForSlot: workerStats.WaitingForSlot,
			MaxConcurrency: workerStats.MaxConcurrency,
			ResultQueued:   workerStats.ResultQueued,
			ResultCapacity: workerStats.ResultCapacity,
			NotifyQueued:   workerStats.NotifyQueued,
			NotifyCapacity: workerStats.NotifyCapacity,
		},
		Monitors: infoDiagnosticsMonitors{
			Total:        backlog.Total,
			DueWaiting:   backlog.DueWaiting,
			NeverChecked: backlog.NeverChecked,
			Fleet:        fleetCountsFromChart(fleet),
			MostOverdue:  overdueFromView(backlog.MostOverdue),
		},
		HeartbeatsPastHour: infoDiagnosticsHeartbeats{
			Total:   heartbeat.Total,
			Success: heartbeat.TotalSuccess,
			Failed:  heartbeat.TotalFailed,
		},
		Incidents: infoDiagnosticsIncidents{
			Total: incidentTotal,
			Open:  incidentOpen,
		},
		Applog: infoDiagnosticsApplog{
			ErrorsStored:   applog.CountErrors(),
			EventsStored:   applog.CountEvents(),
			RequestsStored: applog.CountMonitorRequests(),
			RecentErrors:   recentErrors,
		},
		Tables: tables,
	}
}

// fleetCountsFromChart преобразует stacked fleet segments в объект diagnostics fleet.
// chart — composition chart, уже построенный для UI страницы info.
func fleetCountsFromChart(chart compositionChart) infoDiagnosticsFleet {
	out := infoDiagnosticsFleet{}
	for _, segment := range chart.Segments {
		// Modifier совпадает с BEM-классами UI — переиспользуем для JSON.
		switch segment.Modifier {
		case "up":
			out.Up = segment.Count
		case "down":
			out.Down = segment.Count
		case "unknown":
			out.Unknown = segment.Count
		}
	}
	return out
}

// overdueFromView преобразует template overdue view в JSON pointer или nil.
// view — поля представления most-overdue из computeMonitorBacklog.
func overdueFromView(view overdueMonitorView) *infoDiagnosticsOverdue {
	if !view.HasOverdue {
		return nil // в JSON поле most_overdue будет null
	}
	return &infoDiagnosticsOverdue{
		ID:          view.ID,
		Name:        view.Name,
		URL:         view.URL,
		LastChecked: view.LastChecked,
		OverdueBy:   view.OverdueBy,
	}
}

// marshalInfoDiagnosticsJSON pretty-prints diagnostics для блока <pre> на странице info.
// diagnostics — snapshot, уже собранный buildInfoDiagnostics.
func marshalInfoDiagnosticsJSON(diagnostics infoDiagnostics) (string, error) {
	// MarshalIndent — читаемый блок <pre> на странице info для копирования.
	raw, err := json.MarshalIndent(diagnostics, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal info diagnostics: %w", err)
	}
	return string(raw), nil
}
