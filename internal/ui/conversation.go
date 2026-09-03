package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lum1n/prui/internal/domain"
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

func formatConversation(entries []convEntry, cursor int, width int) string {
	if width < 20 {
		width = 20
	}
	var b strings.Builder
	b.WriteString(wrapWidth(titleStyle.Render("Conversation"), width))
	b.WriteString("\n")
	b.WriteString(wrapWidth(mutedStyle.Render("Threads · j/k select · R reply · c new"), width))
	b.WriteString("\n\n")

	if len(entries) == 0 {
		b.WriteString(wrapWidth(mutedStyle.Render("No general comments yet. Press c to add a draft."), width))
		b.WriteByte('\n')
		return b.String()
	}

	for i, e := range entries {
		b.WriteString(formatConversationEntry(e, i == cursor, width))
		b.WriteByte('\n')
	}
	return b.String()
}

func formatConversationEntry(e convEntry, selected bool, width int) string {
	indent := strings.Repeat("  ", e.Depth)
	branch := ""
	if e.Depth > 0 {
		branch = "↳ "
	}
	meta := e.Author
	if e.Draft {
		meta = draftStyle.Render("✎ draft · " + e.Author)
	} else {
		meta = lipgloss.NewStyle().Foreground(lipgloss.Color("#61afef")).Bold(true).Render(e.Author)
	}
	if e.Path != "" {
		meta += mutedStyle.Render(" · file " + e.Path)
	}
	prefix := "  "
	if selected {
		prefix = "> "
	}
	header := wrapWidth(prefix+indent+branch+meta, width)
	bodyWidth := width - 2 - len(indent)
	if bodyWidth < 12 {
		bodyWidth = 12
	}
	wrapped := wrapWidth(strings.TrimSpace(e.Body), bodyWidth)
	// Indent wrapped body under the header.
	bodyLines := strings.Split(wrapped, "\n")
	for i := range bodyLines {
		bodyLines[i] = "  " + indent + bodyLines[i]
	}
	ruleW := width
	if ruleW > 48 {
		ruleW = 48
	}
	if ruleW < 8 {
		ruleW = 8
	}
	sep := mutedStyle.Render(strings.Repeat("─", ruleW))
	if selected {
		sep = lipgloss.NewStyle().Foreground(lipgloss.Color("#c8a35a")).Render(strings.Repeat("─", ruleW))
	}
	return fmt.Sprintf("%s\n%s\n%s\n", header, strings.Join(bodyLines, "\n"), sep)
}
