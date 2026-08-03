// Package applog stores recent log entries, events, and errors in process memory.
package applog

import (
	"sync"
	"time"
)

const maxEntries = 200

// Entry is a single in-memory log record.
type Entry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Fields  string    `json:"fields"`
}

// EventEntry is an application event (not an HTTP request).
type EventEntry struct {
	Time     time.Time `json:"time"`
	Category string    `json:"category"`
	Message  string    `json:"message"`
}

// MonitorRequestEntry is a worker HTTP request to a monitored site.
type MonitorRequestEntry struct {
	Time           time.Time `json:"time"`
	MonitorName    string    `json:"monitor_name"`
	URL            string    `json:"url"`
	StatusCode     int       `json:"status_code"`
	ResponseTimeMs int64     `json:"response_time_ms"`
	IsUp           bool      `json:"is_up"`
	Error          string    `json:"error"`
}

var (
	logMu     sync.RWMutex
	errorMu   sync.RWMutex
	eventMu   sync.RWMutex
	requestMu sync.RWMutex
	logs      []Entry
	appErrors []Entry
	events    []EventEntry
	requests  []MonitorRequestEntry
)

// AddLog appends a log entry to the in-memory ring buffer.
func AddLog(level, message, fields string) {
	addLogEntry(Entry{
		Time:    time.Now(),
		Level:   level,
		Message: message,
		Fields:  fields,
	})
}

func addLogEntry(entry Entry) {
	logMu.Lock()
	defer logMu.Unlock()
	logs = appendRing(logs, entry)
}

// AddEvent appends an application event to the ring buffer.
func AddEvent(category, message string) {
	eventMu.Lock()
	defer eventMu.Unlock()
	events = appendRing(events, EventEntry{
		Time:     time.Now(),
		Category: category,
		Message:  message,
	})
}

// AddError appends an error-level record to the ring buffer.
// message is the human-readable summary; fields is optional structured context
// (JSON or plain text) shown on the admin errors page for investigation.
func AddError(message, fields string) {
	addErrorEntry(Entry{
		Time:    time.Now(),
		Level:   "error",
		Message: message,
		Fields:  fields,
	})
}

// addErrorEntry appends one error/warn record to the in-memory errors ring buffer.
// entry is the fully populated log record to store for the admin errors page.
func addErrorEntry(entry Entry) {
	errorMu.Lock()
	defer errorMu.Unlock()
	appErrors = appendRing(appErrors, entry)
}

// RecentLogs returns the most recent log entries (at most maxEntries), newest first.
func RecentLogs() []Entry {
	logMu.RLock()
	defer logMu.RUnlock()
	out := copyEntries(logs)
	reverseEntries(out)
	return out
}

// AddMonitorRequest stores a worker HTTP request to a monitored site in memory.
func AddMonitorRequest(monitorName, url string, statusCode int, responseTimeMs int64, isUp bool, errMsg string) {
	requestMu.Lock()
	defer requestMu.Unlock()
	requests = appendRing(requests, MonitorRequestEntry{
		Time:           time.Now(),
		MonitorName:    monitorName,
		URL:            url,
		StatusCode:     statusCode,
		ResponseTimeMs: responseTimeMs,
		IsUp:           isUp,
		Error:          errMsg,
	})
}

// RecentEvents returns the most recent application events (at most maxEntries), newest first.
func RecentEvents() []EventEntry {
	eventMu.RLock()
	defer eventMu.RUnlock()
	out := copyEventEntries(events)
	reverseEventEntries(out)
	return out
}

// RecentMonitorRequests returns the most recent monitor HTTP requests (at most maxEntries), newest first.
func RecentMonitorRequests() []MonitorRequestEntry {
	requestMu.RLock()
	defer requestMu.RUnlock()
	out := copyMonitorRequestEntries(requests)
	reverseMonitorRequestEntries(out)
	return out
}

// RecentErrors returns the most recent error records (at most maxEntries), newest first.
func RecentErrors() []Entry {
	errorMu.RLock()
	defer errorMu.RUnlock()
	out := copyEntries(appErrors)
	reverseEntries(out)
	return out
}

// CountEvents returns how many application events are stored in memory.
func CountEvents() int64 {
	eventMu.RLock()
	defer eventMu.RUnlock()
	return int64(len(events))
}

// CountErrors returns how many error records are stored in memory.
func CountErrors() int64 {
	errorMu.RLock()
	defer errorMu.RUnlock()
	return int64(len(appErrors))
}

// CountMonitorRequests returns how many monitor HTTP requests are stored in memory.
func CountMonitorRequests() int64 {
	requestMu.RLock()
	defer requestMu.RUnlock()
	return int64(len(requests))
}

// EventsPage returns one page of application events ordered newest first.
//
// page is the one-based page number.
// perPage is how many events each page contains.
func EventsPage(page, perPage int) []EventEntry {
	eventMu.RLock()
	buf := copyEventEntries(events)
	eventMu.RUnlock()

	reverseEventEntries(buf)
	return slicePage(buf, page, perPage)
}

// ErrorsPage returns one page of error records ordered newest first.
//
// page is the one-based page number.
// perPage is how many errors each page contains.
func ErrorsPage(page, perPage int) []Entry {
	errorMu.RLock()
	buf := copyEntries(appErrors)
	errorMu.RUnlock()

	reverseEntries(buf)
	return slicePage(buf, page, perPage)
}

// MonitorRequestsPage returns one page of monitor HTTP requests ordered newest first.
//
// page is the one-based page number.
// perPage is how many requests each page contains.
func MonitorRequestsPage(page, perPage int) []MonitorRequestEntry {
	requestMu.RLock()
	buf := copyMonitorRequestEntries(requests)
	requestMu.RUnlock()

	reverseMonitorRequestEntries(buf)
	return slicePage(buf, page, perPage)
}

// ClearAll removes all in-memory log buffers. Used by the Playwright test API.
func ClearAll() {
	logMu.Lock()
	errorMu.Lock()
	eventMu.Lock()
	requestMu.Lock()
	logs = nil
	appErrors = nil
	events = nil
	requests = nil
	requestMu.Unlock()
	eventMu.Unlock()
	errorMu.Unlock()
	logMu.Unlock()
}

func slicePage[T any](items []T, page, perPage int) []T {
	if perPage < 1 {
		perPage = 1
	}
	if page < 1 {
		page = 1
	}
	total := len(items)
	if total == 0 {
		return nil
	}
	offset := (page - 1) * perPage
	if offset >= total {
		return nil
	}
	end := offset + perPage
	if end > total {
		end = total
	}
	return items[offset:end]
}

func appendRing[T any](buf []T, entry T) []T {
	buf = append(buf, entry)
	if len(buf) > maxEntries {
		return buf[len(buf)-maxEntries:]
	}
	return buf
}

func copyEntries(buf []Entry) []Entry {
	out := make([]Entry, len(buf))
	copy(out, buf)
	return out
}

func copyEventEntries(buf []EventEntry) []EventEntry {
	out := make([]EventEntry, len(buf))
	copy(out, buf)
	return out
}

func copyMonitorRequestEntries(buf []MonitorRequestEntry) []MonitorRequestEntry {
	out := make([]MonitorRequestEntry, len(buf))
	copy(out, buf)
	return out
}

func reverseEntries(buf []Entry) {
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
}

func reverseEventEntries(buf []EventEntry) {
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
}

func reverseMonitorRequestEntries(buf []MonitorRequestEntry) {
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
}
