package applog

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestEncodeFields(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "object json", in: `{"code":1}`, want: `{"code":1}`},
		{name: "plain text", in: `manual trigger`, want: `"manual trigger"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeFields(tt.in)
			if string(got) != tt.want {
				t.Fatalf("EncodeFields(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestAddErrorFieldsEmbedAsJSON(t *testing.T) {
	resetForTest()
	AddError("boom", `{"monitor_id":7}`)

	raw, err := json.Marshal(RecentErrors()[0])
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	fields, ok := parsed["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields is %T, want object; raw=%s", parsed["fields"], raw)
	}
	if fields["monitor_id"] != float64(7) {
		t.Fatalf("monitor_id = %#v, want 7", fields["monitor_id"])
	}
}

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

func TestRecentEventsNewestFirst(t *testing.T) {
	resetForTest()

	AddEvent("monitor", "older event")
	AddEvent("monitor", "newer event")

	events := RecentEvents()
	if len(events) != 2 {
		t.Fatalf("RecentEvents() len = %d, want 2", len(events))
	}
	if events[0].Message != "newer event" {
		t.Fatalf("RecentEvents()[0] = %q, want %q", events[0].Message, "newer event")
	}
	if events[1].Message != "older event" {
		t.Fatalf("RecentEvents()[1] = %q, want %q", events[1].Message, "older event")
	}
}

func TestRecentErrorsNewestFirst(t *testing.T) {
	resetForTest()

	AddError("older error", "ctx")
	AddError("newer error", "ctx")

	errors := RecentErrors()
	if len(errors) != 2 {
		t.Fatalf("RecentErrors() len = %d, want 2", len(errors))
	}
	if errors[0].Message != "newer error" {
		t.Fatalf("RecentErrors()[0] = %q, want %q", errors[0].Message, "newer error")
	}
	if errors[1].Message != "older error" {
		t.Fatalf("RecentErrors()[1] = %q, want %q", errors[1].Message, "older error")
	}
}

func TestEventsPageReturnsNewestFirstPage(t *testing.T) {
	resetForTest()

	for i := 1; i <= 105; i++ {
		AddEvent("test", fmt.Sprintf("event %d", i))
	}

	page1 := EventsPage(1, 100)
	if len(page1) != 100 {
		t.Fatalf("EventsPage(1, 100) len = %d, want 100", len(page1))
	}

	page2 := EventsPage(2, 100)
	if len(page2) != 5 {
		t.Fatalf("EventsPage(2, 100) len = %d, want 5", len(page2))
	}
}

func TestRecentMonitorRequestsNewestFirst(t *testing.T) {
	resetForTest()

	AddMonitorRequest("one", "https://one.example", 200, 10, true, "")
	AddMonitorRequest("two", "https://two.example", 500, 20, false, "server error")

	requests := RecentMonitorRequests()
	if len(requests) != 2 {
		t.Fatalf("RecentMonitorRequests() len = %d, want 2", len(requests))
	}
	if requests[0].MonitorName != "two" {
		t.Fatalf("RecentMonitorRequests()[0].MonitorName = %q, want two", requests[0].MonitorName)
	}
	if requests[1].MonitorName != "one" {
		t.Fatalf("RecentMonitorRequests()[1].MonitorName = %q, want one", requests[1].MonitorName)
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
	ClearAll()
}
