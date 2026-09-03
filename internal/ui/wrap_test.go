package ui

import (
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestWrapWidthBreaksLongLines(t *testing.T) {
	long := strings.Repeat("abcdefghi ", 20) // ~200 cols
	out := wrapWidth(long, 40)
	if maxLineWidth(out) > 40 {
		t.Fatalf("line wider than 40: max=%d\n%q", maxLineWidth(out), out)
	}
	if !strings.Contains(out, "\n") {
		t.Fatalf("expected wrap, got single line %q", out)
	}
}

func TestWrapWidthBreaksLongToken(t *testing.T) {
	tok := strings.Repeat("x", 80)
	out := wrapWidth(tok, 30)
	if maxLineWidth(out) > 30 {
		t.Fatalf("token not broken: max=%d", maxLineWidth(out))
	}
}

func TestFormatTasksSectionWraps(t *testing.T) {
	body := strings.Repeat("long-task-word ", 30)
	tasks := []domain.Task{{ID: "1", Body: body, Done: false, Required: true}}
	out := formatTasksSection(tasks, 0, true, 48)
	if maxLineWidth(out) > 48 {
		t.Fatalf("task section overflows: max=%d\n%s", maxLineWidth(out), out)
	}
	if !strings.Contains(out, "long-task-word") {
		t.Fatalf("missing body:\n%s", out)
	}
}

func TestFormatOverviewNoOverflow(t *testing.T) {
	pr := &domain.PullRequest{
		Title:  strings.Repeat("Very Long PR Title Word ", 10),
		State:  "open",
		Author: "alice",
	}
	tasks := []domain.Task{{
		Body:     strings.Repeat("implement the feature across modules ", 15),
		Required: true,
	}}
	out := formatOverview(pr, tasks, 0, nil, 0, sectionTasks, "desc", "", "", false, "short", 50)
	if maxLineWidth(out) > 50 {
		t.Fatalf("overview overflows: max=%d\n%s", maxLineWidth(out), out)
	}
}
