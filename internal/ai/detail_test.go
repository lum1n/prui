package ai_test

import (
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/ai"
)

func TestParseDetailLevel(t *testing.T) {
	cases := map[string]ai.DetailLevel{
		"": "medium", "MEDIUM": "medium", "brief": "short",
		"short": "short", "full": "full", "long": "full",
	}
	for in, want := range cases {
		if got := ai.ParseDetailLevel(in); got != want {
			t.Fatalf("ParseDetailLevel(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNextDetail(t *testing.T) {
	if ai.NextDetail(ai.DetailShort) != ai.DetailMedium {
		t.Fatal("short→medium")
	}
	if ai.NextDetail(ai.DetailMedium) != ai.DetailFull {
		t.Fatal("medium→full")
	}
	if ai.NextDetail(ai.DetailFull) != ai.DetailShort {
		t.Fatal("full→short")
	}
}

func TestSystemPromptFor(t *testing.T) {
	short := ai.SystemPromptFor(ai.DetailShort)
	med := ai.SystemPromptFor(ai.DetailMedium)
	full := ai.SystemPromptFor(ai.DetailFull)
	for _, part := range []string{"SHORT", "one short paragraph", "No headings"} {
		if !strings.Contains(short, part) {
			t.Fatalf("short missing %q", part)
		}
	}
	for _, part := range []string{"MEDIUM", "three short paragraphs"} {
		if !strings.Contains(med, part) {
			t.Fatalf("medium missing %q", part)
		}
	}
	for _, part := range []string{"FULL", "No length restriction"} {
		if !strings.Contains(full, part) {
			t.Fatalf("full missing %q", part)
		}
	}
	if short == med || med == full {
		t.Fatal("prompts should differ by level")
	}
}
