package ui

import (
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestFormatPRStatus(t *testing.T) {
	pr := &domain.PullRequest{State: "open", Draft: true}
	tasks := []domain.Task{{Body: "a", Required: true, Done: false}}
	out := formatPRStatus(pr, tasks, 80)
	for _, part := range []string{"open", "draft", "blocked"} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in %q", part, out)
		}
	}
}

func TestFormatTasksSection(t *testing.T) {
	tasks := []domain.Task{
		{ID: "1", Body: "fix it", Done: false, Required: true},
		{ID: "2", Body: "ship it", Done: true},
	}
	out := formatTasksSection(tasks, 0, true, 60)
	for _, part := range []string{"Tasks", "[ ]", "fix it", "[x]", "ship it", ">"} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in:\n%s", part, out)
		}
	}
}

func TestFormatOverview(t *testing.T) {
	pr := &domain.PullRequest{Title: "Add feature", State: "open"}
	out := formatOverview(pr, nil, 0, nil, 0, sectionDescription, "hello body", "", "", false, "medium", 60)
	for _, part := range []string{"Add feature", "open", "Reviews", "Tasks", "Description", "Summary", "Conversation", "hello body"} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in:\n%s", part, out)
		}
	}
}
