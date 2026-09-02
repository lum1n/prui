package ui

import (
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

// prTabBarHeight is tabs row + separator; keep in sync with renderPRTabs.
const prTabBarHeight = 2

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

func renderPRTabs(active prListTab, width int) string {
	if width < 24 {
		width = 24
	}

	activeStyle := lipgloss.NewStyle().
		Foreground(listFg).
		Background(listSelBg).
		Bold(true).
		Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(listFgDim).
		Padding(0, 1)

	cells := make([]string, 0, prListTabCount)
	underLeft := 0
	underW := 0
	cursor := 0
	for i := prListTab(0); i < prListTabCount; i++ {
		if i > 0 {
			gap := "  "
			cells = append(cells, gap)
			cursor += lipgloss.Width(gap)
		}
		var cell string
		if i == active {
			cell = activeStyle.Render(i.label())
			underLeft = cursor
			underW = lipgloss.Width(cell)
		} else {
			cell = inactiveStyle.Render(i.label())
		}
		cells = append(cells, cell)
		cursor += lipgloss.Width(cell)
	}

	tabs := strings.Join(cells, "")
	hint := mutedStyle.Render("tab ←→")
	gap := width - lipgloss.Width(tabs) - lipgloss.Width(hint)
	var row string
	switch {
	case gap > 0:
		row = tabs + strings.Repeat(" ", gap) + hint
	default:
		row = lipgloss.NewStyle().Width(width).MaxWidth(width).Render(tabs)
	}
	row = lipgloss.NewStyle().Width(width).MaxWidth(width).Render(row)

	if underW < 2 {
		underW = 2
	}
	if underLeft+underW > width {
		underW = width - underLeft
	}
	if underW < 0 {
		underW = 0
	}
	right := width - underLeft - underW
	if right < 0 {
		right = 0
	}
	dim := lipgloss.NewStyle().Foreground(listFgDim)
	acc := lipgloss.NewStyle().Foreground(listAccent)
	rule := dim.Render(strings.Repeat("─", underLeft)) +
		acc.Render(strings.Repeat("─", underW)) +
		dim.Render(strings.Repeat("─", right))

	return lipgloss.JoinVertical(lipgloss.Left, row, rule)
}

func prListHelpLine() string {
	return "enter open · tab/←→ · 1–3 jump · / filter · O browser · ? · q"
}
