package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type prListTab int

const (
	tabOpen prListTab = iota
	tabDrafts
	tabMerged
	prListTabCount
)

func (t prListTab) next() prListTab {
	return prListTab((int(t) + 1) % int(prListTabCount))
}

func (t prListTab) prev() prListTab {
	return prListTab((int(t) - 1 + int(prListTabCount)) % int(prListTabCount))
}

func (t prListTab) label() string {
	switch t {
	case tabDrafts:
		return "Drafts"
	case tabMerged:
		return "Merged"
	default:
		return "Open"
	}
}

func (t prListTab) listState() string {
	switch t {
	case tabDrafts:
		return "draft"
	case tabMerged:
		return "merged"
	default:
		return "open"
	}
}

func (t prListTab) statusNoun() string {
	switch t {
	case tabDrafts:
		return "draft PRs"
	case tabMerged:
		return "merged PRs"
	default:
		return "open PRs"
	}
}

func renderPRTabs(active prListTab) string {
	parts := make([]string, 0, prListTabCount)
	for i := prListTab(0); i < prListTabCount; i++ {
		label := fmt.Sprintf(" %s ", i.label())
		if i == active {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(listFg).
				Background(listSelBg).
				Bold(true).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(listAccent).
				Render(label))
		} else {
			parts = append(parts, mutedStyle.Render(label))
		}
	}
	hint := mutedStyle.Render("  tab/←→ switch")
	return strings.Join(parts, mutedStyle.Render(" │ ")) + hint
}
