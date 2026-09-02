package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

// yankLines returns plain source text for the given diff lines (no gutters,
// signs, or hunk chrome). Hunk headers and meta lines are skipped.
func yankLines(lines []domain.DiffLine) string {
	var b strings.Builder
	n := 0
	for _, ln := range lines {
		if diff.IsHunkHeader(ln) {
			continue
		}
		if strings.HasPrefix(ln.Text, `\`) {
			continue
		}
		if n > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ln.Text)
		n++
	}
	return b.String()
}

func copyClipboard(text string) error {
	if text == "" {
		return fmt.Errorf("nothing to yank")
	}
	var last error
	if err := clipboard.WriteAll(text); err != nil {
		last = err
	} else {
		last = nil
	}
	// Always emit OSC 52 so remote/tmux sessions can still receive the yank.
	if _, err := osc52.New(text).WriteTo(os.Stderr); err != nil && last != nil {
		return fmt.Errorf("clipboard: %v; osc52: %w", last, err)
	}
	if last != nil {
		// Local clipboard failed but OSC 52 may have worked.
		return nil
	}
	return nil
}
