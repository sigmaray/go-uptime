package applog

import (
	"bytes"
	"encoding/json"
	"strconv"
	"sync"
	"time"
)

// CaptureWriter дублирует JSON-строки zerolog в кольцевой буфер логов в памяти.
type CaptureWriter struct {
	mu  sync.Mutex
	buf []byte
}

// NewCaptureWriter возвращает writer, который разбирает вывод zerolog JSON в RecentLogs.
func NewCaptureWriter() *CaptureWriter {
	return &CaptureWriter{}
}

// Write реализует io.Writer и буферизует неполные строки до получения перевода строки.
func (w *CaptureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			// Неполная строка — ждём следующий Write.
			break
		}
		line := w.buf[:idx]
		w.buf = w.buf[idx+1:]
		parseZerologLine(line)
	}
	return len(p), nil
}

// parseZerologLine декодирует одну JSON-строку zerolog в Entry и кладёт в кольцевой буфер логов.
// Поля level, time, message → Entry; остальное → Fields. warn/error/fatal/panic дублируются в буфер ошибок.
func parseZerologLine(line []byte) {
	if len(line) == 0 {
		return
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		// Невалидный JSON — сохраняем сырую строку как unknown-level log.
		addLogEntry(Entry{
			Time:    time.Now(),
			Level:   "unknown",
			Message: string(line),
		})
		return
	}

	level := decodeStringField(fields["level"])
	if level == "" {
		level = "info"
	}

	message := decodeStringField(fields["message"])
	entryTime := decodeTimeField(fields["time"])

	// Все поля кроме level/time/message — structured context в Fields.
	extra := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		if key == "level" || key == "time" || key == "message" {
			continue
		}
		extra[key] = value
	}

	var fieldsJSON json.RawMessage
	if len(extra) > 0 {
		encoded, err := json.Marshal(extra)
		if err == nil {
			fieldsJSON = encoded
		}
	}

	entry := Entry{
		Time:    entryTime,
		Level:   level,
		Message: message,
		Fields:  fieldsJSON,
	}
	addLogEntry(entry)

	// warn+ попадают и в /admin/errors с теми же structured fields для расследования.
	if isCapturedErrorLevel(level) {
		addErrorEntry(entry)
	}
}

// isCapturedErrorLevel сообщает, должен ли уровень zerolog появляться на /admin/errors.
// level — строка уровня zerolog из JSON-строки лога (например "warn" или "error").
func isCapturedErrorLevel(level string) bool {
	switch level {
	case "warn", "error", "fatal", "panic":
		return true
	default:
		return false
	}
}

func decodeStringField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// decodeTimeField читает поле "time" из JSON zerolog (unix int/string или float epoch).
func decodeTimeField(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Now()
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if unix, err := strconv.ParseInt(asString, 10, 64); err == nil {
			return time.Unix(unix, 0)
		}
	}

	var asNumber float64
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		// Zerolog может писать time как float epoch с дробной частью (наносекунды).
		sec := int64(asNumber)
		nsec := int64((asNumber - float64(sec)) * float64(time.Second))
		return time.Unix(sec, nsec)
	}

	return time.Now()
}
