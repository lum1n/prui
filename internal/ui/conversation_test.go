package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/vegard/prui/internal/domain"
)

func TestIsConversationComment(t *testing.T) {
	if !isConversationComment(domain.Comment{Body: "hi"}) {
		t.Fatal("nil anchor should be conversation")
	}
	if !isConversationComment(domain.Comment{Path: "a.go", Anchor: &domain.Anchor{Path: "a.go"}}) {
		t.Fatal("file-level should be conversation")
	}
	if isConversationComment(domain.Comment{Path: "a.go", Anchor: &domain.Anchor{Path: "a.go", Line: 12}}) {
		t.Fatal("line comment should not be conversation")
	}
}

func TestFormatConversation(t *testing.T) {
	out := formatConversation([]domain.Comment{{
		Author: "Alice (alice)", Body: "Looks good overall", Created: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}}, []domain.DraftComment{{Body: "my note"}}, 60)
	for _, part := range []string{"Conversation", "Alice", "Looks good", "draft", "my note"} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in:\n%s", part, out)
		}
	}
}
