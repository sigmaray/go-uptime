// Package applog хранит недавние записи логов, события и ошибки в памяти процесса.
package applog

import (
	"encoding/json"
	"sync"
	"time"
)

const maxEntries = 200

// Entry — одна запись лога в памяти.
type Entry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	// Fields — необязательный структурированный контекст в виде JSON (object, array или scalar).
	Fields json.RawMessage `json:"fields,omitempty"`
}

// EventEntry — событие приложения (не HTTP-запрос).
type EventEntry struct {
	Time     time.Time `json:"time"`
	Category string    `json:"category"`
	Message  string    `json:"message"`
}

// MonitorRequestEntry — HTTP-запрос worker к мониторимому сайту.
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

// AddLog добавляет запись лога в кольцевой буфер в памяти.
// level — уровень серьёзности; message — человекочитаемое описание.
// fields — необязательный контекст: валидный JSON сохраняется как есть, обычный текст становится JSON-строкой.
func AddLog(level, message, fields string) {
	addLogEntry(Entry{
		Time:    time.Now(),
		Level:   level,
		Message: message,
		Fields:  EncodeFields(fields),
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

// AddError добавляет запись уровня error в кольцевой буфер.
// message — человекочитаемое описание; fields — необязательный структурированный контекст
// (валидный JSON или обычный текст), показываемый на странице admin/errors для расследования.
func AddError(message, fields string) {
	addErrorEntry(Entry{
		Time:    time.Now(),
		Level:   "error",
		Message: message,
		Fields:  EncodeFields(fields),
	})
}

// EncodeFields преобразует необязательный контекст в JSON, подходящий для Entry.Fields.
// raw — либо валидный JSON (object/array/scalar), либо обычный текст; обычный текст
// оборачивается в JSON-строку, чтобы диагностика могла встраивать fields без экранирования.
func EncodeFields(raw string) json.RawMessage {
	if raw == "" {
		return nil
	}
	if json.Valid([]byte(raw)) {
		// Уже валидный JSON — сохраняем как structured fields без перекодирования.
		return json.RawMessage(raw)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	return encoded
}

// addErrorEntry добавляет одну запись error/warn в кольцевой буфер ошибок в памяти.
// entry — полностью заполненная запись лога для страницы admin/errors.
func addErrorEntry(entry Entry) {
	errorMu.Lock()
	defer errorMu.Unlock()
	appErrors = appendRing(appErrors, entry)
}

// RecentLogs возвращает самые новые записи логов (не более maxEntries), сначала новые.
func RecentLogs() []Entry {
	logMu.RLock()
	defer logMu.RUnlock()
	out := copyEntries(logs)
	// Буфер хранит хронологический порядок append — для UI нужны сначала новые.
	reverseEntries(out)
	return out
}

// AddMonitorRequest сохраняет HTTP-запрос worker к мониторимому сайту в памяти.
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

// RecentEvents возвращает самые новые события приложения (не более maxEntries), сначала новые.
func RecentEvents() []EventEntry {
	eventMu.RLock()
	defer eventMu.RUnlock()
	out := copyEventEntries(events)
	reverseEventEntries(out)
	return out
}

// RecentMonitorRequests возвращает самые новые HTTP-запросы мониторов (не более maxEntries), сначала новые.
func RecentMonitorRequests() []MonitorRequestEntry {
	requestMu.RLock()
	defer requestMu.RUnlock()
	out := copyMonitorRequestEntries(requests)
	reverseMonitorRequestEntries(out)
	return out
}

// RecentErrors возвращает самые новые записи ошибок (не более maxEntries), сначала новые.
func RecentErrors() []Entry {
	errorMu.RLock()
	defer errorMu.RUnlock()
	out := copyEntries(appErrors)
	reverseEntries(out)
	return out
}

// CountEvents возвращает, сколько событий приложения хранится в памяти.
func CountEvents() int64 {
	eventMu.RLock()
	defer eventMu.RUnlock()
	return int64(len(events))
}

// CountErrors возвращает, сколько записей ошибок хранится в памяти.
func CountErrors() int64 {
	errorMu.RLock()
	defer errorMu.RUnlock()
	return int64(len(appErrors))
}

// CountMonitorRequests возвращает, сколько HTTP-запросов мониторов хранится в памяти.
func CountMonitorRequests() int64 {
	requestMu.RLock()
	defer requestMu.RUnlock()
	return int64(len(requests))
}

// EventsPage возвращает одну страницу событий приложения, отсортированных от новых к старым.
//
// page — номер страницы, начиная с 1.
// perPage — сколько событий на странице.
func EventsPage(page, perPage int) []EventEntry {
	eventMu.RLock()
	buf := copyEventEntries(events)
	eventMu.RUnlock()

	reverseEventEntries(buf)
	return slicePage(buf, page, perPage)
}

// ErrorsPage возвращает одну страницу записей ошибок, отсортированных от новых к старым.
//
// page — номер страницы, начиная с 1.
// perPage — сколько ошибок на странице.
func ErrorsPage(page, perPage int) []Entry {
	errorMu.RLock()
	buf := copyEntries(appErrors)
	errorMu.RUnlock()

	reverseEntries(buf)
	return slicePage(buf, page, perPage)
}

// MonitorRequestsPage возвращает одну страницу HTTP-запросов мониторов, отсортированных от новых к старым.
//
// page — номер страницы, начиная с 1.
// perPage — сколько запросов на странице.
func MonitorRequestsPage(page, perPage int) []MonitorRequestEntry {
	requestMu.RLock()
	buf := copyMonitorRequestEntries(requests)
	requestMu.RUnlock()

	reverseMonitorRequestEntries(buf)
	return slicePage(buf, page, perPage)
}

// ClearAll очищает все буферы логов в памяти. Используется Playwright test API.
// Блокируем все mutex по порядку log→error→event→request; e2e сбрасывает состояние
// перед сценарием, чтобы assertions не видели записи от предыдущих тестов.
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

// slicePage возвращает срез items для страницы page (1-based).
// Вызывающий код уже развернул буфер «сначала новые» — страница 1 = самые свежие записи.
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
		// Запрос страницы за пределами данных.
		return nil
	}
	end := offset + perPage
	if end > total {
		end = total
	}
	return items[offset:end]
}

// appendRing добавляет запись в кольцевой буфер фиксированного размера maxEntries.
// При переполнении отбрасываются самые старые элементы (хвост среза).
func appendRing[T any](buf []T, entry T) []T {
	buf = append(buf, entry)
	if len(buf) > maxEntries {
		// Кольцевой буфер: отрезаем старейшие записи с начала среза.
		return buf[len(buf)-maxEntries:]
	}
	return buf
}

// copyEntries возвращает глубокую копию записей логов/ошибок, чтобы вызывающий код мог безопасно изменять их.
// buf — срез кольцевого буфера для клонирования; срезы Fields копируются независимо.
func copyEntries(buf []Entry) []Entry {
	out := make([]Entry, len(buf))
	for i := range buf {
		out[i] = buf[i]
		if len(buf[i].Fields) > 0 {
			out[i].Fields = append(json.RawMessage(nil), buf[i].Fields...)
		}
	}
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
