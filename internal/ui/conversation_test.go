package ui

import (
	"testing"

	"github.com/lum1n/prui/internal/domain"
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
