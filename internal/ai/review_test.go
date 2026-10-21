package ai_test

import (
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/ai"
	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

const reviewPatch = `@@ -1,3 +1,3 @@
 package a
-var x = 1
+var x = 2
 func f() {}
`

func TestParseReviewFencedAndVerdict(t *testing.T) {
	reply := "Here you go:\n```json\n" + `{"summary":" ok ","verdict":"request changes","comments":[{"path":"a.go","line":2,"side":"RIGHT","severity":"Issue","body":"why 2?"},{"path":"a.go","line":1,"body":"  "}]}` + "\n```"
	r, err := ai.ParseReview(reply)
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary != "ok" || r.Verdict != domain.ActionRequestChanges {
		t.Fatalf("got %+v", r)
	}
	if len(r.Comments) != 1 || r.Comments[0].Body != "why 2?" {
		t.Fatalf("comments %+v", r.Comments)
	}
}

func TestParseReviewErrors(t *testing.T) {
	if _, err := ai.ParseReview("no json here"); err == nil {
		t.Fatal("expected error for missing JSON")
	}
	if _, err := ai.ParseReview(`{"summary": }`); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseVerdict(t *testing.T) {
	cases := map[string]domain.ReviewAction{
		"APPROVE":         domain.ActionApprove,
		"lgtm":            domain.ActionApprove,
		"REQUEST_CHANGES": domain.ActionRequestChanges,
		"needs-work":      domain.ActionRequestChanges,
		"":                domain.ActionComment,
		"whatever":        domain.ActionComment,
	}
	for in, want := range cases {
		if got := ai.ParseVerdict(in); got != want {
			t.Errorf("ParseVerdict(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestAnnotateDiff(t *testing.T) {
	out := ai.AnnotateDiff("a.go", reviewPatch)
	for _, want := range []string{"R1  package a", "L2 -var x = 1", "R2 +var x = 2", "R3  func f() {}"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestResolveSuggestions(t *testing.T) {
	fd, err := diff.ParseUnified("a.go", reviewPatch)
	if err != nil {
		t.Fatal(err)
	}
	diffs := map[string]*domain.FileDiff{"a.go": fd}
	got := ai.ResolveSuggestions([]ai.ReviewComment{
		{Path: "a.go", Line: 2, Side: "RIGHT", Body: "added"},
		{Path: "a.go", Line: 2, Side: "LEFT", Body: "removed"},
		{Path: "a.go", Line: 99, Side: "RIGHT", Body: "outside"},
		{Path: "nope.go", Line: 1, Body: "unknown"},
		{Body: "general"},
	}, diffs)
	if len(got) != 5 {
		t.Fatalf("got %d", len(got))
	}
	if a := got[0].Anchor; a == nil || a.Side != domain.SideRight || a.Line != 2 || got[0].Path != "a.go" {
		t.Fatalf("right anchor %+v", got[0])
	}
	if a := got[1].Anchor; a == nil || a.Side != domain.SideLeft || a.Line != 2 {
		t.Fatalf("left anchor %+v", got[1])
	}
	if got[2].Anchor != nil || !strings.HasPrefix(got[2].Body, "`a.go:99`") {
		t.Fatalf("outside %+v", got[2])
	}
	if got[3].Anchor != nil || !strings.HasPrefix(got[3].Body, "`nope.go:1`") {
		t.Fatalf("unknown %+v", got[3])
	}
	if got[4].Anchor != nil || got[4].Body != "general" || got[4].Status != domain.SuggestionPending {
		t.Fatalf("general %+v", got[4])
	}
}

func TestPackReviewContextAnnotates(t *testing.T) {
	out := ai.PackReviewContext(ai.ContextInput{
		PR:    &domain.PullRequest{Title: "T"},
		Files: []domain.FileChange{{Path: "a.go", Status: domain.FileModified, Patch: reviewPatch}},
	})
	if !strings.Contains(out, "R2 +var x = 2") {
		t.Fatalf("not annotated:\n%s", out)
	}
}
