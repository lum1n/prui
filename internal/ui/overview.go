package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vegard/prui/internal/domain"
)

type overviewSection int

const (
	sectionTasks overviewSection = iota
	sectionDescription
	sectionConversation
)

func (s overviewSection) next() overviewSection {
	return overviewSection((int(s) + 1) % 3)
}

func (s overviewSection) prev() overviewSection {
	return overviewSection((int(s) + 2) % 3)
}

func formatPRStatus(pr *domain.PullRequest, tasks []domain.Task) string {
	if pr == nil {
		return mutedStyle.Render("no pull request")
	}
	parts := make([]string, 0, 4)
	state := strings.ToLower(pr.State)
	if state == "" {
		state = "unknown"
	}
	parts = append(parts, state)
	if pr.Draft {
		parts = append(parts, "draft")
	}
	blocked := pr.Blocked
	openRequired := 0
	for _, t := range tasks {
		if t.Required && !t.Done {
			openRequired++
			blocked = true
		}
	}
	if blocked {
		parts = append(parts, "blocked")
	}
	if openRequired > 0 {
		parts = append(parts, fmt.Sprintf("%d open task(s)", openRequired))
	}
	line := strings.Join(parts, " · ")
	st := lipgloss.NewStyle().Bold(true)
	if blocked {
		st = st.Foreground(lipgloss.Color("#f07178"))
	} else if pr.Draft {
		st = st.Foreground(lipgloss.Color("#e6c07b"))
	} else {
		st = st.Foreground(lipgloss.Color("#98c379"))
	}
	return st.Render(line)
}

func formatTasksSection(tasks []domain.Task, cursor int, active bool, width int) string {
	var b strings.Builder
	b.WriteString(sectionHeader("Tasks", active))
	b.WriteByte('\n')
	if len(tasks) == 0 {
		b.WriteString(mutedStyle.Render("  No tasks on this PR."))
		b.WriteByte('\n')
		return b.String()
	}
	for i, t := range tasks {
		mark := "[ ]"
		if t.Done {
			mark = "[x]"
		}
		prefix := "  "
		if active && i == cursor {
			prefix = "> "
		}
		req := ""
		if t.Required && !t.Done {
			req = mutedStyle.Render(" · required")
		}
		author := ""
		if t.Author != "" {
			author = mutedStyle.Render(" · " + t.Author)
		}
		line := fmt.Sprintf("%s%s %s%s%s", prefix, mark, strings.TrimSpace(t.Body), author, req)
		if width > 4 {
			line = truncate(line, width)
		}
		if active && i == cursor {
			b.WriteString(draftStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func sectionHeader(title string, active bool) string {
	label := title
	if active {
		return titleStyle.Render("▸ " + label)
	}
	return mutedStyle.Render("  " + label)
}

func formatConversationSection(entries []convEntry, cursor int, active bool, width int) string {
	var b strings.Builder
	b.WriteString(sectionHeader("Conversation", active))
	b.WriteByte('\n')
	if !active {
		b.WriteString(mutedStyle.Render("  tab here · j/k select · R reply · c new"))
		b.WriteByte('\n')
	} else {
		b.WriteString(mutedStyle.Render("  j/k select · R reply · c new · space N/A"))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	if len(entries) == 0 {
		b.WriteString(mutedStyle.Render("  No general comments yet. Press c to add a draft."))
		b.WriteByte('\n')
		return b.String()
	}
	for i, e := range entries {
		sel := active && i == cursor
		b.WriteString(formatConversationEntry(e, sel, width))
		b.WriteByte('\n')
	}
	return b.String()
}

func formatOverview(
	pr *domain.PullRequest,
	tasks []domain.Task,
	taskCursor int,
	entries []convEntry,
	convCursor int,
	sec overviewSection,
	description string,
	width int,
) string {
	if width < 40 {
		width = 40
	}
	var b strings.Builder
	title := "Overview"
	if pr != nil && pr.Title != "" {
		title = pr.Title
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(formatPRStatus(pr, tasks))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("tab section · esc back"))
	b.WriteString("\n\n")

	b.WriteString(formatTasksSection(tasks, taskCursor, sec == sectionTasks, width))
	b.WriteByte('\n')

	b.WriteString(sectionHeader("Description", sec == sectionDescription))
	b.WriteByte('\n')
	if strings.TrimSpace(description) == "" {
		b.WriteString(mutedStyle.Render("  _No description_"))
		b.WriteByte('\n')
	} else {
		b.WriteString(description)
		if !strings.HasSuffix(description, "\n") {
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')

	b.WriteString(formatConversationSection(entries, convCursor, sec == sectionConversation, width))
	return b.String()
}

func openTaskCount(tasks []domain.Task) int {
	n := 0
	for _, t := range tasks {
		if !t.Done {
			n++
		}
	}
	return n
}
