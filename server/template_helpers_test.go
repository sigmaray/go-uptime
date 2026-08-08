package server

import (
	"testing"

	"go-uptime/models"
)

func TestFormatUptimePercent(t *testing.T) {
	// Assert: пустая сводка (TotalSeconds=0) — показываем тире, а не «0%» или NaN.
	if got := formatUptimePercent(models.UptimeSummary{}); got != "—" {
		t.Fatalf("formatUptimePercent empty = %q, want dash", got)
	}
	// Assert: 90 из 100 секунд — ровно два знака после запятой.
	if got := formatUptimePercent(models.UptimeSummary{UpSeconds: 90, TotalSeconds: 100}); got != "90.00%" {
		t.Fatalf("formatUptimePercent = %q, want 90.00%%", got)
	}
}

func TestCheckIntervalFormValue(t *testing.T) {
	// Assert: nil CheckIntervalSeconds — пустая строка в форме (наследование глобального интервала).
	if got := checkIntervalFormValue(models.MonitorURL{}); got != "" {
		t.Fatalf("nil interval = %q, want empty", got)
	}
	// Arrange: явный интервал 120 секунд.
	seconds := 120
	// Assert: значение для input type=text — десятичная строка без суффиксов.
	if got := checkIntervalFormValue(models.MonitorURL{CheckIntervalSeconds: &seconds}); got != "120" {
		t.Fatalf("custom interval = %q, want 120", got)
	}
}
