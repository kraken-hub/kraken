package util

import (
	"testing"

	"gopkg.in/gookit/color.v1"
)

func withColorDisabled(t *testing.T) {
	t.Helper()
	prev := color.Enable
	color.Enable = false
	t.Cleanup(func() { color.Enable = prev })
}

func TestBuildSeparator(t *testing.T) {
	tests := []struct {
		name   string
		widths []int
		want   string
	}{
		{"single column", []int{3}, "+-----+\n"},
		{"multiple columns", []int{2, 4}, "+----+------+\n"},
		{"zero width column", []int{0}, "+--+\n"},
		{"no columns", []int{}, "++\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildSeparator(tt.widths); got != tt.want {
				t.Errorf("BuildSeparator(%v) = %q, want %q", tt.widths, got, tt.want)
			}
		})
	}
}

func TestBuildRow(t *testing.T) {
	tests := []struct {
		name   string
		cols   []string
		widths []int
		want   string
	}{
		{"single column padded", []string{"a"}, []int{3}, "| a   |\n"},
		{"multiple columns", []string{"ab", "c"}, []int{2, 3}, "| ab | c   |\n"},
		{"content exceeds width", []string{"toolong"}, []int{3}, "| toolong |\n"},
		{"empty column", []string{""}, []int{2}, "|    |\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildRow(tt.cols, tt.widths); got != tt.want {
				t.Errorf("BuildRow(%v,%v) = %q, want %q", tt.cols, tt.widths, got, tt.want)
			}
		})
	}
}

func TestBuildHeaderRow(t *testing.T) {
	withColorDisabled(t)
	tests := []struct {
		name   string
		cols   []string
		widths []int
		want   string
	}{
		{"single column padded", []string{"NAME"}, []int{6}, "| NAME   |\n"},
		{"multiple columns", []string{"A", "BB"}, []int{2, 3}, "| A  | BB  |\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildHeaderRow(tt.cols, tt.widths); got != tt.want {
				t.Errorf("BuildHeaderRow(%v,%v) = %q, want %q", tt.cols, tt.widths, got, tt.want)
			}
		})
	}
}
