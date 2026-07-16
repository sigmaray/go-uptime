package handlers

import (
	"fmt"
	"net/url"
	"strconv"

	"go-uptime/models"
)

// PaginationPageLink is one numbered page link in a paginated list.
type PaginationPageLink struct {
	Page   int
	URL    string
	Active bool
}

// PaginationView holds pagination state and navigation links for templates.
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

// parseQueryPage reads a one-based page number from a query string value.
func parseQueryPage(raw string) int {
	page, err := strconv.Atoi(raw)
	if err != nil || page < 1 {
		return 1
	}
	return page
}

// buildMonitorShowURL builds the monitor detail URL preserving both pagination params.
func buildMonitorShowURL(monitorID uint, incidentsPage, heartbeatsPage int) string {
	path := fmt.Sprintf("/admin/monitors/%d", monitorID)
	q := url.Values{}
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

// buildAdminListURL builds a paginated admin list URL using the page query param.
func buildAdminListURL(path string, page int) string {
	if page <= 1 {
		return path
	}
	return path + "?page=" + strconv.Itoa(page)
}

// buildPaginationView prepares template data for a paginated list section.
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
	for p := 1; p <= totalPages; p++ {
		view.Pages = append(view.Pages, PaginationPageLink{
			Page:   p,
			URL:    urlForPage(p),
			Active: p == page,
		})
	}
	return view
}
