package handlers

import (
	"testing"

	"go-uptime/models"
)

var monitorSortFields = []string{"ID", "URL", "IsUp", "LastCheckedAt", "LastError"}

var heartbeatSortFields = []string{"MonitorURL", "CheckedAt", "ResponseTimeMs", "IsUp"}

func TestParseListSort(t *testing.T) {
	// Табличный тест ParseListSort: whitelist полей, default order, case-insensitive order.
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
			rawSort:      "ID",
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
			name:         "monitors ID asc",
			model:        models.MonitorURL{},
			path:         "/admin/monitors",
			defaultOrder: "monitor_urls.created_at desc, monitor_urls.id asc",
			rawSort:      "ID",
			rawOrder:     "asc",
			fields:       monitorSortFields,
			wantColumn:   "ID",
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
			// Act: парсим sort/order из query-string с whitelist fields.
			got := ParseListSort(tt.path, tt.model, tt.defaultOrder, tt.rawSort, tt.rawOrder, tt.fields...)
			// Assert: Path всегда сохраняется для генерации URL.
			if got.Path != tt.path {
				t.Fatalf("Path = %q, want %q", got.Path, tt.path)
			}
			// Assert: Column/Order — либо явные want*, либо пустые при fallback на defaultOrder.
			if got.Column != tt.wantColumn || got.Order != tt.wantOrder {
				t.Fatalf("got column=%q order=%q, want column=%q order=%q", got.Column, got.Order, tt.wantColumn, tt.wantOrder)
			}
		})
	}
}

func TestListSortURLs(t *testing.T) {
	// Arrange: активная сортировка monitors по ID asc.
	sort := ParseListSort("/admin/monitors", models.MonitorURL{}, "monitor_urls.created_at desc, monitor_urls.id asc", "ID", "asc", monitorSortFields...)

	// Assert: AscURL/DescURL меняют только sort+order, path сохраняется.
	if got := sort.AscURL("URL"); got != "/admin/monitors?order=asc&sort=URL" {
		t.Fatalf("AscURL(URL) = %q", got)
	}
	if got := sort.DescURL("ID"); got != "/admin/monitors?order=desc&sort=ID" {
		t.Fatalf("DescURL(ID) = %q", got)
	}
	// Assert: PageURL добавляет page, сохраняя текущий sort/order.
	if got := sort.PageURL(2); got != "/admin/monitors?order=asc&page=2&sort=ID" {
		t.Fatalf("PageURL(2) = %q", got)
	}
	// Assert: IsActiveAsc/IsActiveDesc — UI-подсветка активного столбца.
	if !sort.IsActiveAsc("ID") {
		t.Fatal("expected ID asc to be active")
	}
	if sort.IsActiveDesc("ID") {
		t.Fatal("did not expect ID desc to be active")
	}

	// Arrange: heartbeats с другим path и desc order.
	heartbeatSort := ParseListSort("/admin/heartbeats", models.MonitorCheck{}, "monitor_checks.checked_at desc, monitor_checks.id asc", "ResponseTimeMs", "desc", heartbeatSortFields...)
	// Assert: pagination URL для heartbeats использует свой path и order.
	if got := heartbeatSort.PageURL(2); got != "/admin/heartbeats?order=desc&page=2&sort=ResponseTimeMs" {
		t.Fatalf("heartbeats PageURL(2) = %q", got)
	}
}

func TestListSortPageURLDefaults(t *testing.T) {
	// Arrange: без rawSort/rawOrder — дефолтная сортировка из defaultOrder SQL.
	sort := ParseListSort("/admin/monitors", models.MonitorURL{}, "monitor_urls.created_at desc, monitor_urls.id asc", "", "", monitorSortFields...)
	// Assert: page=1 не добавляет query (канонический URL списка).
	if got := sort.PageURL(1); got != "/admin/monitors" {
		t.Fatalf("default page 1 = %q", got)
	}
	// Assert: page>1 добавляет только page, без sort/order.
	if got := sort.PageURL(2); got != "/admin/monitors?page=2" {
		t.Fatalf("default page 2 = %q", got)
	}
}

func TestSortableColumnsByFields(t *testing.T) {
	// Act: whitelist sortable columns для MonitorURL.
	columns, stable := sortableColumnsByFields(models.MonitorURL{}, monitorSortFields)
	// Assert: stable tie-breaker — id asc для детерминированной пагинации.
	if stable != "monitor_urls.id asc" {
		t.Fatalf("stable = %q", stable)
	}
	// Assert: SQL expr для whitelist-полей.
	if columns["ID"].expr != "monitor_urls.id" {
		t.Fatalf("ID expr = %q", columns["ID"].expr)
	}
	if columns["IsUp"].expr != "monitor_urls.is_up" {
		t.Fatalf("IsUp expr = %q", columns["IsUp"].expr)
	}
	// Assert: поля вне whitelist (CreatedAt, Name) не sortable.
	if _, ok := columns["CreatedAt"]; ok {
		t.Fatal("CreatedAt should not be sortable")
	}
	if _, ok := columns["Name"]; ok {
		t.Fatal("Name should not be sortable")
	}

	// Act: heartbeats — association MonitorURL требует JOIN.
	hbColumns, hbStable := sortableColumnsByFields(models.MonitorCheck{}, heartbeatSortFields)
	if hbStable != "monitor_checks.id asc" {
		t.Fatalf("heartbeats stable = %q", hbStable)
	}
	monitor := hbColumns["MonitorURL"]
	// Assert: join и expr заданы для сортировки по URL монитора.
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
	// Act: сборка URL списка с page и query values из ListSort.
	got := buildAdminListURLWithQuery("/admin/monitors", 2, ListSort{Column: "ID", Order: "asc"}.QueryValues())
	want := "/admin/monitors?order=asc&page=2&sort=ID"
	// Assert: sort, order и page в одном query string.
	if got != want {
		t.Fatalf("buildAdminListURLWithQuery() = %q, want %q", got, want)
	}

	// Assert: buildAdminListURL без sort — page 1 без query, page 3 только с page=.
	if got := buildAdminListURL("/admin/users", 1); got != "/admin/users" {
		t.Fatalf("buildAdminListURL page 1 = %q", got)
	}
	if got := buildAdminListURL("/admin/users", 3); got != "/admin/users?page=3" {
		t.Fatalf("buildAdminListURL page 3 = %q", got)
	}
}
