// Package applog хранит последние записи логов, событий и ошибок в памяти процесса.
package applog

import (
	"sync"
	"time"
)

const maxEntries = 200

// Entry — одна запись в памяти.
type Entry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Fields  string    `json:"fields"`
}

// EventEntry — событие приложения (не HTTP-запрос).
type EventEntry struct {
	Time     time.Time `json:"time"`
	Category string    `json:"category"`
	Message  string    `json:"message"`
}

// MonitorRequestEntry — HTTP-запрос воркера к мониторируемому сайту.
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

// AddEvent добавляет событие приложения в кольцевой буфер.
func AddEvent(category, message string) {
	eventMu.Lock()
	defer eventMu.Unlock()
	events = appendRing(events, EventEntry{
		Time:     time.Now(),
		Category: category,
		Message:  message,
	})
}

// AddError добавляет запись об ошибке в кольцевой буфер.
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

// RecentLogs возвращает последние записи логов (не более maxEntries), новые первыми.
func RecentLogs() []Entry {
	logMu.RLock()
	defer logMu.RUnlock()
	out := copyEntries(logs)
	reverseEntries(out)
	return out
}

// AddMonitorRequest сохраняет HTTP-запрос воркера к мониторируемому сайту в памяти.
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

// RecentEvents возвращает последние события приложения (не более maxEntries), новые первыми.
func RecentEvents() []EventEntry {
	eventMu.RLock()
	defer eventMu.RUnlock()
	out := copyEventEntries(events)
	reverseEventEntries(out)
	return out
}

// RecentMonitorRequests возвращает последние HTTP-запросы к мониторам (не более maxEntries), новые первыми.
func RecentMonitorRequests() []MonitorRequestEntry {
	requestMu.RLock()
	defer requestMu.RUnlock()
	out := copyMonitorRequestEntries(requests)
	reverseMonitorRequestEntries(out)
	return out
}

// RecentErrors возвращает последние записи ошибок (не более maxEntries).
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
