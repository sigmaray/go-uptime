package models

import "testing"

func TestTotalPages(t *testing.T) {
	// Arrange: total, perPage и ожидаемое число страниц (минимум 1).
	tests := []struct {
		name    string
		total   int64
		perPage int
		want    int
	}{
		{name: "zero total", total: 0, perPage: 20, want: 1},
		{name: "exact page", total: 20, perPage: 20, want: 1},
		{name: "needs second page", total: 21, perPage: 20, want: 2},
		{name: "full second page", total: 40, perPage: 20, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act + Assert: TotalPages округляет вверх и не даёт 0 страниц.
			if got := TotalPages(tt.total, tt.perPage); got != tt.want {
				t.Fatalf("TotalPages() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClampPage(t *testing.T) {
	// Arrange: номер страницы, total и perPage для clamp в допустимый диапазон.
	tests := []struct {
		name    string
		page    int
		total   int64
		perPage int
		want    int
	}{
		{name: "below minimum", page: 0, total: 25, perPage: 20, want: 1},
		{name: "above maximum", page: 99, total: 25, perPage: 20, want: 2},
		{name: "valid page", page: 2, total: 25, perPage: 20, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act + Assert: ClampPage ограничивает page ∈ [1, TotalPages].
			if got := ClampPage(tt.page, tt.total, tt.perPage); got != tt.want {
				t.Fatalf("ClampPage() = %d, want %d", got, tt.want)
			}
		})
	}
}
