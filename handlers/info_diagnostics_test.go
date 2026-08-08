package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/worker"
)

func TestBuildInfoDiagnostics(t *testing.T) {
	// Arrange: заполняем in-memory applog тестовыми записями разных типов.
	applog.ClearAll()
	applog.AddError("first error", `{"code":1}`)
	applog.AddError("second error", "")
	applog.AddEvent("monitor", "check started")
	applog.AddMonitorRequest("Site", "https://example.com", 200, 12, true, "")

	// Arrange: фиксированное время и синтетические агрегаты для всех секций diagnostics.
	now := time.Date(2026, 8, 8, 6, 0, 0, 0, time.UTC)
	backlog := monitorBacklog{
		Total:        3,
		DueWaiting:   2,
		NeverChecked: 1,
		MostOverdue: overdueMonitorView{
			ID:          9,
			Name:        "Overdue",
			URL:         "https://overdue.example",
			LastChecked: "2026-08-08 05:00:00",
			OverdueBy:   "1h 0m",
			HasOverdue:  true,
		},
	}
	fleet := compositionChart{
		Total: 3,
		Segments: []compositionSegment{
			{Label: "Up", Count: 1, Modifier: "up"},
			{Label: "Down", Count: 1, Modifier: "down"},
			{Label: "Unknown", Count: 1, Modifier: "unknown"},
		},
	}
	heartbeat := heartbeatHourChart{Total: 5, TotalSuccess: 3, TotalFailed: 2}
	workerStats := worker.Stats{
		DueThisWave:    4,
		InFlight:       2,
		WaitingForSlot: 2,
		MaxConcurrency: 150,
		ResultQueued:   1,
		ResultCapacity: 2048,
		NotifyQueued:   0,
		NotifyCapacity: 256,
	}
	tables := []tableRowCount{
		{Name: "monitor_urls", Count: 3, TotalBytes: 8192},
		{Name: "users", Count: 1, TotalBytes: 4096},
	}

	// Act: собираем полный snapshot diagnostics (worker=nil — standalone server без worker process).
	got := buildInfoDiagnostics(
		now,
		"development",
		nil,
		workerStats,
		backlog,
		fleet,
		heartbeat,
		10,
		2,
		tables,
	)

	// Assert: метаданные snapshot.
	if !got.GeneratedAt.Equal(now) {
		t.Fatalf("GeneratedAt = %v, want %v", got.GeneratedAt, now)
	}
	if got.Environment != "development" {
		t.Fatalf("Environment = %q, want development", got.Environment)
	}
	// Assert: nil worker — не running и не paused, но stats из workerStats всё равно попадают в JSON.
	if got.Worker.Running || got.Worker.Paused {
		t.Fatalf("nil worker should report running/paused false: %+v", got.Worker)
	}
	if got.Worker.DueThisWave != 4 || got.Worker.MaxConcurrency != 150 {
		t.Fatalf("unexpected worker stats: %+v", got.Worker)
	}
	// Assert: секция monitors — backlog counters.
	if got.Monitors.Total != 3 || got.Monitors.DueWaiting != 2 || got.Monitors.NeverChecked != 1 {
		t.Fatalf("unexpected monitors: %+v", got.Monitors)
	}
	// Assert: fleet breakdown по up/down/unknown.
	if got.Monitors.Fleet.Up != 1 || got.Monitors.Fleet.Down != 1 || got.Monitors.Fleet.Unknown != 1 {
		t.Fatalf("unexpected fleet: %+v", got.Monitors.Fleet)
	}
	// Assert: most_overdue присутствует, когда HasOverdue=true во входе.
	if got.Monitors.MostOverdue == nil || got.Monitors.MostOverdue.ID != 9 {
		t.Fatalf("unexpected most_overdue: %+v", got.Monitors.MostOverdue)
	}
	// Assert: heartbeats за последний час.
	if got.HeartbeatsPastHour.Total != 5 || got.HeartbeatsPastHour.Success != 3 || got.HeartbeatsPastHour.Failed != 2 {
		t.Fatalf("unexpected heartbeats: %+v", got.HeartbeatsPastHour)
	}
	// Assert: incidents — total и open переданы аргументами (10 total, 2 open).
	if got.Incidents.Total != 10 || got.Incidents.Open != 2 {
		t.Fatalf("unexpected incidents: %+v", got.Incidents)
	}
	// Assert: applog counters — 2 error, 1 event, 1 monitor request.
	if got.Applog.ErrorsStored != 2 || got.Applog.EventsStored != 1 || got.Applog.RequestsStored != 1 {
		t.Fatalf("unexpected applog counts: %+v", got.Applog)
	}
	// Assert: recent_errors — обе записи, newest-first (second error первым).
	if len(got.Applog.RecentErrors) != 2 {
		t.Fatalf("recent_errors len = %d, want 2", len(got.Applog.RecentErrors))
	}
	if got.Applog.RecentErrors[0].Message != "second error" {
		t.Fatalf("recent_errors[0] = %q, want second error", got.Applog.RecentErrors[0].Message)
	}
	// Assert: tables — row counts и bytes из входного слайса.
	if len(got.Tables) != 2 || got.Tables[0].TotalBytes != 8192 {
		t.Fatalf("unexpected tables: %+v", got.Tables)
	}
}

func TestBuildInfoDiagnosticsLimitsRecentErrors(t *testing.T) {
	// Arrange: больше ошибок, чем infoDiagnosticsRecentErrorsLimit — проверяем truncation.
	applog.ClearAll()
	for i := 0; i < infoDiagnosticsRecentErrorsLimit+10; i++ {
		applog.AddError("error", "")
	}

	// Act: минимальный вызов buildInfoDiagnostics — только applog и tables интересны.
	got := buildInfoDiagnostics(
		time.Now(),
		"test",
		nil,
		worker.Stats{},
		monitorBacklog{},
		compositionChart{},
		heartbeatHourChart{},
		0,
		0,
		nil,
	)
	// Assert: recent_errors обрезаны до лимита (не все 110+).
	if len(got.Applog.RecentErrors) != infoDiagnosticsRecentErrorsLimit {
		t.Fatalf("recent_errors len = %d, want %d", len(got.Applog.RecentErrors), infoDiagnosticsRecentErrorsLimit)
	}
	// Assert: nil tables → пустой slice, не null в JSON.
	if got.Tables == nil {
		t.Fatal("tables should be empty slice, not nil")
	}
	if len(got.Tables) != 0 {
		t.Fatalf("tables len = %d, want 0", len(got.Tables))
	}
}

func TestBuildInfoDiagnosticsOmitsMostOverdueWhenEmpty(t *testing.T) {
	// Arrange: пустой backlog без просроченного монитора.
	applog.ClearAll()
	// Act.
	got := buildInfoDiagnostics(
		time.Now(),
		"test",
		nil,
		worker.Stats{},
		monitorBacklog{},
		compositionChart{},
		heartbeatHourChart{},
		0,
		0,
		[]tableRowCount{},
	)
	// Assert: most_overdue должен быть nil/omit, а не пустой объект.
	if got.Monitors.MostOverdue != nil {
		t.Fatalf("most_overdue = %+v, want nil", got.Monitors.MostOverdue)
	}
}

func TestMarshalInfoDiagnosticsJSON(t *testing.T) {
	// Arrange: одна ошибка с JSON fields — проверяем embed, а не escaped string.
	applog.ClearAll()
	applog.AddError("boom", `{"monitor_id":7}`)
	diagnostics := buildInfoDiagnostics(
		time.Date(2026, 8, 8, 6, 0, 0, 0, time.UTC),
		"development",
		nil,
		worker.Stats{MaxConcurrency: 150, ResultCapacity: 2048, NotifyCapacity: 256},
		monitorBacklog{Total: 1},
		compositionChart{},
		heartbeatHourChart{},
		0,
		0,
		[]tableRowCount{{Name: "users", Count: 1, TotalBytes: 4096}},
	)

	// Act: marshal в JSON string для API / copy-to-clipboard.
	raw, err := marshalInfoDiagnosticsJSON(diagnostics)
	if err != nil {
		t.Fatalf("marshalInfoDiagnosticsJSON: %v", err)
	}

	// Assert: top-level keys присутствуют (контракт фронтенда).
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v\n%s", err, raw)
	}
	for _, key := range []string{"generated_at", "environment", "worker", "monitors", "heartbeats_past_hour", "incidents", "applog", "tables"} {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("missing key %q in %s", key, raw)
		}
	}

	// Assert: applog.recent_errors[0].fields — вложенный object, не строка.
	applogSection, ok := parsed["applog"].(map[string]any)
	if !ok {
		t.Fatalf("applog is %T, want object", parsed["applog"])
	}
	recent, ok := applogSection["recent_errors"].([]any)
	if !ok || len(recent) != 1 {
		t.Fatalf("recent_errors = %#v, want one entry", applogSection["recent_errors"])
	}
	entry, ok := recent[0].(map[string]any)
	if !ok {
		t.Fatalf("recent_errors[0] is %T, want object", recent[0])
	}
	fields, ok := entry["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields is %T (%v), want embedded JSON object (not escaped string)", entry["fields"], entry["fields"])
	}
	// Assert: json.Unmarshal чисел в map[string]any даёт float64(7).
	if fields["monitor_id"] != float64(7) {
		t.Fatalf("fields.monitor_id = %#v, want 7", fields["monitor_id"])
	}
}
