package diff_test

import (
	"strings"
	"testing"

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
