package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSafeMonitorsListReturnURL(t *testing.T) {
	// Табличный тест: safeMonitorsListReturnURL должен пропускать только безопасные
	// относительные URL списка мониторов и отклонять open redirect / path traversal.
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "/admin/monitors"},
		{name: "bare path", in: "/admin/monitors", want: "/admin/monitors"},
		{name: "with filters", in: "/admin/monitors?status=down&q=api", want: "/admin/monitors?status=down&q=api"},
		{name: "with sort and page", in: "/admin/monitors?order=asc&page=2&sort=URL&status=down", want: "/admin/monitors?order=asc&page=2&sort=URL&status=down"},
		{name: "absolute http", in: "http://evil.example/admin/monitors", want: "/admin/monitors"},
		{name: "protocol relative", in: "//evil.example/admin/monitors", want: "/admin/monitors"},
		{name: "other admin path", in: "/admin/users", want: "/admin/monitors"},
		{name: "monitor detail", in: "/admin/monitors/1", want: "/admin/monitors"},
		{name: "path traversal", in: "/admin/monitors/../users", want: "/admin/monitors"},
		{name: "javascript", in: "javascript:alert(1)", want: "/admin/monitors"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act: санитизируем входной return URL.
			got := safeMonitorsListReturnURL(tt.in)
			// Assert: опасные и неверные пути должны сводиться к дефолтному списку мониторов.
			if got != tt.want {
				t.Fatalf("safeMonitorsListReturnURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMonitorsListReturnURLPrefersPostForm(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Arrange: POST с return_to в теле формы; в query-string тоже есть return_to — форма должна иметь приоритет.
	form := url.Values{}
	form.Set("return_to", "/admin/monitors?status=down&q=api")
	req := httptest.NewRequest(http.MethodPost, "/admin/monitors/1/delete?return_to=%2Fadmin%2Fmonitors%3Fstatus%3Dup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	// Act: извлекаем return URL из gin-контекста.
	got := monitorsListReturnURL(c)
	want := "/admin/monitors?status=down&q=api"
	// Assert: взято значение из POST-формы, а не из query.
	if got != want {
		t.Fatalf("monitorsListReturnURL() = %q, want %q", got, want)
	}
}

func TestMonitorsListReturnURLFromQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Arrange: GET-запрос без тела; return_to только в query-string (URL-encoded).
	req := httptest.NewRequest(http.MethodGet, "/admin/monitors/1/edit?return_to=%2Fadmin%2Fmonitors%3Fstatus%3Ddown%26sort%3DURL%26order%3Dasc", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	// Act: извлекаем return URL, когда формы нет.
	got := monitorsListReturnURL(c)
	want := "/admin/monitors?status=down&sort=URL&order=asc"
	// Assert: query-параметр корректно декодируется и сохраняет фильтры/сортировку.
	if got != want {
		t.Fatalf("monitorsListReturnURL() = %q, want %q", got, want)
	}
}
