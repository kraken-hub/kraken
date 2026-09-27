package util

import (
	"fmt"
	"strings"

	"gopkg.in/gookit/color.v1"
)

// BuildSeparator returns a table separator row for the given column widths.
func BuildSeparator(widths []int) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		parts[i] = strings.Repeat("-", w+2)
	}
	return "+" + strings.Join(parts, "+") + "+\n"
}

// BuildRow returns a table data row for the given columns and widths.
func BuildRow(cols []string, widths []int) string {
	parts := make([]string, len(cols))
	for i, col := range cols {
		parts[i] = fmt.Sprintf(" %-*s ", widths[i], col)
	}
	return "|" + strings.Join(parts, "|") + "|\n"
}

// BuildHeaderRow returns a table header row with cyan-coloured cells.
func BuildHeaderRow(cols []string, widths []int) string {
	parts := make([]string, len(cols))
	for i, col := range cols {
		parts[i] = color.Cyan.Sprint(fmt.Sprintf(" %-*s ", widths[i], col))
	}
	return "|" + strings.Join(parts, "|") + "|\n"
}
