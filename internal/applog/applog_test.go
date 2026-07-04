package applog

import "testing"

func TestRecentLogsNewestFirst(t *testing.T) {
	resetForTest()

	AddLog("info", "older entry", "")
	AddLog("info", "newer entry", "")

	logs := RecentLogs()
	if len(logs) != 2 {
		t.Fatalf("RecentLogs() len = %d, want 2", len(logs))
	}
	if logs[0].Message != "newer entry" {
		t.Fatalf("RecentLogs()[0] = %q, want %q", logs[0].Message, "newer entry")
	}
	if logs[1].Message != "older entry" {
		t.Fatalf("RecentLogs()[1] = %q, want %q", logs[1].Message, "older entry")
	}
}

func TestRecentLogsStoresZerologEntriesSeparatelyFromEvents(t *testing.T) {
	resetForTest()

	AddLog("info", "GET /admin/monitors", `{"status":200}`)
	AddEvent("monitor", `Created monitor "example.com" (https://example.com)`)

	logs := RecentLogs()
	if len(logs) != 1 {
		t.Fatalf("RecentLogs() len = %d, want 1", len(logs))
	}
	if logs[0].Message != "GET /admin/monitors" {
		t.Fatalf("unexpected log message: %q", logs[0].Message)
	}

	events := RecentEvents()
	if len(events) != 1 {
		t.Fatalf("RecentEvents() len = %d, want 1", len(events))
	}
	if events[0].Message != `Created monitor "example.com" (https://example.com)` {
		t.Fatalf("unexpected event message: %q", events[0].Message)
	}
}

func resetForTest() {
	logMu.Lock()
	errorMu.Lock()
	eventMu.Lock()
	logs = nil
	appErrors = nil
	events = nil
	eventMu.Unlock()
	errorMu.Unlock()
	logMu.Unlock()
}
