package models

import "testing"

func TestDefaultMonitorName(t *testing.T) {
	// Arrange: URL разных форматов и ожидаемое имя по умолчанию.
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "https host",
			in:   "https://example.com/path",
			want: "example.com",
		},
		{
			name: "http with port",
			in:   "http://foo.bar:8080/",
			want: "foo.bar:8080",
		},
		{
			name: "trim spaces",
			in:   "  https://example.org  ",
			want: "example.org",
		},
		{
			name: "invalid url",
			in:   "not-a-url",
			want: "not-a-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act: извлекаем display name из URL.
			got := DefaultMonitorName(tt.in)
			// Assert: имя совпадает с ожиданием для данного формата.
			if got != tt.want {
				t.Fatalf("DefaultMonitorName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveMonitorName(t *testing.T) {
	// Act + Assert: непустое пользовательское имя имеет приоритет.
	if got := ResolveMonitorName("Custom", "https://example.com"); got != "Custom" {
		t.Fatalf("ResolveMonitorName with name = %q, want Custom", got)
	}
	// Act + Assert: пробелы считаются пустым именем — fallback на host из URL.
	if got := ResolveMonitorName("  ", "https://example.com"); got != "example.com" {
		t.Fatalf("ResolveMonitorName empty name = %q, want example.com", got)
	}
}
