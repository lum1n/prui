package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lum1n/prui/internal/domain"
)

func testDiffLines(n int) []domain.DiffLine {
	lines := make([]domain.DiffLine, n)
	for i := range lines {
		lines[i] = domain.DiffLine{
			Kind:      domain.LineContext,
			Text:      " line",
			NewNumber: i + 1,
			OldNumber: i + 1,
		}
	}
	return lines
}

func TestReviewMouseWheelScrollsDiff(t *testing.T) {
	m := Model{
		screen:        screenReview,
		pane:          paneDiff,
		width:         100,
		height:        40,
		leftWidth:     30,
		rightWidth:    70,
		contentHeight: 37,
		cursorLine:    10,
		fileDiff: &domain.FileDiff{
			Path:  "a.go",
			Lines: testDiffLines(40),
		},
	}
	m.diffVP.Width = 66
	m.diffVP.Height = 30

	next, _ := m.handleReviewMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
		X:      50,
		Y:      5,
	})
	got := next.(Model)
	if got.cursorLine != 10+mouseWheelLines {
		t.Fatalf("wheel down: cursor %d want %d", got.cursorLine, 10+mouseWheelLines)
	}

	next, _ = got.handleReviewMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelUp,
		X:      50,
		Y:      5,
	})
	got = next.(Model)
	if got.cursorLine != 10 {
		t.Fatalf("wheel up: cursor %d want 10", got.cursorLine)
	}
}

func TestReviewMouseWheelIgnoresChrome(t *testing.T) {
	m := Model{
		screen:        screenReview,
		pane:          paneDiff,
		width:         100,
		height:        40,
		leftWidth:     30,
		contentHeight: 37,
		cursorLine:    5,
		fileDiff: &domain.FileDiff{
			Path:  "a.go",
			Lines: testDiffLines(20),
		},
	}
	next, _ := m.handleReviewMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
		X:      50,
		Y:      0, // title row
	})
	got := next.(Model)
	if got.cursorLine != 5 {
		t.Fatalf("title wheel moved cursor to %d", got.cursorLine)
	}
}

func TestReviewMouseClickSelectsDiffLine(t *testing.T) {
	m := Model{
		screen:        screenReview,
		pane:          paneDiff,
		width:         100,
		height:        40,
		leftWidth:     30,
		rightWidth:    70,
		contentHeight: 37,
		cursorLine:    0,
		fileDiff: &domain.FileDiff{
			Path:  "a.go",
			Lines: testDiffLines(20),
		},
	}
	m.diffVP.Width = 66
	m.diffVP.Height = 30
	m.renderDiff()

	// With YOffset 0, click the viewport row that maps to cursor index 7.
	target := -1
	for i, idx := range m.diffClickMap {
		if idx == 7 {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatal("no click map entry for line 7")
	}
	if target >= m.diffVP.Height {
		t.Fatalf("mapped line %d outside viewport height %d", target, m.diffVP.Height)
	}

	next, _ := m.handleReviewMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      50,
		Y:      1 + 1 + target, // title + panel border + content row
	})
	got := next.(Model)
	if got.cursorLine != 7 {
		t.Fatalf("click: cursor %d want 7 (content row %d)", got.cursorLine, target)
	}
	if got.pane != paneDiff {
		t.Fatalf("pane=%v want diff", got.pane)
	}
}
