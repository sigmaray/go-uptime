package forms

import (
	"strings"
	"testing"
)

func TestParseURLList(t *testing.T) {
	// Табличный тест ParseURLList: split по comma/newline, trim, dedupe, CRLF.
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "empty",
			in:   "",
			want: nil,
		},
		{
			name: "whitespace only",
			in:   "  \n  ,  ",
			want: nil,
		},
		{
			name: "comma separated",
			in:   "https://a.example.com, https://b.example.com",
			want: []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name: "newline separated",
			in:   "https://a.example.com\nhttps://b.example.com",
			want: []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name: "mixed commas and newlines",
			in:   "https://a.example.com, https://b.example.com\nhttps://c.example.com",
			want: []string{"https://a.example.com", "https://b.example.com", "https://c.example.com"},
		},
		{
			name: "trims whitespace and skips empties",
			in:   "  https://a.example.com  ,\n\n  https://b.example.com\n",
			want: []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name: "dedupes preserving first order",
			in:   "https://a.example.com, https://b.example.com, https://a.example.com",
			want: []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name: "crlf newlines",
			in:   "https://a.example.com\r\nhttps://b.example.com",
			want: []string{"https://a.example.com", "https://b.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act.
			got := ParseURLList(tt.in)
			// Assert: nil и пустой slice эквивалентны для «нет URL».
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseURLList() len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("ParseURLList()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMonitorURLBulkInputValidate(t *testing.T) {
	t.Run("requires at least one url", func(t *testing.T) {
		// Arrange: только пробелы и разделители — после ParseURLList nil.
		input := MonitorURLBulkInput{URLs: "  , \n "}
		// Assert.
		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("rejects invalid url", func(t *testing.T) {
		// Arrange: один валидный URL, один мусор.
		input := MonitorURLBulkInput{URLs: "https://ok.example.com\nnot-a-url"}
		// Act.
		err := input.Validate()
		// Assert: error и упоминание invalid token в сообщении.
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), "not-a-url") {
			t.Fatalf("error %q should mention the invalid URL", err.Error())
		}
	})

	t.Run("rejects non http scheme", func(t *testing.T) {
		// Arrange: ftp:// не поддерживается мониторингом HTTP(S).
		input := MonitorURLBulkInput{URLs: "ftp://files.example.com"}
		// Assert.
		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error for ftp URL")
		}
	})

	t.Run("accepts valid urls and interval", func(t *testing.T) {
		// Arrange: два https URL и интервал 30s (в допустимых bounds).
		input := MonitorURLBulkInput{
			URLs:                 "https://a.example.com, https://b.example.com",
			CheckIntervalSeconds: "30",
		}
		// Assert: happy path без ошибок.
		if err := input.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects bad interval", func(t *testing.T) {
		// Arrange: URL валиден, интервал 5s — ниже минимума 10.
		input := MonitorURLBulkInput{
			URLs:                 "https://a.example.com",
			CheckIntervalSeconds: "5",
		}
		// Assert.
		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error for interval")
		}
	})
}
