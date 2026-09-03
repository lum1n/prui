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

func formatPRStatus(pr *domain.PullRequest, tasks []domain.Task, width int) string {
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
	meta := strings.Join(parts, " · ")
	st := lipgloss.NewStyle().Bold(true)
	if blocked || len(pr.Reviews.ChangeRequesters) > 0 {
		st = st.Foreground(badgeChangesFg)
	} else if pr.Draft {
		st = st.Foreground(listAccent)
	} else {
		st = st.Foreground(badgeApproveFg)
	}
	out := st.Render(meta)
	if badge := formatReviewBadge(pr.Reviews); badge != "" {
		out += mutedStyle.Render(" · ") + badge
	}
	return wrapWidth(out, width)
}

// formatReviewBadge is a short badge for lists/status: "✓3 ✗1 you✓" with color.
func formatReviewBadge(rs domain.ReviewStatus) string {
	if !rs.HasReviews() && rs.ViewerDecision == domain.DecisionNone {
		return ""
	}
	approve := lipgloss.NewStyle().Foreground(badgeApproveFg)
	changes := lipgloss.NewStyle().Foreground(badgeChangesFg)
	var parts []string
	if n := len(rs.Approvers); n > 0 {
		parts = append(parts, approve.Render(fmt.Sprintf("✓%d", n)))
	}
	if n := len(rs.ChangeRequesters); n > 0 {
		parts = append(parts, changes.Render(fmt.Sprintf("✗%d", n)))
	}
	switch rs.ViewerDecision {
	case domain.DecisionApproved:
		parts = append(parts, approve.Render("you✓"))
	case domain.DecisionChangesRequested:
		parts = append(parts, changes.Render("you✗"))
	}
	return strings.Join(parts, " ")
}

func formatReviewsSection(rs domain.ReviewStatus, width int) string {
	var b strings.Builder
	b.WriteString(sectionHeader("Reviews", false))
	b.WriteByte('\n')
	if !rs.HasReviews() && rs.ViewerDecision == domain.DecisionNone {
		b.WriteString(wrapWidth(mutedStyle.Render("  No approvals or change requests yet."), width))
		b.WriteByte('\n')
		return b.String()
	}
	if rs.ViewerDecision == domain.DecisionApproved {
		b.WriteString(wrapWidth(draftStyle.Render("  You approved this PR."), width))
		b.WriteByte('\n')
	} else if rs.ViewerDecision == domain.DecisionChangesRequested {
		b.WriteString(wrapWidth(errorStyle.Render("  You requested changes."), width))
		b.WriteByte('\n')
	}
	if len(rs.Approvers) > 0 {
		line := "  Approved: " + strings.Join(rs.Approvers, ", ")
		b.WriteString(wrapWidth(line, width))
		b.WriteByte('\n')
	}
	if len(rs.ChangeRequesters) > 0 {
		line := "  Changes requested: " + strings.Join(rs.ChangeRequesters, ", ")
		b.WriteString(wrapWidth(errorStyle.Render(line), width))
		b.WriteByte('\n')
	}
	return b.String()
}

func formatTasksSection(tasks []domain.Task, cursor int, active bool, width int) string {
	var b strings.Builder
	b.WriteString(sectionHeader("Tasks", active))
	b.WriteByte('\n')
	if len(tasks) == 0 {
		b.WriteString(wrapWidth(mutedStyle.Render("  No tasks on this PR."), width))
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
		body := strings.TrimSpace(t.Body)
		head := fmt.Sprintf("%s%s %s", prefix, mark, body)
		if active && i == cursor {
			head = draftStyle.Render(head)
		}
		meta := ""
		if t.Author != "" {
			meta += mutedStyle.Render(" · " + t.Author)
		}
		if t.Required && !t.Done {
			meta += mutedStyle.Render(" · required")
		}
		b.WriteString(wrapWidth(head+meta, width))
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
	b.WriteString(wrapWidth(sectionHeader("Conversation", active), width))
	b.WriteByte('\n')
	hint := "  tab here · j/k select · R reply · c new"
	if active {
		hint = "  j/k select · R reply · c new · space N/A"
	}
	b.WriteString(wrapWidth(mutedStyle.Render(hint), width))
	b.WriteByte('\n')
	b.WriteByte('\n')
	if len(entries) == 0 {
		b.WriteString(wrapWidth(mutedStyle.Render("  No general comments yet. Press c to add a draft."), width))
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

func formatSummarySection(summary, summaryErr string, summarizing, active bool, detail string, width int) string {
	var b strings.Builder
	title := "Summary"
	if detail != "" {
		title = "Summary · " + detail
	}
	b.WriteString(wrapWidth(sectionHeader(title, active), width))
	b.WriteByte('\n')
	switch {
	case summarizing:
		b.WriteString(wrapWidth(mutedStyle.Render("  Summarizing ("+detail+")…"), width))
		b.WriteByte('\n')
	case summaryErr != "":
		b.WriteString(wrapWidth(errorStyle.Render("  "+summaryErr), width))
		b.WriteByte('\n')
	case strings.TrimSpace(summary) == "":
		b.WriteString(wrapWidth(mutedStyle.Render("  Press S to summarize · s cycles detail (short/medium/full)"), width))
		b.WriteByte('\n')
	default:
		b.WriteString(wrapWidth(summary, width))
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
	summaryDetail string,
	width int,
) string {
	if width < 20 {
		width = 20
	}
	var b strings.Builder
	title := "Pull request"
	if pr != nil && pr.Title != "" {
		title = pr.Title
	}
	b.WriteString(wrapWidth(titleStyle.Render(title), width))
	b.WriteByte('\n')
	b.WriteString(formatPRStatus(pr, tasks, width))
	b.WriteByte('\n')
	b.WriteString(wrapWidth(mutedStyle.Render("tab section · S summarize · s detail · esc back"), width))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(formatReviewsSection(prReviews(pr), width))
	b.WriteByte('\n')
	b.WriteString(formatTasksSection(tasks, taskCursor, sec == sectionTasks, width))
	b.WriteByte('\n')
	b.WriteString(wrapWidth(sectionHeader("Description", sec == sectionDescription), width))
	b.WriteByte('\n')
	if strings.TrimSpace(description) == "" {
		b.WriteString(wrapWidth(mutedStyle.Render("  (no description)"), width))
		b.WriteByte('\n')
	} else {
		b.WriteString(wrapWidth(description, width))
		if !strings.HasSuffix(description, "\n") {
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	b.WriteString(formatSummarySection(summary, summaryErr, summarizing, sec == sectionSummary, summaryDetail, width))
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
