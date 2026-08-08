package handlers

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Значения query-фильтра статуса списка мониторов.
const (
	monitorsStatusAll  = ""
	monitorsStatusUp   = "up"
	monitorsStatusDown = "down"
)

// MonitorsListFilter содержит фильтры статуса и фрагмента URL для списка мониторов админки.
type MonitorsListFilter struct {
	// Status — "up", "down" или пусто для всех мониторов.
	Status string
	// Q — регистронезависимая подстрока URL для поиска; пусто означает отсутствие фильтра по URL.
	Q string
	// Path — путь страницы списка, используемый при формировании URL фильтров.
	Path string
}

// parseMonitorsListFilter читает фильтры статуса и поиска по URL из query запроса.
// c — контекст Gin-запроса, чьи Query-значения проверяются.
func parseMonitorsListFilter(c *gin.Context) MonitorsListFilter {
	return MonitorsListFilter{
		Path:   "/admin/monitors",
		Status: normalizeMonitorsStatus(c.Query("status")),
		Q:      strings.TrimSpace(c.Query("q")), // поиск по подстроке URL
	}
}

// normalizeMonitorsStatus сопоставляет сырое значение query status с поддерживаемым фильтром.
// raw — query-параметр status из запроса.
func normalizeMonitorsStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case monitorsStatusUp:
		return monitorsStatusUp
	case monitorsStatusDown:
		return monitorsStatusDown
	default:
		return monitorsStatusAll
	}
}

// Apply добавляет WHERE-условия для активных фильтров статуса и фрагмента URL.
// db — GORM-запрос, уже ограниченный monitor_urls.
func (f MonitorsListFilter) Apply(db *gorm.DB) *gorm.DB {
	switch f.Status {
	case monitorsStatusUp:
		// Соответствует бейджу Up: проверен хотя бы раз и сейчас up.
		db = db.Where("monitor_urls.last_checked_at IS NOT NULL AND monitor_urls.is_up = ?", true)
	case monitorsStatusDown:
		// Соответствует бейджу Down: проверен хотя бы раз и сейчас не up.
		db = db.Where("monitor_urls.last_checked_at IS NOT NULL AND monitor_urls.is_up IS DISTINCT FROM ?", true)
	}
	if f.Q != "" {
		// strpos трактует needle как литерал, поэтому %, _ и \ не требуют экранирования.
		db = db.Where("strpos(lower(monitor_urls.url), lower(?)) > 0", f.Q)
	}
	return db
}

// QueryValues возвращает нестандартные параметры фильтра для ссылок пагинации и сортировки.
func (f MonitorsListFilter) QueryValues() url.Values {
	q := url.Values{}
	if f.Status != monitorsStatusAll {
		q.Set("status", f.Status)
	}
	if f.Q != "" {
		q.Set("q", f.Q)
	}
	if len(q) == 0 {
		return nil
	}
	return q
}

// IsActive сообщает, выбран ли указанный вариант фильтра статуса.
// status — "up", "down" или пусто для All.
func (f MonitorsListFilter) IsActive(status string) bool {
	return f.Status == normalizeMonitorsStatus(status)
}

// StatusURL формирует URL списка, переключающий фильтр статуса с сохранением поиска по URL.
// status — "up", "down" или пусто для All; ссылка сбрасывает на страницу 1.
// sort — активная сортировка списка; сохраняются только column и order (не ExtraQuery).
func (f MonitorsListFilter) StatusURL(status string, sort ListSort) string {
	next := f
	next.Status = normalizeMonitorsStatus(status)
	sortOnly := url.Values{}
	if !sort.IsDefault() {
		sortOnly.Set("sort", sort.Column)
		sortOnly.Set("order", sort.Order)
	}
	// Смена фильтра статуса → страница 1; поиск q сохраняется через next.QueryValues().
	return buildAdminListURLWithQuery(next.Path, 1, mergeURLValues(sortOnly, next.QueryValues()))
}

// mergeURLValues копирует ключи из каждого не-nil url.Values в новую map.
// values — variadic-список query map; более поздние записи перезаписывают более ранние ключи при конфликте.
func mergeURLValues(values ...url.Values) url.Values {
	merged := url.Values{}
	for _, item := range values {
		for key, vals := range item {
			for _, value := range vals {
				merged.Add(key, value) // поздние values перезаписывают ключи через Add-порядок
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
