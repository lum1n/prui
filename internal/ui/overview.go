package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lum1n/prui/internal/domain"
)

type overviewSection int

const (
	sectionTasks overviewSection = iota
	sectionDescription
	sectionSummary
	sectionConversation
	overviewSectionCount
)

func (s overviewSection) next() overviewSection {
	return overviewSection((int(s) + 1) % int(overviewSectionCount))
}

func (s overviewSection) prev() overviewSection {
	return overviewSection((int(s) + int(overviewSectionCount) - 1) % int(overviewSectionCount))
}

func formatPRStatus(pr *domain.PullRequest, tasks []domain.Task) string {
	if pr == nil {
		return mutedStyle.Render("no pull request")
	}
	parts := make([]string, 0, 8)
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
	if badge := formatReviewBadge(pr.Reviews); badge != "" {
		parts = append(parts, badge)
	}
	line := strings.Join(parts, " · ")
	st := lipgloss.NewStyle().Bold(true)
	if blocked || len(pr.Reviews.ChangeRequesters) > 0 {
		st = st.Foreground(lipgloss.Color("#f07178"))
	} else if pr.Draft {
		st = st.Foreground(lipgloss.Color("#e6c07b"))
	} else if len(pr.Reviews.Approvers) > 0 {
		st = st.Foreground(lipgloss.Color("#98c379"))
	} else {
		st = st.Foreground(lipgloss.Color("#98c379"))
	}
	return st.Render(line)
}

// formatReviewBadge is a short badge for lists/status: "✓3 ✗1 you✓".
func formatReviewBadge(rs domain.ReviewStatus) string {
	if !rs.HasReviews() && rs.ViewerDecision == domain.DecisionNone {
		return ""
	}
	var parts []string
	if n := len(rs.Approvers); n > 0 {
		parts = append(parts, fmt.Sprintf("✓%d", n))
	}
	if n := len(rs.ChangeRequesters); n > 0 {
		parts = append(parts, fmt.Sprintf("✗%d", n))
	}
	switch rs.ViewerDecision {
	case domain.DecisionApproved:
		parts = append(parts, "you✓")
	case domain.DecisionChangesRequested:
		parts = append(parts, "you✗")
	}
	return strings.Join(parts, " ")
}

func formatReviewsSection(rs domain.ReviewStatus, width int) string {
	var b strings.Builder
	b.WriteString(sectionHeader("Reviews", false))
	b.WriteByte('\n')
	if !rs.HasReviews() && rs.ViewerDecision == domain.DecisionNone {
		b.WriteString(mutedStyle.Render("  No approvals or change requests yet."))
		b.WriteByte('\n')
		return b.String()
	}
	if rs.ViewerDecision == domain.DecisionApproved {
		b.WriteString(draftStyle.Render("  You approved this PR."))
		b.WriteByte('\n')
	} else if rs.ViewerDecision == domain.DecisionChangesRequested {
		b.WriteString(errorStyle.Render("  You requested changes."))
		b.WriteByte('\n')
	}
	if len(rs.Approvers) > 0 {
		line := "  Approved: " + strings.Join(rs.Approvers, ", ")
		if width > 4 {
			line = truncate(line, width)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if len(rs.ChangeRequesters) > 0 {
		line := "  Changes requested: " + strings.Join(rs.ChangeRequesters, ", ")
		if width > 4 {
			line = truncate(line, width)
		}
		b.WriteString(errorStyle.Render(line))
		b.WriteByte('\n')
	}
	return b.String()
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

func formatSummarySection(summary, summaryErr string, summarizing, active bool) string {
	var b strings.Builder
	b.WriteString(sectionHeader("Summary", active))
	b.WriteByte('\n')
	switch {
	case summarizing:
		b.WriteString(mutedStyle.Render("  Summarizing…"))
		b.WriteByte('\n')
	case summaryErr != "":
		b.WriteString(errorStyle.Render("  " + summaryErr))
		b.WriteByte('\n')
	case strings.TrimSpace(summary) == "":
		b.WriteString(mutedStyle.Render("  Press S to summarize (configure ai: in config.yaml)"))
		b.WriteByte('\n')
	default:
		b.WriteString(summary)
		if !strings.HasSuffix(summary, "\n") {
			b.WriteByte('\n')
		}
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
	summary string,
	summaryErr string,
	summarizing bool,
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
	b.WriteString(mutedStyle.Render("tab section · S summarize · esc back"))
	b.WriteString("\n\n")

	b.WriteString(formatReviewsSection(prReviews(pr), width))
	b.WriteByte('\n')

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

	b.WriteString(formatSummarySection(summary, summaryErr, summarizing, sec == sectionSummary))
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

func prReviews(pr *domain.PullRequest) domain.ReviewStatus {
	if pr == nil {
		return domain.ReviewStatus{}
	}
	return pr.Reviews
}
