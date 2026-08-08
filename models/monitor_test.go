package models

import (
	"testing"
)

func TestMonitorCheckIntervalSeconds(t *testing.T) {
	// Arrange: глобальный интервал по умолчанию и кастомное значение монитора.
	global := 60
	custom := 300

	tests := []struct {
		name    string
		monitor MonitorURL
		want    int
	}{
		{
			name:    "uses global when unset",
			monitor: MonitorURL{},
			want:    60,
		},
		{
			name:    "uses monitor override",
			monitor: MonitorURL{CheckIntervalSeconds: &custom},
			want:    300,
		},
		{
			name:    "ignores invalid override",
			monitor: MonitorURL{CheckIntervalSeconds: intPtr(5)},
			want:    60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act: выбираем эффективный интервал проверки.
			if got := MonitorCheckIntervalSeconds(tt.monitor, global); got != tt.want {
				// Assert: интервал должен соответствовать правилам override/validation.
				t.Fatalf("MonitorCheckIntervalSeconds() = %d, want %d", got, tt.want)
			}
		})
	}
}

// intPtr возвращает указатель на int — удобный helper для optional-полей в table-driven тестах.
func intPtr(v int) *int {
	return &v
}
