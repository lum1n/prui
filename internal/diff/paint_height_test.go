package diff_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

func TestPaintAlwaysSingleLine(t *testing.T) {
	th := diff.ThemeFor("dark")
	h := diff.NewHighlighter("dark")
	cases := []domain.DiffLine{
		{Kind: domain.LineContext, Text: "\tfmt.Println(\"hello\")", OldNumber: 1, NewNumber: 1},
		{Kind: domain.LineAdded, Text: strings.Repeat("x", 200), NewNumber: 2},
		{Kind: domain.LineRemoved, Text: "func main() {", OldNumber: 3},
		{Kind: domain.LineContext, Text: "line with\nembedded", OldNumber: 4, NewNumber: 4},
		{Kind: domain.LineContext, Text: "trailing\r", OldNumber: 5, NewNumber: 5},
		{Kind: domain.LineAdded, Text: "\t\t\tvery\twide\ttabs\tand\tmore\tstuff\there", NewNumber: 6},
		{Kind: domain.LineContext, Text: "@@ -1,2 +1,3 @@"},
		{Kind: domain.LineAdded, Text: "emoji 😀 and wide 中文 chars " + strings.Repeat("y", 80), NewNumber: 8},
	}
	for _, w := range []int{30, 40, 50, 60, 72, 80, 100, 120} {
		for j, ln := range cases {
			for _, sel := range []bool{false, true} {
				for _, split := range []bool{false, true} {
					out := diff.Paint(h, "main.go", ln, diff.Options{Theme: th, Width: w, Selected: sel, Split: split})
					if h := lipgloss.Height(out); h != 1 {
						t.Fatalf("W=%d case=%d sel=%v split=%v height=%d\n%q", w, j, sel, split, h, out)
					}
					if strings.Contains(out, "\n") {
						t.Fatalf("W=%d case=%d contains newline:\n%q", w, j, out)
					}
				}
			}
		}
	}
}
