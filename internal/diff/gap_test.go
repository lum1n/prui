package diff_test

import (
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

func TestExpandCollapseGap(t *testing.T) {
	patch := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,2 +1,2 @@
 a
-b
+B
@@ -5,1 +5,1 @@
-e
+E
`
	fd, err := diff.ParseUnified("foo.go", patch)
	if err != nil {
		t.Fatal(err)
	}
	var hdrIdx int
	var hdr domain.DiffLine
	for i, ln := range fd.Lines {
		if diff.ExpandableGap(ln) {
			hdrIdx, hdr = i, ln
			break
		}
	}
	if hdr.GapBefore != 2 || hdr.GapFrom != 3 || hdr.GapTo != 5 {
		t.Fatalf("gap %+v", hdr)
	}
	file := "a\nB\nc\nd\nE\n"
	cur, saved, err := diff.ExpandGap(fd, hdrIdx, file)
	if err != nil {
		t.Fatal(err)
	}
	if cur != hdrIdx {
		t.Fatalf("cursor %d", cur)
	}
	if !fd.Lines[hdrIdx].Expanded || fd.Lines[hdrIdx].Text != "c" {
		t.Fatalf("first expanded %+v", fd.Lines[hdrIdx])
	}
	if !fd.Lines[hdrIdx+1].Expanded || fd.Lines[hdrIdx+1].Text != "d" {
		t.Fatalf("second expanded %+v", fd.Lines[hdrIdx+1])
	}
	for _, ln := range fd.Lines {
		if diff.IsHunkHeader(ln) && ln.GapBefore > 0 {
			t.Fatal("gap header still present")
		}
	}
	cur, err = diff.CollapseGap(fd, hdrIdx, saved)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.ExpandableGap(fd.Lines[cur]) {
		t.Fatalf("expected restored gap, got %+v", fd.Lines[cur])
	}
	if !strings.Contains(diff.HunkGapLabel(fd.Lines[cur]), "2 unchanged") {
		t.Fatalf("label %q", diff.HunkGapLabel(fd.Lines[cur]))
	}
}
