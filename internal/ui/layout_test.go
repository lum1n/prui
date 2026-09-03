package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestSplitPanelWidthsSum(t *testing.T) {
	for _, total := range []int{40, 80, 120, 30, 100} {
		l, r := splitPanelWidths(total)
		if l+r != total {
			t.Fatalf("total=%d left=%d right=%d sum=%d", total, l, r, l+r)
		}
		if l < 1 || r < 1 {
			t.Fatalf("non-positive panel: %d+%d", l, r)
		}
	}
}

func TestRenderBorderedPanelFitsOuter(t *testing.T) {
	sty := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	inner := strings.Repeat(strings.Repeat("x", 40)+"\n", 30)
	for _, tc := range []struct{ w, h int }{{40, 20}, {24, 12}, {60, 30}} {
		out := renderBorderedPanel(sty, tc.w, tc.h, inner)
		if got := lipgloss.Width(out); got != tc.w {
			t.Fatalf("width: got %d want %d", got, tc.w)
		}
		if got := lipgloss.Height(out); got != tc.h {
			t.Fatalf("height: got %d want %d\n%s", got, tc.h, out)
		}
	}
}

func TestReviewBodyFitsTerminal(t *testing.T) {
	sty := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	termW, termH := 100, 40
	chrome := 3 // title + help + status
	contentH := termH - chrome
	lw, rw := splitPanelWidths(termW)
	left := renderBorderedPanel(sty, lw, contentH, strings.Repeat("file\n", 50))
	right := renderBorderedPanel(sty, rw, contentH, strings.Repeat(strings.Repeat("d", 80)+"\n", 50))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	if lipgloss.Width(body) != termW {
		t.Fatalf("body width %d want %d", lipgloss.Width(body), termW)
	}
	if lipgloss.Height(body) != contentH {
		t.Fatalf("body height %d want %d", lipgloss.Height(body), contentH)
	}
}

func TestBorderedPanelShortContentStillFills(t *testing.T) {
	sty := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	out := renderBorderedPanel(sty, 40, 20, "hi")
	if got := lipgloss.Width(out); got != 40 {
		t.Fatalf("width: got %d want 40", got)
	}
	if got := lipgloss.Height(out); got != 20 {
		t.Fatalf("height: got %d want 20", got)
	}
}
