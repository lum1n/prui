package ui

import (
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestScrollDiffCursorIntoViewKeepsOffset(t *testing.T) {
	m := Model{
		cursorLine: 10,
		fileDiff: &domain.FileDiff{
			Path:  "a.go",
			Lines: testDiffLines(40),
		},
	}
	m.diffVP.Width = 80
	m.diffVP.Height = 12
	m.renderDiff()

	target := m.contentLineForCursor()
	if target < 0 {
		t.Fatal("no content line for cursor")
	}
	// Put cursor near the bottom of the viewport.
	m.diffVP.SetYOffset(max(0, target-m.diffVP.Height+3))
	prev := m.diffVP.YOffset

	m.cursorLine = 11
	m.renderDiff()
	// Small move within view should not jump back to top / re-center hard.
	if m.diffVP.YOffset == 0 && prev > 0 {
		t.Fatalf("viewport jumped to top (was %d)", prev)
	}
	got := m.contentLineForCursor()
	y := m.diffVP.YOffset
	if got < y || got >= y+m.diffVP.Height {
		t.Fatalf("cursor content line %d not visible at y=%d h=%d", got, y, m.diffVP.Height)
	}
}

func TestScrollDiffCursorIntoViewScrollsWhenNeeded(t *testing.T) {
	m := Model{
		cursorLine: 0,
		fileDiff: &domain.FileDiff{
			Path:  "a.go",
			Lines: testDiffLines(80),
		},
	}
	m.diffVP.Width = 80
	m.diffVP.Height = 10
	m.renderDiff()
	m.diffVP.SetYOffset(0)

	m.cursorLine = 50
	m.renderDiff()
	got := m.contentLineForCursor()
	y := m.diffVP.YOffset
	if got < y || got >= y+m.diffVP.Height {
		t.Fatalf("after jump, cursor %d not visible at y=%d h=%d\n%s", got, y, m.diffVP.Height, strings.Join(nil, ""))
	}
}
