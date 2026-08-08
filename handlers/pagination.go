package handlers

import (
	"fmt"
	"net/url"
	"strconv"

	"go-uptime/models"
)

// PaginationPageLink — одна пронумерованная ссылка на страницу в пагинированном списке.
type PaginationPageLink struct {
	Page   int
	URL    string
	Active bool
}

// PaginationView содержит состояние пагинации и навигационные ссылки для шаблонов.
type PaginationView struct {
	Label      string
	Page       int
	TotalPages int
	Total      int64
	HasPrev    bool
	HasNext    bool
	PrevURL    string
	NextURL    string
	Pages      []PaginationPageLink
}

// parseQueryPage читает номер страницы (с единицы) из значения query string.
func parseQueryPage(raw string) int {
	page, err := strconv.Atoi(raw)
	if err != nil || page < 1 {
		return 1 // некорректный или отсутствующий page → первая страница
	}
	return page
}

// buildMonitorShowURL формирует URL страницы монитора с сохранением обоих параметров пагинации.
func buildMonitorShowURL(monitorID uint, incidentsPage, heartbeatsPage int) string {
	path := fmt.Sprintf("/admin/monitors/%d", monitorID)
	q := url.Values{}
	// page=1 не пишем в URL — это дефолт.
	if incidentsPage > 1 {
		q.Set("incidents_page", strconv.Itoa(incidentsPage))
	}
	if heartbeatsPage > 1 {
		q.Set("heartbeats_page", strconv.Itoa(heartbeatsPage))
	}
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// buildAdminListURL формирует пагинированный URL списка админки с параметром page.
// path — путь списка админки, например "/admin/users".
// page — номер страницы (с единицы); для страницы 1 query string опускается.
func buildAdminListURL(path string, page int) string {
	return buildAdminListURLWithQuery(path, page, nil)
}

// buildAdminListURLWithQuery формирует URL списка админки с page и дополнительными query-параметрами.
// path — путь списка админки, например "/admin/monitors".
// page — номер страницы (с единицы); для страницы 1 параметр page опускается.
// query — дополнительные параметры, например sort и order; nil или пустое значение допустимы.
func buildAdminListURLWithQuery(path string, page int, query url.Values) string {
	q := url.Values{}
	for key, values := range query {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	if len(q) == 0 {
		return path // чистый path без ? для первой страницы без фильтров
	}
	return path + "?" + q.Encode()
}

// buildPaginationView подготавливает данные шаблона для пагинированной секции списка.
// Сейчас для каждой страницы от 1 до totalPages формируется отдельная ссылка —
// для типичных размеров admin-списков этого достаточно (без «окна» страниц).
func buildPaginationView(total int64, page, perPage int, label string, urlForPage func(page int) string) PaginationView {
	page = models.ClampPage(page, total, perPage)
	totalPages := models.TotalPages(total, perPage)

	view := PaginationView{
		Label:      label,
		Page:       page,
		TotalPages: totalPages,
		Total:      total,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
	}
	if view.HasPrev {
		view.PrevURL = urlForPage(page - 1)
	}
	if view.HasNext {
		view.NextURL = urlForPage(page + 1)
	}
	// Полный список страниц 1..N — без «окна» для типичных admin-списков.
	for p := 1; p <= totalPages; p++ {
		view.Pages = append(view.Pages, PaginationPageLink{
			Page:   p,
			URL:    urlForPage(p),
			Active: p == page,
		})
	}
	return view
}
