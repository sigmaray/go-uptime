package handlers

import (
	"testing"

	"go-uptime/models"
)

var monitorSortFields = []string{"Name", "URL", "IsUp", "LastCheckedAt", "LastError"}

var heartbeatSortFields = []string{"MonitorURL", "CheckedAt", "ResponseTimeMs", "IsUp"}

func TestParseListSort(t *testing.T) {
	tests := []struct {
		name         string
		model        any
		path         string
		defaultOrder string
		rawSort      string
		rawOrder     string
		fields       []string
		wantColumn   string
		wantOrder    string
	}{
		{
			name:         "empty defaults",
			model:        models.MonitorURL{},
			path:         "/admin/monitors",
			defaultOrder: "monitor_urls.created_at desc, monitor_urls.id asc",
			fields:       monitorSortFields,
		},
		{
			name:         "sort without order",
			model:        models.MonitorURL{},
			path:         "/admin/monitors",
			defaultOrder: "monitor_urls.created_at desc, monitor_urls.id asc",
			rawSort:      "Name",
			fields:       monitorSortFields,
		},
		{
			name:         "rejects unknown field",
			model:        models.MonitorURL{},
			path:         "/admin/monitors",
			defaultOrder: "monitor_urls.created_at desc, monitor_urls.id asc",
			rawSort:      "CreatedAt",
			rawOrder:     "asc",
			fields:       monitorSortFields,
		},
		{
			name:         "monitors Name asc",
			model:        models.MonitorURL{},
			path:         "/admin/monitors",
			defaultOrder: "monitor_urls.created_at desc, monitor_urls.id asc",
			rawSort:      "Name",
			rawOrder:     "asc",
			fields:       monitorSortFields,
			wantColumn:   "Name",
			wantOrder:    "asc",
		},
		{
			name:         "monitors IsUp case insensitive",
			model:        models.MonitorURL{},
			path:         "/admin/monitors",
			defaultOrder: "monitor_urls.created_at desc, monitor_urls.id asc",
			rawSort:      "isup",
			rawOrder:     "DESC",
			fields:       monitorSortFields,
			wantColumn:   "IsUp",
			wantOrder:    "desc",
		},
		{
			name:         "heartbeats MonitorURL",
			model:        models.MonitorCheck{},
			path:         "/admin/heartbeats",
			defaultOrder: "monitor_checks.checked_at desc, monitor_checks.id asc",
			rawSort:      "MonitorURL",
			rawOrder:     "asc",
			fields:       heartbeatSortFields,
			wantColumn:   "MonitorURL",
			wantOrder:    "asc",
		},
		{
			name:         "heartbeats ResponseTimeMs",
			model:        models.MonitorCheck{},
			path:         "/admin/heartbeats",
			defaultOrder: "monitor_checks.checked_at desc, monitor_checks.id asc",
			rawSort:      "ResponseTimeMs",
			rawOrder:     "desc",
			fields:       heartbeatSortFields,
			wantColumn:   "ResponseTimeMs",
			wantOrder:    "desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseListSort(tt.path, tt.model, tt.defaultOrder, tt.rawSort, tt.rawOrder, tt.fields...)
			if got.Path != tt.path {
				t.Fatalf("Path = %q, want %q", got.Path, tt.path)
			}
			if got.Column != tt.wantColumn || got.Order != tt.wantOrder {
				t.Fatalf("got column=%q order=%q, want column=%q order=%q", got.Column, got.Order, tt.wantColumn, tt.wantOrder)
			}
		})
	}
}

func TestListSortURLs(t *testing.T) {
	sort := ParseListSort("/admin/monitors", models.MonitorURL{}, "monitor_urls.created_at desc, monitor_urls.id asc", "Name", "asc", monitorSortFields...)

	if got := sort.AscURL("URL"); got != "/admin/monitors?order=asc&sort=URL" {
		t.Fatalf("AscURL(URL) = %q", got)
	}
	if got := sort.DescURL("Name"); got != "/admin/monitors?order=desc&sort=Name" {
		t.Fatalf("DescURL(Name) = %q", got)
	}
	if got := sort.PageURL(2); got != "/admin/monitors?order=asc&page=2&sort=Name" {
		t.Fatalf("PageURL(2) = %q", got)
	}
	if !sort.IsActiveAsc("Name") {
		t.Fatal("expected Name asc to be active")
	}
	if sort.IsActiveDesc("Name") {
		t.Fatal("did not expect Name desc to be active")
	}

	heartbeatSort := ParseListSort("/admin/heartbeats", models.MonitorCheck{}, "monitor_checks.checked_at desc, monitor_checks.id asc", "ResponseTimeMs", "desc", heartbeatSortFields...)
	if got := heartbeatSort.PageURL(2); got != "/admin/heartbeats?order=desc&page=2&sort=ResponseTimeMs" {
		t.Fatalf("heartbeats PageURL(2) = %q", got)
	}
}

func TestListSortPageURLDefaults(t *testing.T) {
	sort := ParseListSort("/admin/monitors", models.MonitorURL{}, "monitor_urls.created_at desc, monitor_urls.id asc", "", "", monitorSortFields...)
	if got := sort.PageURL(1); got != "/admin/monitors" {
		t.Fatalf("default page 1 = %q", got)
	}
	if got := sort.PageURL(2); got != "/admin/monitors?page=2" {
		t.Fatalf("default page 2 = %q", got)
	}
}

func TestSortableColumnsByFields(t *testing.T) {
	columns, stable := sortableColumnsByFields(models.MonitorURL{}, monitorSortFields)
	if stable != "monitor_urls.id asc" {
		t.Fatalf("stable = %q", stable)
	}
	if columns["Name"].expr != "monitor_urls.name" {
		t.Fatalf("Name expr = %q", columns["Name"].expr)
	}
	if columns["IsUp"].expr != "monitor_urls.is_up" {
		t.Fatalf("IsUp expr = %q", columns["IsUp"].expr)
	}
	if _, ok := columns["CreatedAt"]; ok {
		t.Fatal("CreatedAt should not be sortable")
	}

	hbColumns, hbStable := sortableColumnsByFields(models.MonitorCheck{}, heartbeatSortFields)
	if hbStable != "monitor_checks.id asc" {
		t.Fatalf("heartbeats stable = %q", hbStable)
	}
	monitor := hbColumns["MonitorURL"]
	if monitor.join == "" {
		t.Fatal("expected join for MonitorURL association")
	}
	if monitor.expr == "" {
		t.Fatal("expected expr for MonitorURL association")
	}
	if hbColumns["ResponseTimeMs"].expr != "monitor_checks.response_time_ms" {
		t.Fatalf("ResponseTimeMs expr = %q", hbColumns["ResponseTimeMs"].expr)
	}
}

func TestBuildAdminListURLWithQuery(t *testing.T) {
	got := buildAdminListURLWithQuery("/admin/monitors", 2, ListSort{Column: "Name", Order: "asc"}.QueryValues())
	want := "/admin/monitors?order=asc&page=2&sort=Name"
	if got != want {
		t.Fatalf("buildAdminListURLWithQuery() = %q, want %q", got, want)
	}

	if got := buildAdminListURL("/admin/users", 1); got != "/admin/users" {
		t.Fatalf("buildAdminListURL page 1 = %q", got)
	}
	if got := buildAdminListURL("/admin/users", 3); got != "/admin/users?page=3" {
		t.Fatalf("buildAdminListURL page 3 = %q", got)
	}
}
