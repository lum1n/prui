package ui

import (
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestYankLinesSkipsChrome(t *testing.T) {
	lines := []domain.DiffLine{
		{Text: "@@ -1,3 +1,4 @@", Kind: domain.LineContext},
		{Text: "package main", Kind: domain.LineContext},
		{Text: "func hello() {", Kind: domain.LineAdded},
		{Text: `\ No newline at end of file`, Kind: domain.LineContext},
		{Text: "}", Kind: domain.LineAdded},
	}
	got := yankLines(lines)
	want := "package main\nfunc hello() {\n}"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if strings.Contains(got, "@@") || strings.Contains(got, "+") {
		t.Fatalf("chrome leaked: %q", got)
	}
}
