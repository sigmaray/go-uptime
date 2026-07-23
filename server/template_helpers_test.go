package server

import (
	"testing"

	"go-uptime/models"
)

func TestFormatUptimePercent(t *testing.T) {
	if got := formatUptimePercent(models.UptimeSummary{}); got != "—" {
		t.Fatalf("formatUptimePercent empty = %q, want dash", got)
	}
	if got := formatUptimePercent(models.UptimeSummary{UpSeconds: 90, TotalSeconds: 100}); got != "90.00%" {
		t.Fatalf("formatUptimePercent = %q, want 90.00%%", got)
	}
}

func TestCheckIntervalFormValue(t *testing.T) {
	if got := checkIntervalFormValue(models.MonitorURL{}); got != "" {
		t.Fatalf("nil interval = %q, want empty", got)
	}
	seconds := 120
	if got := checkIntervalFormValue(models.MonitorURL{CheckIntervalSeconds: &seconds}); got != "120" {
		t.Fatalf("custom interval = %q, want 120", got)
	}
}
