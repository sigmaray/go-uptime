package handlers

import (
	"errors"
	"testing"
)

func TestEvaluateHealth(t *testing.T) {
	tests := []struct {
		name          string
		dbErr         error
		workerRunning bool
		wantOK        bool
		wantDatabase  string
		wantWorker    string
	}{
		{
			name:          "all healthy",
			dbErr:         nil,
			workerRunning: true,
			wantOK:        true,
			wantDatabase:  "ok",
			wantWorker:    "ok",
		},
		{
			name:          "database down",
			dbErr:         errors.New("connection refused"),
			workerRunning: true,
			wantOK:        false,
			wantDatabase:  "unavailable",
			wantWorker:    "ok",
		},
		{
			name:          "worker stopped",
			dbErr:         nil,
			workerRunning: false,
			wantOK:        false,
			wantDatabase:  "ok",
			wantWorker:    "not running",
		},
		{
			name:          "both unhealthy",
			dbErr:         errors.New("timeout"),
			workerRunning: false,
			wantOK:        false,
			wantDatabase:  "unavailable",
			wantWorker:    "not running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateHealth(tt.dbErr, tt.workerRunning)
			if got.OK != tt.wantOK {
				t.Fatalf("OK = %v, want %v", got.OK, tt.wantOK)
			}
			if got.Database != tt.wantDatabase {
				t.Fatalf("Database = %q, want %q", got.Database, tt.wantDatabase)
			}
			if got.Worker != tt.wantWorker {
				t.Fatalf("Worker = %q, want %q", got.Worker, tt.wantWorker)
			}
		})
	}
}
