package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBorderedPanelHeightAccounting(t *testing.T) {
	sty := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	inner := strings.Repeat("line\n", 50)
	for _, h := range []int{10, 20, 30} {
		out := renderBorderedPanel(sty, 40, h, inner)
		got := lipgloss.Height(out)
		t.Logf("outerH=%d got=%d", h, got)
		if got != h {
			t.Fatalf("outerH=%d got height %d", h, got)
		}
		// Also check without MaxHeight
		raw := sty.Width(38).Height(h - 2).Render(inner)
		t.Logf("  without MaxHeight: %d (content box asked %d)", lipgloss.Height(raw), h-2)
		raw2 := sty.Width(38).Height(h - 2).MaxHeight(h).Render(inner)
		t.Logf("  with MaxHeight(%d): %d", h, lipgloss.Height(raw2))
		raw3 := sty.Width(38).Height(h - 2).MaxHeight(h - 2).Render(inner)
		t.Logf("  with MaxHeight(%d): %d", h-2, lipgloss.Height(raw3))
	}
}

func TestFullFrameWithBorders(t *testing.T) {
	termW, termH := 100, 40
	chrome := 3
	contentH := termH - chrome
	lw, rw := splitPanelWidths(termW)
	sty := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	left := renderBorderedPanel(sty, lw, contentH, strings.Repeat("L\n", 100))
	right := renderBorderedPanel(sty, rw, contentH, strings.Repeat("R\n", 100))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	title := lipgloss.NewStyle().Width(termW).MaxHeight(1).Render("title")
	help := lipgloss.NewStyle().Width(termW).MaxHeight(1).Render("help")
	status := lipgloss.NewStyle().Width(termW).MaxHeight(1).Render("status")
	body = lipgloss.NewStyle().Height(contentH).MaxHeight(contentH).Render(body)
	out := lipgloss.JoinVertical(lipgloss.Left, title, body, help, status)
	pinned := lipgloss.NewStyle().MaxWidth(termW).MaxHeight(termH).Height(termH).Render(out)
	t.Logf("body=%d out=%d pinned=%d left=%d right=%d",
		lipgloss.Height(body), lipgloss.Height(out), lipgloss.Height(pinned),
		lipgloss.Height(left), lipgloss.Height(right))
	if lipgloss.Height(pinned) != termH {
		t.Fatalf("pinned %d", lipgloss.Height(pinned))
	}
	if lipgloss.Height(out) != termH {
		// This would cause alt-screen scroll before pin!
		t.Errorf("UNPINNED out height %d > term %d — pin masks overflow of %+d",
			lipgloss.Height(out), termH, lipgloss.Height(out)-termH)
	}
	fmt.Println("ok")
}
