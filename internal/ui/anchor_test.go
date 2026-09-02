package ui

import (
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestAnchorHint(t *testing.T) {
	if got := anchorHint(nil); got != "" {
		t.Fatalf("%q", got)
	}
	if got := anchorHint(&domain.Anchor{Line: 12}); got != " line 12" {
		t.Fatalf("%q", got)
	}
	if got := anchorHint(&domain.Anchor{Line: 10, EndLine: 18}); got != " lines 10–18" {
		t.Fatalf("%q", got)
	}
}

func TestReplyTargetLabel(t *testing.T) {
	m := Model{
		comments: []domain.Comment{{
			ID: "42", Author: "Bob (bob)", Body: "Please rename this helper.",
		}},
		replyParentID: "42",
		fileDiff: &domain.FileDiff{Lines: []domain.DiffLine{{
			Anchor: domain.Anchor{Line: 15, Side: domain.SideRight},
		}}},
		cursorLine: 0,
	}
	got := m.replyTargetLabel(50)
	for _, part := range []string{"Bob", "rename", "reply"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q in %q", part, got)
		}
	}
}
