package models

const (
	// MonitorDetailListPageSize is how many incidents and heartbeats are shown
	// per page on `/admin/monitors/:id`.
	MonitorDetailListPageSize = 20

	// AdminListPageSize is how many rows admin index pages show per page.
	AdminListPageSize = 100
)

// TotalPages returns the number of pages needed for total items at perPage size.
func TotalPages(total int64, perPage int) int {
	if perPage < 1 {
		return 1
	}
	if total == 0 {
		return 1
	}
	return int((total + int64(perPage) - 1) / int64(perPage))
}

// ClampPage limits page to a valid range for the given total and page size.
func ClampPage(page int, total int64, perPage int) int {
	if page < 1 {
		page = 1
	}
	maxPage := TotalPages(total, perPage)
	if page > maxPage {
		return maxPage
	}
	return page
}

// PageOffset returns the SQL offset for a one-based page number.
func PageOffset(page, perPage int) int {
	return (page - 1) * perPage
}
