package ui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestFitPathKeepBase(t *testing.T) {
	path := "internal/provider/bitbucketdc/bitbucketdc.go"
	got := fitPathKeepBase(path, 28)
	if !strings.HasSuffix(got, "bitbucketdc.go") {
		t.Fatalf("basename lost: %q", got)
	}
	if !strings.HasPrefix(got, "…") {
		t.Fatalf("expected ellipsis: %q", got)
	}
	if runewidth.StringWidth(got) > 28 {
		t.Fatalf("too wide: %q (%d)", got, runewidth.StringWidth(got))
	}

	if fitPathKeepBase("short.go", 40) != "short.go" {
		t.Fatal("short path should be unchanged")
	}

	got = fitPathKeepBase("verylongfilenamewithoutslashes.go", 12)
	if runewidth.StringWidth(got) > 12 {
		t.Fatalf("long base overflow: %q (%d)", got, runewidth.StringWidth(got))
	}
	if !strings.HasSuffix(got, ".go") {
		t.Fatalf("should keep file end: %q", got)
	}
}
