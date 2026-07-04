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

var (
	logMu     sync.RWMutex
	errorMu   sync.RWMutex
	eventMu   sync.RWMutex
	logs      []Entry
	appErrors []Entry
	events    []EventEntry
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

// RecentEvents возвращает последние события приложения (не более maxEntries).
func RecentEvents() []EventEntry {
	eventMu.RLock()
	defer eventMu.RUnlock()
	return copyEventEntries(events)
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

func reverseEntries(buf []Entry) {
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
}
