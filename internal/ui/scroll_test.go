package ui

import (
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestScrollDiffCursorCentersInViewport(t *testing.T) {
	m := Model{
		cursorLine:   40,
		diffWinStart: -1,
		diffWinEnd:   -1,
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
	want := h / 2
	if pos < want-1 || pos > want+1 {
		t.Fatalf("cursor screen pos %d want ~%d (target=%d y=%d h=%d)", pos, want, target, y, h)
	}

	m.cursorLine = 55
	m.renderDiff()
	target = m.contentLineForCursor()
	y = m.diffVP.YOffset
	pos = target - y
	if pos < want-1 || pos > want+1 {
		t.Fatalf("after move down: pos %d want ~%d (target=%d y=%d)", pos, want, target, y)
	}

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
		cursorLine:   0,
		diffWinStart: -1,
		diffWinEnd:   -1,
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
	if got-y < h/2 {
		t.Fatalf("at bottom: expected cursor in lower half, pos=%d h=%d", got-y, h)
	}
}

func TestDiffRenderWindowStickyWhileCentered(t *testing.T) {
	// Tall threads near the top of a sliding window used to yank the buffer
	// upward on every j and jump YOffset even with zz centering.
	lines := testDiffLines(600)
	for i := range lines {
		lines[i].Anchor = domain.Anchor{Path: "a.go", Line: lines[i].NewNumber, Side: domain.SideRight}
	}
	m := Model{
		cursorLine:   300,
		diffWinStart: -1,
		diffWinEnd:   -1,
		comments: []domain.Comment{
			{ID: "1", Path: "a.go", Body: "thread A\nline2\nline3\nline4\nline5", Anchor: &domain.Anchor{Path: "a.go", Line: 101, Side: domain.SideRight}},
			{ID: "2", Path: "a.go", Body: "thread B\nline2\nline3\nline4\nline5", Anchor: &domain.Anchor{Path: "a.go", Line: 102, Side: domain.SideRight}},
		},
		fileDiff: &domain.FileDiff{Path: "a.go", Lines: lines},
	}
	m.diffVP.Width = 80
	m.diffVP.Height = 20
	m.renderDiff()
	start := m.diffWinStart
	if start < 0 {
		t.Fatal("expected windowed render")
	}
	prevY := m.diffVP.YOffset
	wantPos := m.diffVP.Height / 2

	for step := 1; step <= 30; step++ {
		m.cursorLine = 300 + step
		m.renderDiff()
		if m.diffWinStart != start {
			t.Fatalf("step %d: window slid start %d → %d (want sticky)", step, start, m.diffWinStart)
		}
		target := m.contentLineForCursor()
		pos := target - m.diffVP.YOffset
		if pos < wantPos-1 || pos > wantPos+1 {
			t.Fatalf("step %d: cursor pos %d want ~%d", step, pos, wantPos)
		}
		delta := m.diffVP.YOffset - prevY
		if delta < 0 || delta > 3 {
			// One logical line is a single painted row; allow a little slack.
			t.Fatalf("step %d: YOffset jumped by %d (prev=%d now=%d) — layout shift", step, delta, prevY, m.diffVP.YOffset)
		}
		prevY = m.diffVP.YOffset
	}
}

func TestDiffRenderWindowReanchorsNearEdge(t *testing.T) {
	m := Model{
		cursorLine:   200,
		diffWinStart: -1,
		diffWinEnd:   -1,
		fileDiff:     &domain.FileDiff{Path: "a.go", Lines: testDiffLines(800)},
	}
	m.diffVP.Width = 80
	m.diffVP.Height = 16
	m.renderDiff()
	start := m.diffWinStart
	// Jump close to the bottom edge of the window → must re-anchor.
	m.cursorLine = m.diffWinEnd - 10
	m.renderDiff()
	if m.diffWinStart == start {
		t.Fatalf("expected re-anchor near edge; start stayed %d", start)
	}
	target := m.contentLineForCursor()
	pos := target - m.diffVP.YOffset
	want := m.diffVP.Height / 2
	if pos < want-1 || pos > want+1 {
		t.Fatalf("after re-anchor: pos %d want ~%d", pos, want)
	}
}
