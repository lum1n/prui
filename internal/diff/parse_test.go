package diff_test

import (
	"testing"

	"github.com/vegard/prui/internal/diff"
	"github.com/vegard/prui/internal/domain"
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

func TestLanguageFromPath(t *testing.T) {
	if diff.LanguageFromPath("x.go") != "go" {
		t.Fatal(diff.LanguageFromPath("x.go"))
	}
	if diff.LanguageFromPath("a.tsx") != "tsx" {
		t.Fatal(diff.LanguageFromPath("a.tsx"))
	}
}
