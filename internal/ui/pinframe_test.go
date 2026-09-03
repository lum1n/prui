package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestPinFrameExactGrid(t *testing.T) {
	in := strings.Repeat(strings.Repeat("x", 50)+"\n", 20) + strings.Repeat("y", 80)
	out := pinFrame(in, 40, 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 10 {
		t.Fatalf("lines=%d want 10", len(lines))
	}
	if strings.HasSuffix(out, "\n") {
		t.Fatal("trailing newline")
	}
	for i, ln := range lines {
		if w := ansi.StringWidth(ln); w > 40 {
			t.Fatalf("line %d width %d", i, w)
		}
	}
	if lipgloss.Height(out) != 10 {
		t.Fatalf("height %d", lipgloss.Height(out))
	}
}
