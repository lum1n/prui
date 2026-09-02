package ui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lum1n/prui/internal/domain"
)

// Dim list chrome — no Charm pink/purple defaults.
var (
	listAccent     = lipgloss.Color("#c8a35a")
	listFg         = lipgloss.Color("#c8ccd4")
	listFgDim      = lipgloss.Color("#6b7280")
	listFgMuted    = lipgloss.Color("#8b919a")
	listSelBg      = lipgloss.Color("#2a303c")
	listTitleFg    = lipgloss.Color("#d5d8de")
	listFilterFg   = lipgloss.Color("#a3b18a")
	statusAddFg    = lipgloss.Color("#98c379")
	statusModFg    = lipgloss.Color("#e5c07b")
	statusDelFg    = lipgloss.Color("#e06c75")
	statusRenameFg = lipgloss.Color("#61afef")
	badgeApproveFg = lipgloss.Color("#98c379")
	badgeChangesFg = lipgloss.Color("#e06c75")
)

func dimListStyles() list.Styles {
	s := list.DefaultStyles()
	s.TitleBar = lipgloss.NewStyle().Padding(0, 0, 0, 1)
	s.Title = lipgloss.NewStyle().
		Foreground(listTitleFg).
		Bold(true).
		Padding(0, 1)
	s.Spinner = lipgloss.NewStyle().Foreground(listFgDim)
	s.FilterPrompt = lipgloss.NewStyle().Foreground(listFilterFg)
	s.FilterCursor = lipgloss.NewStyle().Foreground(listAccent)
	s.DefaultFilterCharacterMatch = lipgloss.NewStyle().Underline(true).Foreground(listAccent)
	s.StatusBar = lipgloss.NewStyle().Foreground(listFgDim).Padding(0, 0, 0, 1)
	s.StatusEmpty = lipgloss.NewStyle().Foreground(listFgDim)
	s.StatusBarActiveFilter = lipgloss.NewStyle().Foreground(listFgMuted)
	s.StatusBarFilterCount = lipgloss.NewStyle().Foreground(listFgDim)
	s.NoItems = lipgloss.NewStyle().Foreground(listFgDim)
	s.ArabicPagination = lipgloss.NewStyle().Foreground(listFgDim)
	s.PaginationStyle = lipgloss.NewStyle().PaddingLeft(1)
	s.HelpStyle = lipgloss.NewStyle().Padding(0, 0, 0, 1).Foreground(listFgDim)
	s.ActivePaginationDot = lipgloss.NewStyle().Foreground(listFgMuted).SetString("•")
	s.InactivePaginationDot = lipgloss.NewStyle().Foreground(listFgDim).SetString("•")
	s.DividerDot = lipgloss.NewStyle().Foreground(listFgDim).SetString(" · ")
	return s
}

func dimItemStyles() list.DefaultItemStyles {
	s := list.NewDefaultItemStyles()
	s.NormalTitle = lipgloss.NewStyle().
		Foreground(listFg).
		Padding(0, 0, 0, 2)
	s.NormalDesc = s.NormalTitle.Foreground(listFgDim)
	s.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(listAccent).
		Foreground(listFg).
		Background(listSelBg).
		Bold(true).
		Padding(0, 0, 0, 1)
	s.SelectedDesc = s.SelectedTitle.
		Foreground(listFgMuted).
		Bold(false)
	s.DimmedTitle = lipgloss.NewStyle().
		Foreground(listFgDim).
		Padding(0, 0, 0, 2)
	s.DimmedDesc = s.DimmedTitle.Foreground(listFgDim)
	s.FilterMatch = lipgloss.NewStyle().Underline(true).Foreground(listAccent)
	return s
}

func configureList(l *list.Model, title, singular, plural string) {
	l.Title = title
	l.Styles = dimListStyles()
	l.SetShowStatusBar(false) // we render the count at the bottom ourselves
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetStatusBarItemName(singular, plural)
}

func newPRDelegate() prDelegate {
	return prDelegate{styles: dimItemStyles()}
}

// prDelegate keeps review badge colors even when the row is selected.
type prDelegate struct {
	styles list.DefaultItemStyles
}

func (d prDelegate) Height() int                         { return 2 }
func (d prDelegate) Spacing() int                        { return 0 }
func (d prDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d prDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	pi, ok := item.(prItem)
	if !ok || m.Width() <= 0 {
		return
	}

	title := fmt.Sprintf("#%d  %s", pi.pr.Ref.Number, pi.pr.Title)
	base := fmt.Sprintf("%s · %s", pi.pr.Author, pi.pr.State)
	badge := formatReviewBadge(pi.pr.Reviews)

	padLeft := d.styles.NormalTitle.GetPaddingLeft()
	textwidth := m.Width() - padLeft - d.styles.NormalTitle.GetPaddingRight()
	if textwidth < 8 {
		textwidth = 8
	}
	title = ansi.Truncate(title, textwidth, "…")
	base = ansi.Truncate(base, textwidth, "…")

	isSelected := index == m.Index() && m.FilterState() != list.Filtering
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""

	var titleOut, descOut string
	switch {
	case emptyFilter:
		titleOut = d.styles.DimmedTitle.Render(title)
		descOut = d.styles.DimmedDesc.Render(base)
	case isSelected:
		titleOut = d.styles.SelectedTitle.Render(title)
		descOut = d.styles.SelectedDesc.Render(base)
		if badge != "" {
			descOut += d.styles.SelectedDesc.Inline(true).Render(" · ") + badge
		}
	default:
		titleOut = d.styles.NormalTitle.Render(title)
		descOut = d.styles.NormalDesc.Render(base)
		if badge != "" {
			descOut += d.styles.NormalDesc.Inline(true).Render(" · ") + badge
		}
	}
	fmt.Fprintf(w, "%s\n%s", titleOut, descOut) //nolint:errcheck
}

func newFileDelegate() fileDelegate {
	return fileDelegate{styles: dimItemStyles()}
}

// fileDelegate is a single-line file row with a colored git status glyph.
type fileDelegate struct {
	styles list.DefaultItemStyles
}

func (d fileDelegate) Height() int                         { return 1 }
func (d fileDelegate) Spacing() int                        { return 0 }
func (d fileDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d fileDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	fi, ok := item.(fileItem)
	if !ok || m.Width() <= 0 {
		return
	}

	mark, markStyle := fileStatusGlyph(fi.file.Status)
	extra := ""
	if fi.drafts > 0 || fi.comments > 0 {
		extra = fmt.Sprintf("  [%d/%d]", fi.drafts, fi.comments)
	}
	prefixW := 2 // "X "
	path := fi.file.Path
	if fi.maxTitle > 0 {
		budget := fi.maxTitle - prefixW - len(extra)
		path = fitPathKeepBase(path, budget)
	}
	rest := path + extra

	isSelected := index == m.Index() && m.FilterState() != list.Filtering
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""
	textwidth := m.Width() - 2
	if textwidth < 4 {
		textwidth = 4
	}
	rest = ansi.Truncate(rest, textwidth-prefixW, "…")

	var line string
	switch {
	case emptyFilter:
		line = d.styles.DimmedTitle.Render(mark + " " + rest)
	case isSelected:
		markPart := markStyle.Background(listSelBg).Bold(true).Render(mark)
		restPart := lipgloss.NewStyle().
			Foreground(listFg).
			Background(listSelBg).
			Bold(true).
			Render(" " + rest)
		border := lipgloss.NewStyle().
			Foreground(listAccent).
			Background(listSelBg).
			SetString("│")
		line = border.String() + markPart + restPart
	default:
		line = " " + markStyle.Render(mark) + " " + d.styles.NormalTitle.Inline(true).Render(rest)
	}
	fmt.Fprint(w, line) //nolint:errcheck
}

// fileStatusGlyph returns a git-style status letter and its color.
func fileStatusGlyph(st domain.FileStatus) (string, lipgloss.Style) {
	switch st {
	case domain.FileAdded:
		return "A", lipgloss.NewStyle().Foreground(statusAddFg)
	case domain.FileRemoved:
		return "D", lipgloss.NewStyle().Foreground(statusDelFg)
	case domain.FileRenamed:
		return "R", lipgloss.NewStyle().Foreground(statusRenameFg)
	default:
		return "M", lipgloss.NewStyle().Foreground(statusModFg)
	}
}

// listViewWithFooter renders the list and a bottom "N items" line.
func listViewWithFooter(l list.Model) string {
	n := len(l.VisibleItems())
	singular, plural := l.StatusBarItemName()
	name := plural
	if n == 1 {
		name = singular
	}
	var footer string
	switch {
	case len(l.Items()) == 0:
		footer = mutedStyle.Render("No " + plural)
	case l.FilterState() == list.Filtering && n == 0:
		footer = mutedStyle.Render("Nothing matched")
	default:
		text := fmt.Sprintf("%d %s", n, name)
		if filtered := len(l.Items()) - n; filtered > 0 {
			text += fmt.Sprintf(" · %d filtered", filtered)
		}
		footer = mutedStyle.Render(text)
	}
	footer = lipgloss.NewStyle().Padding(0, 1).Render(footer)
	return lipgloss.JoinVertical(lipgloss.Left, l.View(), footer)
}
