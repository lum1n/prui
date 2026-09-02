package diff_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/vegard/prui/internal/diff"
	"github.com/vegard/prui/internal/domain"
)

func TestPaintUnifiedSmoke(t *testing.T) {
	h := diff.NewHighlighter("dark")
	th := diff.DarkTheme()
	lines := []domain.DiffLine{
		{Kind: domain.LineContext, Text: "@@ -1,2 +1,3 @@"},
		{Kind: domain.LineContext, OldNumber: 1, NewNumber: 1, Text: "package main"},
		{Kind: domain.LineRemoved, OldNumber: 2, Text: "old()", Anchor: domain.Anchor{Line: 2, Side: domain.SideLeft}},
		{Kind: domain.LineAdded, NewNumber: 2, Text: "new()", Anchor: domain.Anchor{Line: 2, Side: domain.SideRight}},
		{Kind: domain.LineAdded, NewNumber: 3, Text: "extra()", Anchor: domain.Anchor{Line: 3, Side: domain.SideRight}},
	}
	var b strings.Builder
	b.WriteString(diff.PaintFileHeader("main.go", domain.FileModified, th, 72))
	b.WriteByte('\n')
	for i, ln := range lines {
		b.WriteString(diff.Paint(h, "main.go", ln, diff.Options{Theme: th, Width: 72, Selected: i == 2}))
		b.WriteByte('\n')
	}
	out := b.String()
	if !strings.Contains(out, "main.go") {
		t.Fatalf("missing header:\n%s", out)
	}
	if !strings.Contains(out, "···") {
		t.Fatalf("missing hunk separator:\n%s", out)
	}
	if diff.Commentable(lines[0]) {
		t.Fatal("hunk should not be commentable")
	}
	if !diff.Commentable(lines[3]) {
		t.Fatal("added line should be commentable")
	}
}

func TestPaintAppliesLineBackground(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	h := diff.NewHighlighter("dark")
	th := diff.DarkTheme()
	added := diff.Paint(h, "main.go", domain.DiffLine{
		Kind: domain.LineAdded, NewNumber: 1, Text: `fmt.Println("hi")`,
	}, diff.Options{Theme: th, Width: 80})
	removed := diff.Paint(h, "main.go", domain.DiffLine{
		Kind: domain.LineRemoved, OldNumber: 1, Text: `fmt.Println("bye")`,
	}, diff.Options{Theme: th, Width: 80})
	// lipgloss emits 48;2 RGB background sequences for themed rows
	if !strings.Contains(added, "48;2") {
		t.Fatalf("added line missing background ANSI:\n%q", added)
	}
	if !strings.Contains(removed, "48;2") {
		t.Fatalf("removed line missing background ANSI:\n%q", removed)
	}
}
