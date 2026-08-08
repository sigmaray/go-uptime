package applog

import (
	"strings"
	"testing"
	"time"
)

func TestCaptureWriterParsesZerologLine(t *testing.T) {
	resetForTest()

	writer := NewCaptureWriter()
	line := `{"level":"info","time":1700000000,"message":"starting server","port":"8080"}` + "\n"
	if _, err := writer.Write([]byte(line)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	entries := RecentLogs()
	if len(entries) != 1 {
		t.Fatalf("RecentLogs() len = %d, want 1", len(entries))
	}

	entry := entries[0]
	if entry.Level != "info" {
		t.Fatalf("level = %q, want info", entry.Level)
	}
	if entry.Message != "starting server" {
		t.Fatalf("message = %q, want starting server", entry.Message)
	}
	if !strings.Contains(string(entry.Fields), `"port":"8080"`) {
		t.Fatalf("fields = %s, want port field", entry.Fields)
	}
	if entry.Time.Unix() != 1700000000 {
		t.Fatalf("time = %v, want unix 1700000000", entry.Time)
	}
}

func TestCaptureWriterBuffersPartialLines(t *testing.T) {
	resetForTest()

	writer := NewCaptureWriter()
	if _, err := writer.Write([]byte(`{"level":"warn","time":`)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if len(RecentLogs()) != 0 {
		t.Fatalf("expected no entries before newline")
	}

	if _, err := writer.Write([]byte(`1700000001,"message":"partial"}` + "\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	entries := RecentLogs()
	if len(entries) != 1 {
		t.Fatalf("RecentLogs() len = %d, want 1", len(entries))
	}
	if entries[0].Level != "warn" {
		t.Fatalf("level = %q, want warn", entries[0].Level)
	}

	errors := RecentErrors()
	if len(errors) != 1 {
		t.Fatalf("RecentErrors() len = %d, want 1 for warn", len(errors))
	}
}

func TestCaptureWriterMirrorsWarnAndErrorIntoRecentErrors(t *testing.T) {
	resetForTest()

	writer := NewCaptureWriter()
	line := `{"level":"error","time":1700000002,"message":"failed to send monitor notification","error":"smtp dial timeout","monitor_id":42}` + "\n"
	if _, err := writer.Write([]byte(line)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := writer.Write([]byte(`{"level":"info","time":1700000003,"message":"ok"}` + "\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	errors := RecentErrors()
	if len(errors) != 1 {
		t.Fatalf("RecentErrors() len = %d, want 1", len(errors))
	}
	entry := errors[0]
	if entry.Message != "failed to send monitor notification" {
		t.Fatalf("message = %q", entry.Message)
	}
	if entry.Level != "error" {
		t.Fatalf("level = %q, want error", entry.Level)
	}
	if !strings.Contains(string(entry.Fields), `"error":"smtp dial timeout"`) {
		t.Fatalf("fields = %s, want error detail", entry.Fields)
	}
	if !strings.Contains(string(entry.Fields), `"monitor_id":42`) {
		t.Fatalf("fields = %s, want monitor_id", entry.Fields)
	}
}

func TestCaptureWriterUsesCurrentTimeWhenMissing(t *testing.T) {
	resetForTest()

	before := time.Now()
	writer := NewCaptureWriter()
	line := `{"level":"error","message":"boom"}` + "\n"
	if _, err := writer.Write([]byte(line)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	after := time.Now()

	entries := RecentLogs()
	if len(entries) != 1 {
		t.Fatalf("RecentLogs() len = %d, want 1", len(entries))
	}
	if entries[0].Time.Before(before) || entries[0].Time.After(after) {
		t.Fatalf("time %v not between %v and %v", entries[0].Time, before, after)
	}
}
