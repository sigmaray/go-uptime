package handlers

import (
	"strings"
	"testing"
	"time"

	"go-uptime/models"
	"go-uptime/worker"
)

func TestFormatDuration(t *testing.T) {
	// Табличный тест human-readable формата длительности для UI info-страницы.
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "seconds", in: 45 * time.Second, want: "45s"},
		{name: "minutes", in: 2*time.Minute + 5*time.Second, want: "2m 5s"},
		{name: "hours", in: 3*time.Hour + 12*time.Minute, want: "3h 12m"},
		{name: "negative treated as zero", in: -time.Second, want: "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act + Assert: formatDuration округляет/обрезает до понятных единиц; отрицательные — как 0s.
			if got := formatDuration(tt.in); got != tt.want {
				t.Fatalf("formatDuration() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildOverdueMonitorView(t *testing.T) {
	// Assert: nil monitor — пустое представление без флага HasOverdue.
	if got := buildOverdueMonitorView(nil, 0); got.HasOverdue {
		t.Fatal("expected empty view for nil monitor")
	}

	// Arrange: monitor с LastCheckedAt и порог просрочки 90s (1m 30s).
	last := time.Date(2026, 7, 23, 6, 0, 0, 0, time.UTC)
	monitor := &models.MonitorURL{
		ID:            7,
		Name:          "Example",
		URL:           "https://example.com",
		LastCheckedAt: &last,
	}
	// Act: строим view просроченного монитора.
	got := buildOverdueMonitorView(monitor, 90*time.Second)
	// Assert: HasOverdue и поля идентичности монитора.
	if !got.HasOverdue {
		t.Fatal("expected HasOverdue")
	}
	if got.Name != "Example" || got.ID != 7 {
		t.Fatalf("unexpected view: %+v", got)
	}
	// Assert: OverdueBy — human-readable разница (90 секунд = 1m 30s).
	if got.OverdueBy != "1m 30s" {
		t.Fatalf("OverdueBy = %q, want 1m 30s", got.OverdueBy)
	}
	// Assert: LastChecked форматируется в UTC без timezone-сuffix.
	if got.LastChecked != "2026-07-23 06:00:00" {
		t.Fatalf("LastChecked = %q", got.LastChecked)
	}
}

func TestComputeMonitorBacklog(t *testing.T) {
	// Arrange: фиксированный «сейчас» и интервал проверки 60s для всех мониторов.
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	interval := 60
	recent := now.Add(-30 * time.Second)  // свежая проверка — не просрочен
	old := now.Add(-10 * time.Minute)     // просрочен, но не самый старый
	older := now.Add(-30 * time.Minute) // самый просроченный

	monitors := []models.MonitorURL{
		{ID: 1, Name: "Fresh", URL: "https://fresh.example", LastCheckedAt: &recent, CheckIntervalSeconds: &interval},
		{ID: 2, Name: "Never", URL: "https://never.example", CheckIntervalSeconds: &interval},
		{ID: 3, Name: "Overdue", URL: "https://overdue.example", LastCheckedAt: &old, CheckIntervalSeconds: &interval},
		{ID: 4, Name: "Oldest", URL: "https://oldest.example", LastCheckedAt: &older, CheckIntervalSeconds: &interval},
	}

	// Act: считаем backlog относительно workerDueSeconds=60.
	got := computeMonitorBacklog(monitors, 60, now)
	// Assert: всего 4 монитора в флоте.
	if got.Total != 4 {
		t.Fatalf("Total = %d, want 4", got.Total)
	}
	// Assert: 3 ждут проверки (Never + Overdue + Oldest); Fresh ещё «on schedule».
	if got.DueWaiting != 3 {
		t.Fatalf("DueWaiting = %d, want 3", got.DueWaiting)
	}
	// Assert: ровно один никогда не проверялся.
	if got.NeverChecked != 1 {
		t.Fatalf("NeverChecked = %d, want 1", got.NeverChecked)
	}
	// Assert: MostOverdue — монитор с самым старым LastCheckedAt.
	if !got.MostOverdue.HasOverdue || got.MostOverdue.Name != "Oldest" {
		t.Fatalf("MostOverdue = %+v, want Oldest", got.MostOverdue)
	}
	// Assert: просрочка Oldest = 30m − 1m (интервал) = 29m относительно due-логики.
	if got.MostOverdue.OverdueBy != "29m 0s" {
		t.Fatalf("OverdueBy = %q, want 29m 0s", got.MostOverdue.OverdueBy)
	}
}

func TestPercentOf(t *testing.T) {
	// Табличный тест: процент заполнения с clamping на 0..100.
	tests := []struct {
		name  string
		value int
		max   int
		want  int
	}{
		{name: "half", value: 25, max: 50, want: 50},
		{name: "zero max", value: 5, max: 0, want: 0},
		{name: "zero value", value: 0, max: 10, want: 0},
		{name: "over capacity", value: 12, max: 10, want: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act + Assert: деление на ноль и переполнение не ломают UI-шкалы.
			if got := percentOf(tt.value, tt.max); got != tt.want {
				t.Fatalf("percentOf(%d, %d) = %d, want %d", tt.value, tt.max, got, tt.want)
			}
		})
	}
}

func TestBuildUtilizationGauges(t *testing.T) {
	// Arrange: типичные значения worker.Stats для четырёх gauge-метрик.
	gauges := buildUtilizationGauges(worker.Stats{
		DueThisWave:    10,
		InFlight:       4,
		WaitingForSlot: 6,
		MaxConcurrency: 8,
		ResultQueued:   512,
		ResultCapacity: 2048,
		NotifyQueued:   2,
		NotifyCapacity: 256,
	})
	// Assert: ровно 4 gauge — check slots, waiting, result queue, notify queue.
	if len(gauges) != 4 {
		t.Fatalf("len = %d, want 4", len(gauges))
	}
	// Assert: check slots — 4/8 = 50%.
	if gauges[0].Label != "Check slots" || gauges[0].Percent != 50 || gauges[0].Detail != "4 / 8" {
		t.Fatalf("check slots gauge = %+v", gauges[0])
	}
	// Assert: waiting — 6/10 due this wave = 60%.
	if gauges[1].Label != "Waiting for slot" || gauges[1].Percent != 60 || gauges[1].Detail != "6 / 10" {
		t.Fatalf("waiting gauge = %+v", gauges[1])
	}
	// Assert: result queue — 512/2048 = 25%.
	if gauges[2].Label != "Result queue" || gauges[2].Percent != 25 || gauges[2].Detail != "512 / 2048" {
		t.Fatalf("result gauge = %+v", gauges[2])
	}
	// Assert: notify queue — 2/256 округляется до 0% (малый процент).
	if gauges[3].Label != "Notify queue" || gauges[3].Percent != 0 || gauges[3].Detail != "2 / 256" {
		t.Fatalf("notify gauge = %+v", gauges[3])
	}
}

func TestBuildFleetComposition(t *testing.T) {
	// Arrange: 2 up, 1 down, 1 без LastCheckedAt/IsUp (unknown).
	now := time.Now()
	up := true
	down := false
	monitors := []models.MonitorURL{
		{LastCheckedAt: &now, IsUp: &up},
		{LastCheckedAt: &now, IsUp: &up},
		{LastCheckedAt: &now, IsUp: &down},
		{},
	}
	// Act: строим pie-chart сегменты флота.
	got := buildFleetComposition(monitors)
	// Assert: Total = число мониторов.
	if got.Total != 4 {
		t.Fatalf("Total = %d, want 4", got.Total)
	}
	// Assert: сегмент up — 2 монитора.
	if got.Segments[0].Count != 2 || got.Segments[0].Modifier != "up" {
		t.Fatalf("up segment = %+v", got.Segments[0])
	}
	// Assert: сегмент down — 1 монитор.
	if got.Segments[1].Count != 1 || got.Segments[1].Modifier != "down" {
		t.Fatalf("down segment = %+v", got.Segments[1])
	}
	// Assert: unknown — монитор без статуса.
	if got.Segments[2].Count != 1 || got.Segments[2].Modifier != "unknown" {
		t.Fatalf("unknown segment = %+v", got.Segments[2])
	}
	// Assert: 2 из 4 = 50% для up.
	if got.Segments[0].Percent != 50 {
		t.Fatalf("up percent = %d, want 50", got.Segments[0].Percent)
	}
}

func TestBuildBacklogComposition(t *testing.T) {
	// Arrange: Total=5, DueWaiting=3, NeverChecked=1 → On schedule = 5−3 = 2, Due (не never) = 3−1 = 2.
	got := buildBacklogComposition(monitorBacklog{
		Total:        5,
		DueWaiting:   3,
		NeverChecked: 1,
	})
	// Assert: Total совпадает с входом.
	if got.Total != 5 {
		t.Fatalf("Total = %d, want 5", got.Total)
	}
	// Assert: Due — просроченные, но уже хотя бы раз проверенные.
	if got.Segments[0].Label != "Due" || got.Segments[0].Count != 2 {
		t.Fatalf("due segment = %+v", got.Segments[0])
	}
	// Assert: Never checked — отдельный сегмент.
	if got.Segments[1].Label != "Never checked" || got.Segments[1].Count != 1 {
		t.Fatalf("never segment = %+v", got.Segments[1])
	}
	// Assert: On schedule — остаток флота, не ждущий проверки.
	if got.Segments[2].Label != "On schedule" || got.Segments[2].Count != 2 {
		t.Fatalf("schedule segment = %+v", got.Segments[2])
	}
}

func TestBuildHeartbeatHourChart(t *testing.T) {
	// Arrange: «сейчас» с секундами — chart строится по минутным bucket'ам.
	now := time.Date(2026, 7, 25, 15, 30, 40, 0, time.UTC)
	current := now.Truncate(time.Minute)
	counts := []models.HeartbeatMinuteCount{
		{BucketAt: current.Add(-2 * time.Minute), Success: 3, Failed: 1},
		{BucketAt: current, Success: 0, Failed: 2},
	}

	// Act: строим часовой график heartbeat'ов.
	got := buildHeartbeatHourChart(counts, now)
	// Assert: фиксированное число минутных столбцов (HeartbeatHourMinutes).
	if len(got.Bars) != models.HeartbeatHourMinutes {
		t.Fatalf("len(Bars) = %d, want %d", len(got.Bars), models.HeartbeatHourMinutes)
	}
	// Assert: MaxPerMinute — максимум total за минуту (3+1=4).
	if got.MaxPerMinute != 4 {
		t.Fatalf("MaxPerMinute = %d, want 4", got.MaxPerMinute)
	}
	// Assert: агрегированные totals по всему часу.
	if got.TotalSuccess != 3 || got.TotalFailed != 3 || got.Total != 6 {
		t.Fatalf("totals = success %d failed %d total %d", got.TotalSuccess, got.TotalFailed, got.Total)
	}

	// Assert: «busy» bar — bucket −2 мин от текущей минуты; height 100% как самый загруженный.
	busy := got.Bars[models.HeartbeatHourMinutes-3]
	if busy.Success != 3 || busy.Failed != 1 || busy.Total != 4 {
		t.Fatalf("busy bar = %+v", busy)
	}
	if busy.HeightPercent != 100 {
		t.Fatalf("busy HeightPercent = %d, want 100", busy.HeightPercent)
	}
	// Assert: доли success/failed внутри столбца (3/4=75%, 1/4=25%).
	if busy.SuccessPercent != 75 || busy.FailedPercent != 25 {
		t.Fatalf("busy shares = success %d failed %d", busy.SuccessPercent, busy.FailedPercent)
	}

	// Assert: latest bar — текущая минута; только failed, height 50% (2/4 от max).
	latest := got.Bars[models.HeartbeatHourMinutes-1]
	if latest.Success != 0 || latest.Failed != 2 || latest.HeightPercent != 50 {
		t.Fatalf("latest bar = %+v", latest)
	}
	if latest.SuccessPercent != 0 || latest.FailedPercent != 100 {
		t.Fatalf("latest shares = success %d failed %d", latest.SuccessPercent, latest.FailedPercent)
	}

	// Assert: пустой bar в начале окна — нулевые counts и height.
	empty := got.Bars[0]
	if empty.Total != 0 || empty.HeightPercent != 0 {
		t.Fatalf("empty bar = %+v", empty)
	}
}

func TestMarshalHeartbeatHourChartJSON(t *testing.T) {
	// Arrange: минимальный chart с одной точкой данных.
	now := time.Date(2026, 7, 25, 15, 30, 0, 0, time.UTC)
	chart := buildHeartbeatHourChart([]models.HeartbeatMinuteCount{
		{BucketAt: now, Success: 2, Failed: 1},
	}, now)

	// Act: сериализуем для фронтенда (info-diagnostics chart).
	got, err := marshalHeartbeatHourChartJSON(chart)
	if err != nil {
		t.Fatalf("marshalHeartbeatHourChartJSON: %v", err)
	}
	raw := string(got)
	// Assert: JSON содержит ожидаемые ключи массивов для Chart.js или аналога.
	if !strings.Contains(raw, `"labels"`) || !strings.Contains(raw, `"success"`) || !strings.Contains(raw, `"failed"`) {
		t.Fatalf("unexpected JSON: %s", raw)
	}
	// Assert: метка текущей минуты в labels (15:30).
	if !strings.Contains(raw, `"15:30"`) {
		t.Fatalf("missing current label in JSON: %s", raw)
	}
}

func TestApplicationTableModels(t *testing.T) {
	// Arrange: канонический порядок и имена таблиц для info/diagnostics.
	want := []string{
		"app_settings",
		"incidents",
		"monitor_checks",
		"monitor_urls",
		"stat_daily",
		"stat_hourly",
		"stat_minutely",
		"users",
	}
	// Assert: список моделей для подсчёта row count не урезан.
	if len(applicationTableModels) != len(want) {
		t.Fatalf("len = %d, want %d", len(applicationTableModels), len(want))
	}
	for i, entry := range applicationTableModels {
		// Assert: имя таблицы и привязка к GORM-модели на каждой позиции.
		if entry.name != want[i] {
			t.Fatalf("applicationTableModels[%d].name = %q, want %q", i, entry.name, want[i])
		}
		if entry.model == nil {
			t.Fatalf("applicationTableModels[%d].model is nil", i)
		}
	}
}
