package diff_test

import (
	"testing"

	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

func TestParseUnified(t *testing.T) {
	patch := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 package foo
+import "fmt"
 
-func old() {}
+func new() {}
`
	fd, err := diff.ParseUnified("foo.go", patch)
	if err != nil {
		t.Fatal(err)
	}
	if fd.Path != "foo.go" {
		t.Fatalf("path %q", fd.Path)
	}
	var added, removed int
	for _, ln := range fd.Lines {
		switch ln.Kind {
		case domain.LineAdded:
			added++
			if ln.Anchor.Side != domain.SideRight || ln.NewNumber == 0 {
				t.Fatalf("bad added anchor: %+v", ln.Anchor)
			}
		case domain.LineRemoved:
			removed++
			if ln.Anchor.Side != domain.SideLeft || ln.OldNumber == 0 {
				t.Fatalf("bad removed anchor: %+v", ln.Anchor)
			}
		}
	}
	if added < 2 || removed < 1 {
		t.Fatalf("added=%d removed=%d lines=%d", added, removed, len(fd.Lines))
	}
}

func TestParseUnifiedHunkGaps(t *testing.T) {
	patch := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,3 @@
 line1
-line2
+line2b
 line3
@@ -53,2 +52,2 @@
 line52
-line53
+line53b
`
	fd, err := diff.ParseUnified("foo.go", patch)
	if err != nil {
		t.Fatal(err)
	}
	var headers []domain.DiffLine
	for _, ln := range fd.Lines {
		if diff.IsHunkHeader(ln) {
			headers = append(headers, ln)
		}
	}
	if len(headers) != 2 {
		t.Fatalf("headers=%d", len(headers))
	}
	if headers[0].GapBefore != 0 {
		t.Fatalf("first gap=%d want 0", headers[0].GapBefore)
	}
	// First hunk ends at new line 3; second starts at 52 → omitted 4..51 = 48 lines.
	if headers[1].GapBefore != 48 || headers[1].GapFrom != 4 || headers[1].GapTo != 52 {
		t.Fatalf("second gap before=%d from=%d to=%d want 48,4,52",
			headers[1].GapBefore, headers[1].GapFrom, headers[1].GapTo)
	}
	if headers[1].NewNumber != 52 || headers[1].OldNumber != 53 {
		t.Fatalf("second starts old=%d new=%d", headers[1].OldNumber, headers[1].NewNumber)
	}
}

func TestLanguageFromPath(t *testing.T) {
	if diff.LanguageFromPath("x.go") != "go" {
		t.Fatal(diff.LanguageFromPath("x.go"))
	}
	if diff.LanguageFromPath("a.tsx") != "tsx" {
		t.Fatal(diff.LanguageFromPath("a.tsx"))
	}
}
