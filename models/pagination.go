package models

const (
	// MonitorDetailListPageSize — сколько инцидентов и heartbeat отображается
	// на одной странице `/admin/monitors/:id`.
	MonitorDetailListPageSize = 20

	// AdminListPageSize — сколько строк показывают индексные страницы админки на одной странице.
	AdminListPageSize = 100
)

// TotalPages возвращает число страниц для total элементов при размере страницы perPage.
func TotalPages(total int64, perPage int) int {
	if perPage < 1 {
		return 1
	}
	if total == 0 {
		// Пустой список — одна «пустая» страница, не ноль.
		return 1
	}
	return int((total + int64(perPage) - 1) / int64(perPage))
}

// ClampPage ограничивает page допустимым диапазоном для заданного total и размера страницы.
func ClampPage(page int, total int64, perPage int) int {
	if page < 1 {
		page = 1
	}
	maxPage := TotalPages(total, perPage)
	if page > maxPage {
		// Запрос page=999 при 3 страницах — показываем последнюю.
		return maxPage
	}
	return page
}

// PageOffset возвращает SQL-смещение для номера страницы, начинающегося с 1.
func PageOffset(page, perPage int) int {
	return (page - 1) * perPage
}
