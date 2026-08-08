package applog

import (
	"bytes"
	"encoding/json"
	"strconv"
	"sync"
	"time"
)

// CaptureWriter mirrors zerolog JSON lines into the in-memory log ring buffer.
type CaptureWriter struct {
	mu  sync.Mutex
	buf []byte
}

// NewCaptureWriter returns a writer that parses zerolog JSON output into RecentLogs.
func NewCaptureWriter() *CaptureWriter {
	return &CaptureWriter{}
}

// Write implements io.Writer and buffers partial lines until a newline is received.
func (w *CaptureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := w.buf[:idx]
		w.buf = w.buf[idx+1:]
		parseZerologLine(line)
	}
	return len(p), nil
}

// parseZerologLine decodes one zerolog JSON line and stores it in the ring buffer.
func parseZerologLine(line []byte) {
	if len(line) == 0 {
		return
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
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

	// Mirror warn+ entries into the admin errors buffer with full structured fields
	// so production failures (error message, monitor_id, path, etc.) are investigable.
	if isCapturedErrorLevel(level) {
		addErrorEntry(entry)
	}
}

// isCapturedErrorLevel reports whether a zerolog level should appear on /admin/errors.
// level is the zerolog level string from a JSON log line (for example "warn" or "error").
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
		sec := int64(asNumber)
		nsec := int64((asNumber - float64(sec)) * float64(time.Second))
		return time.Unix(sec, nsec)
	}

	return time.Now()
}
