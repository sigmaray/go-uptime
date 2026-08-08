package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizeMonitorsStatus(t *testing.T) {
	// Табличный тест: нормализация фильтра status для списка мониторов.
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty is all", in: "", want: ""},
		{name: "all alias", in: "all", want: ""},
		{name: "up", in: "up", want: "up"},
		{name: "UP case", in: "UP", want: "up"},
		{name: "down", in: "down", want: "down"},
		{name: "unknown", in: "unknown", want: ""},
		{name: "spaces", in: "  down  ", want: "down"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act + Assert: неизвестные значения и «all» → пустая строка (без фильтра).
			got := normalizeMonitorsStatus(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeMonitorsStatus(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseMonitorsListFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Arrange: GET с status=up и q с пробелами (URL-encoded).
	req := httptest.NewRequest(http.MethodGet, "/admin/monitors?status=up&q=%20api.example%20", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	// Act: парсим фильтр из gin-контекста.
	got := parseMonitorsListFilter(c)
	// Assert: status нормализован.
	if got.Status != "up" {
		t.Fatalf("Status = %q, want up", got.Status)
	}
	// Assert: q trim'ится — « api.example » → «api.example».
	if got.Q != "api.example" {
		t.Fatalf("Q = %q, want api.example", got.Q)
	}
	// Assert: Path берётся из request URL.
	if got.Path != "/admin/monitors" {
		t.Fatalf("Path = %q, want /admin/monitors", got.Path)
	}
}

func TestMonitorsListFilterQueryValues(t *testing.T) {
	// Табличный тест: обратное преобразование фильтра в url.Values для ссылок.
	tests := []struct {
		name   string
		filter MonitorsListFilter
		want   string
	}{
		{name: "empty", filter: MonitorsListFilter{}, want: ""},
		{name: "status only", filter: MonitorsListFilter{Status: "down"}, want: "status=down"},
		{name: "q only", filter: MonitorsListFilter{Q: "foo"}, want: "q=foo"},
		{name: "both", filter: MonitorsListFilter{Status: "up", Q: "bar"}, want: "q=bar&status=up"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act + Assert: Encode() — стабильный порядок ключей для тестов.
			got := tt.filter.QueryValues().Encode()
			if got != tt.want {
				t.Fatalf("QueryValues().Encode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMonitorsListFilterStatusURL(t *testing.T) {
	// Arrange: активный фильтр up + поиск + sort по ID asc.
	filter := MonitorsListFilter{
		Path:   "/admin/monitors",
		Status: "up",
		Q:      "example.com",
	}
	sort := ListSort{Path: "/admin/monitors", Column: "ID", Order: "asc"}

	// Act: URL для переключения status на down.
	got := filter.StatusURL("down", sort)
	want := "/admin/monitors?order=asc&q=example.com&sort=ID&status=down"
	// Assert: сохраняются q и sort, меняется только status.
	if got != want {
		t.Fatalf("StatusURL(down) = %q, want %q", got, want)
	}

	// Act: переключение на «all» — status убирается из query.
	gotAll := filter.StatusURL("", sort)
	wantAll := "/admin/monitors?order=asc&q=example.com&sort=ID"
	if gotAll != wantAll {
		t.Fatalf("StatusURL(all) = %q, want %q", gotAll, wantAll)
	}
}

func TestListSortQueryValuesIncludesExtraQuery(t *testing.T) {
	// Arrange: ListSort с ExtraQuery — фильтры мониторов merge'ятся в pagination/sort URL.
	sort := ListSort{
		Path:   "/admin/monitors",
		Column: "URL",
		Order:  "desc",
		ExtraQuery: url.Values{
			"status": []string{"down"},
			"q":      []string{"api"},
		},
	}

	// Assert: QueryValues включает sort, order и extra params.
	got := sort.QueryValues().Encode()
	want := "order=desc&q=api&sort=URL&status=down"
	if got != want {
		t.Fatalf("QueryValues().Encode() = %q, want %q", got, want)
	}

	// Assert: PageURL сохраняет все query params + page.
	pageURL := sort.PageURL(2)
	wantPage := "/admin/monitors?order=desc&page=2&q=api&sort=URL&status=down"
	if pageURL != wantPage {
		t.Fatalf("PageURL(2) = %q, want %q", pageURL, wantPage)
	}
}

func TestMergeURLValues(t *testing.T) {
	// Act: merge нескольких url.Values; nil-блок игнорируется.
	got := mergeURLValues(
		url.Values{"sort": []string{"Name"}},
		url.Values{"status": []string{"up"}},
		nil,
	).Encode()
	want := "sort=Name&status=up"
	// Assert: ключи из всех non-nil maps присутствуют.
	if got != want {
		t.Fatalf("mergeURLValues = %q, want %q", got, want)
	}
}
