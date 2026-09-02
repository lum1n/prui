package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/vegard/prui/internal/domain"
)

// isConversationComment is a PR discussion comment not tied to a diff line.
// Includes fully general comments and file-level anchors (path, no line).
func isConversationComment(c domain.Comment) bool {
	if c.Anchor == nil {
		return true
	}
	return c.Anchor.Line <= 0
}

func conversationComments(all []domain.Comment) []domain.Comment {
	out := make([]domain.Comment, 0)
	for _, c := range all {
		if isConversationComment(c) {
			out = append(out, c)
		}
	}
	return out
}

func generalDrafts(drafts []domain.DraftComment) []domain.DraftComment {
	out := make([]domain.DraftComment, 0)
	for _, d := range drafts {
		if d.Anchor == nil {
			out = append(out, d)
		}
	}
	return out
}

func formatConversation(comments []domain.Comment, drafts []domain.DraftComment, width int) string {
	if width < 40 {
		width = 40
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Conversation"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("PR comments not tied to a diff line"))
	b.WriteString("\n\n")

	if len(comments) == 0 && len(drafts) == 0 {
		b.WriteString(mutedStyle.Render("No general comments yet. Press c to add a draft."))
		b.WriteByte('\n')
		return b.String()
	}

	for _, c := range comments {
		b.WriteString(formatConversationEntry(c.Author, c.Body, c.Path, c.Created, false, width))
		b.WriteByte('\n')
	}
	for _, d := range drafts {
		b.WriteString(formatConversationEntry("you", d.Body, d.Path, time.Time{}, true, width))
		b.WriteByte('\n')
	}
	return b.String()
}

func formatConversationEntry(author, body, path string, created time.Time, draft bool, width int) string {
	meta := author
	if draft {
		meta = draftStyle.Render("✎ draft · " + author)
	} else {
		meta = lipgloss.NewStyle().Foreground(lipgloss.Color("#61afef")).Bold(true).Render(author)
	}
	if path != "" {
		meta += mutedStyle.Render(" · file " + path)
	}
	if !created.IsZero() {
		meta += mutedStyle.Render(" · " + created.Local().Format("2006-01-02 15:04"))
	}
	wrapped := lipgloss.NewStyle().Width(width - 2).Render(strings.TrimSpace(body))
	sep := mutedStyle.Render(strings.Repeat("─", min(width, 48)))
	return fmt.Sprintf("%s\n%s\n%s\n", meta, wrapped, sep)
}
