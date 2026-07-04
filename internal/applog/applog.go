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

// AddError appends an error record to the ring buffer.
func AddError(message, fields string) {
	errorMu.Lock()
	defer errorMu.Unlock()
	appErrors = appendRing(appErrors, Entry{
		Time:    time.Now(),
		Level:   "error",
		Message: message,
		Fields:  fields,
	})
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

// RecentErrors returns the most recent error records (at most maxEntries).
func RecentErrors() []Entry {
	errorMu.RLock()
	defer errorMu.RUnlock()
	return copyEntries(appErrors)
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
