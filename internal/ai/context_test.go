package ai_test

import (
	"strings"
	"testing"

	"github.com/vegard/prui/internal/ai"
	"github.com/vegard/prui/internal/domain"
)

func TestPackContextTruncates(t *testing.T) {
	pr := &domain.PullRequest{
		Title:  "Fix bug",
		Author: "alice",
		State:  "open",
		Body:   "Does a thing",
	}
	files := []domain.FileChange{
		{Path: "a.go", Status: domain.FileModified},
		{Path: "b.go", Status: domain.FileAdded},
	}
	big := strings.Repeat("x", 5000)
	out := ai.PackContext(ai.ContextInput{
		PR:              pr,
		Files:           files,
		Diffs:           map[string]string{"a.go": big, "b.go": big},
		MaxContextBytes: 2000,
	})
	if !strings.Contains(out, "Fix bug") || !strings.Contains(out, "a.go") {
		t.Fatalf("missing basics:\n%s", out)
	}
	if len(out) > 2500 {
		t.Fatalf("expected truncation, got len=%d", len(out))
	}
	if !strings.Contains(out, "truncated") {
		t.Fatalf("expected truncation marker:\n%s", out)
	}
}

func TestPackContextTasks(t *testing.T) {
	out := ai.PackContext(ai.ContextInput{
		PR:    &domain.PullRequest{Title: "t", State: "open"},
		Tasks: []domain.Task{{Body: "do it", Done: false}, {Body: "done", Done: true}},
	})
	for _, part := range []string{"## Tasks", "[ ] do it", "[x] done"} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in %s", part, out)
		}
	}
}

func TestCLIArgs(t *testing.T) {
	codex := ai.CodexArgs("gpt-5", "hello")
	want := []string{"exec", "--sandbox", "read-only", "--model", "gpt-5", "hello"}
	if strings.Join(codex, " ") != strings.Join(want, " ") {
		t.Fatalf("codex got %v", codex)
	}
	oc := ai.OpenCodeArgs("", "hi")
	if strings.Join(oc, " ") != "run hi" {
		t.Fatalf("opencode got %v", oc)
	}
}
