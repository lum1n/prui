package ui

import (
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestScrollDiffCursorCentersInViewport(t *testing.T) {
	m := Model{
		cursorLine: 40,
		fileDiff: &domain.FileDiff{
			Path:  "a.go",
			Lines: testDiffLines(100),
		},
	}
	m.diffVP.Width = 80
	m.diffVP.Height = 20
	m.renderDiff()

	target := m.contentLineForCursor()
	if target < 0 {
		t.Fatal("no content line for cursor")
	}
	y := m.diffVP.YOffset
	h := m.diffVP.Height
	pos := target - y
	// Cursor should sit near vertical center (±1 for odd heights).
	want := h / 2
	if pos < want-1 || pos > want+1 {
		t.Fatalf("cursor screen pos %d want ~%d (target=%d y=%d h=%d)", pos, want, target, y, h)
	}

	// Moving down keeps it centered.
	m.cursorLine = 55
	m.renderDiff()
	target = m.contentLineForCursor()
	y = m.diffVP.YOffset
	pos = target - y
	if pos < want-1 || pos > want+1 {
		t.Fatalf("after move down: pos %d want ~%d (target=%d y=%d)", pos, want, target, y)
	}

	// Moving up also keeps it centered (not stuck at bottom).
	m.cursorLine = 30
	m.renderDiff()
	target = m.contentLineForCursor()
	y = m.diffVP.YOffset
	pos = target - y
	if pos < want-1 || pos > want+1 {
		t.Fatalf("after move up: pos %d want ~%d (target=%d y=%d)", pos, want, target, y)
	}
}

func TestScrollDiffCursorClampsNearEnds(t *testing.T) {
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
	if m.diffVP.YOffset != 0 {
		t.Fatalf("at top: YOffset=%d want 0", m.diffVP.YOffset)
	}

	m.cursorLine = 79
	m.renderDiff()
	got := m.contentLineForCursor()
	y := m.diffVP.YOffset
	h := m.diffVP.Height
	if got < y || got >= y+h {
		t.Fatalf("at bottom: cursor %d not visible at y=%d h=%d", got, y, h)
	}
	// Near EOF the cursor sits in the lower half (can't center past max scroll).
	if got-y < h/2 {
		t.Fatalf("at bottom: expected cursor in lower half, pos=%d h=%d", got-y, h)
	}
}
