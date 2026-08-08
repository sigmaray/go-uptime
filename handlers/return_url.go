package handlers

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// Безопасный return_to после create/edit/delete монитора: только whitelist пути списка,
// чтобы open redirect через поле формы был невозможен.

const monitorsListPath = "/admin/monitors"

// monitorsListReturnURL читает пользовательское значение return_to из запроса и
// возвращает безопасный URL списка мониторов. c — контекст Gin-запроса; return_to может быть
// полем формы (POST) или query-параметром (GET). Некорректные или отсутствующие значения
// приводят к базовому пути списка мониторов.
func monitorsListReturnURL(c *gin.Context) string {
	// POST-форма приоритетнее query (edit form vs GET-ссылки).
	raw := strings.TrimSpace(c.PostForm("return_to"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("return_to"))
	}
	return safeMonitorsListReturnURL(raw)
}

// safeMonitorsListReturnURL проверяет raw как относительный return URL для страницы
// списка мониторов. Whitelist намеренно узкий:
//   • только путь /admin/monitors (сохраняем фильтры/сортировку/страницу в query);
//   • отклоняются абсолютные URL, другие хосты, scheme и path traversal — иначе злоумышленник
//     мог бы подставить return_to=https://evil.example после успешного POST.
// Некорректные значения безопасно сводятся к /admin/monitors без query.
func safeMonitorsListReturnURL(raw string) string {
	if raw == "" {
		return monitorsListPath
	}

	u, err := url.Parse(raw)
	if err != nil {
		return monitorsListPath
	}
	// Отклоняем абсолютные URL, другие хосты, userinfo — защита от open redirect.
	if u.Scheme != "" || u.Opaque != "" || u.User != nil || u.Host != "" {
		return monitorsListPath
	}
	if u.Path != monitorsListPath {
		return monitorsListPath // только /admin/monitors, не /admin/users и т.д.
	}

	if u.RawQuery == "" {
		return monitorsListPath
	}
	// Query (фильтры, sort, page) сохраняем — path уже проверен.
	return monitorsListPath + "?" + u.RawQuery
}
