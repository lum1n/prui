package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// wrapWidth hard-wraps s to at most width columns (ANSI-aware).
// Long tokens without spaces are still broken so nothing overflows horizontally.
func wrapWidth(s string, width int) string {
	if width < 1 {
		return s
	}
	s = strings.ReplaceAll(s, "\t", "    ")
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(s)
}

// maxLineWidth returns the widest visual line in s (ANSI-aware).
func maxLineWidth(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if w := ansi.StringWidth(line); w > max {
			max = w
		}
	}
	return max
}
