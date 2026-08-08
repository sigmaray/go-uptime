package handlers

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const monitorsListPath = "/admin/monitors"

// monitorsListReturnURL reads a user-supplied return_to value from the request and
// returns a safe monitors list URL. c is the Gin request context; return_to may be a
// form field (POST) or query parameter (GET). Invalid or missing values fall back to
// the bare monitors list path.
func monitorsListReturnURL(c *gin.Context) string {
	raw := strings.TrimSpace(c.PostForm("return_to"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("return_to"))
	}
	return safeMonitorsListReturnURL(raw)
}

// safeMonitorsListReturnURL validates raw as a relative return URL for the monitors
// list page. raw is typically a return_to form or query value that may include list
// filters, sort, and page. Only paths exactly equal to /admin/monitors with an
// optional query string are accepted; anything else returns /admin/monitors.
func safeMonitorsListReturnURL(raw string) string {
	if raw == "" {
		return monitorsListPath
	}

	u, err := url.Parse(raw)
	if err != nil {
		return monitorsListPath
	}
	if u.Scheme != "" || u.Opaque != "" || u.User != nil || u.Host != "" {
		return monitorsListPath
	}
	if u.Path != monitorsListPath {
		return monitorsListPath
	}

	if u.RawQuery == "" {
		return monitorsListPath
	}
	return monitorsListPath + "?" + u.RawQuery
}
