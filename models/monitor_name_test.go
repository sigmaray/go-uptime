package models

import "testing"

func TestDefaultMonitorName(t *testing.T) {
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
			got := DefaultMonitorName(tt.in)
			if got != tt.want {
				t.Fatalf("DefaultMonitorName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveMonitorName(t *testing.T) {
	if got := ResolveMonitorName("Custom", "https://example.com"); got != "Custom" {
		t.Fatalf("ResolveMonitorName with name = %q, want Custom", got)
	}
	if got := ResolveMonitorName("  ", "https://example.com"); got != "example.com" {
		t.Fatalf("ResolveMonitorName empty name = %q, want example.com", got)
	}
}
