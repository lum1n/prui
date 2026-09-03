package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

func TestLineThreadNestsReplies(t *testing.T) {
	ln := domain.DiffLine{Anchor: domain.Anchor{Side: domain.SideRight, Line: 10}}
	comments := []domain.Comment{
		{ID: "1", Author: "Alice", Body: "root", Path: "a.go", Anchor: &domain.Anchor{Side: domain.SideRight, Line: 10}, Created: time.Unix(1, 0)},
		{ID: "2", Author: "Bob", Body: "reply", Path: "a.go", ParentID: "1", Created: time.Unix(2, 0)},
		{ID: "3", Author: "Carol", Body: "nested", Path: "a.go", ParentID: "2", Created: time.Unix(3, 0)},
	}
	drafts := []domain.DraftComment{
		{ID: "d1", Body: "my reply", ParentID: "2"},
	}
	nodes := lineThread(comments, drafts, "a.go", ln)
	if len(nodes) != 4 {
		t.Fatalf("got %d nodes, want 4", len(nodes))
	}
	want := []struct {
		depth int
		body  string
		draft bool
	}{
		{0, "root", false},
		{1, "reply", false},
		{2, "my reply", true},
		{2, "nested", false},
	}
	for i, w := range want {
		if nodes[i].Depth != w.depth {
			t.Fatalf("node %d depth=%d want %d", i, nodes[i].Depth, w.depth)
		}
		body := ""
		draft := nodes[i].Draft != nil
		if draft {
			body = nodes[i].Draft.Body
		} else {
			body = nodes[i].Comment.Body
		}
		if body != w.body || draft != w.draft {
			t.Fatalf("node %d body=%q draft=%v want %q/%v", i, body, draft, w.body, w.draft)
		}
	}
}

func TestBuildConversationEntriesThreads(t *testing.T) {
	comments := []domain.Comment{
		{ID: "1", Author: "Alice", Body: "general", Created: time.Unix(1, 0)},
		{ID: "2", Author: "Bob", Body: "reply", ParentID: "1", Created: time.Unix(2, 0)},
		{ID: "3", Author: "Carol", Body: "newer root", Created: time.Unix(10, 0)},
		{ID: "9", Author: "Eve", Body: "line", Path: "a.go", Anchor: &domain.Anchor{Path: "a.go", Line: 3}, Created: time.Unix(1, 0)},
	}
	drafts := []domain.DraftComment{
		{ID: "d1", Body: "draft reply", ParentID: "1"},
		{ID: "d2", Body: "top draft"},
	}
	entries := buildConversationEntries(comments, drafts)
	if len(entries) != 5 {
		t.Fatalf("got %d entries: %+v", len(entries), entries)
	}
	// Newest activity first: local top draft, then newer root, then older thread.
	if entries[0].Body != "top draft" || !entries[0].Draft {
		t.Fatalf("top draft first: %+v", entries[0])
	}
	if entries[1].Body != "newer root" || entries[1].Depth != 0 {
		t.Fatalf("newest root: %+v", entries[1])
	}
	if entries[2].Body != "general" || entries[2].Depth != 0 {
		t.Fatalf("older root: %+v", entries[2])
	}
	if entries[3].Body != "draft reply" || entries[3].Depth != 1 || !entries[3].Draft {
		t.Fatalf("draft reply: %+v", entries[3])
	}
	if entries[4].Body != "reply" || entries[4].Depth != 1 {
		t.Fatalf("reply: %+v", entries[4])
	}
}

func TestFormatConversationShowsNesting(t *testing.T) {
	entries := []convEntry{
		{ID: "1", Author: "Alice", Body: "root"},
		{ID: "2", Author: "Bob", Body: "nested", Depth: 1, Parent: "1"},
	}
	out := formatConversation(entries, 1, 60)
	for _, part := range []string{"Conversation", "Alice", "root", "Bob", "nested", "↳", ">"} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in:\n%s", part, out)
		}
	}
}

func TestPaintThreadNumbersSelected(t *testing.T) {
	nodes := []threadNode{
		{Comment: &domain.Comment{ID: "1", Author: "Alice", Body: "one"}, Depth: 0},
		{Comment: &domain.Comment{ID: "2", Author: "Bob", Body: "two"}, Depth: 1},
		{Comment: &domain.Comment{ID: "3", Author: "Carol", Body: "three"}, Depth: 1},
	}
	th := diff.ThemeFor("dark")
	out := paintThread(nodes, "2", true, th, 80)
	for _, part := range []string{"#1 Alice", "#2 Bob", "#3 Carol", "▸"} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in:\n%s", part, out)
		}
	}
	if replyableIDs(nodes)[1] != "2" {
		t.Fatal("replyable order")
	}
}
