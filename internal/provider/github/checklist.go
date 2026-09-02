package github

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/lum1n/prui/internal/domain"
)

var checklistRE = regexp.MustCompile(`(?m)^(\s*(?:[-*+]|\d+\.)\s+)\[([ xX])\](\s+.*)$`)

// parseChecklistTasks extracts markdown checkbox items from a PR body.
// IDs are checklist-<n> in match order.
func parseChecklistTasks(body string) []domain.Task {
	matches := checklistRE.FindAllStringSubmatch(body, -1)
	out := make([]domain.Task, 0, len(matches))
	for i, m := range matches {
		done := m[2] == "x" || m[2] == "X"
		text := strings.TrimSpace(m[3])
		out = append(out, domain.Task{
			ID:       fmt.Sprintf("checklist-%d", i),
			Body:     text,
			Done:     done,
			Required: true,
		})
	}
	return out
}

// setChecklistDone flips the Nth checkbox in body (ID checklist-N).
func setChecklistDone(body, taskID string, done bool) (string, error) {
	if !strings.HasPrefix(taskID, "checklist-") {
		return body, fmt.Errorf("unknown task id %q", taskID)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(taskID, "checklist-"))
	if err != nil || n < 0 {
		return body, fmt.Errorf("unknown task id %q", taskID)
	}
	mark := " "
	if done {
		mark = "x"
	}
	idx := 0
	out := checklistRE.ReplaceAllStringFunc(body, func(line string) string {
		cur := idx
		idx++
		if cur != n {
			return line
		}
		m := checklistRE.FindStringSubmatch(line)
		if m == nil {
			return line
		}
		return m[1] + "[" + mark + "]" + m[3]
	})
	if n >= idx {
		return body, fmt.Errorf("task %s not found", taskID)
	}
	return out, nil
}

func anyOpenRequired(tasks []domain.Task) bool {
	for _, t := range tasks {
		if t.Required && !t.Done {
			return true
		}
	}
	return false
}
