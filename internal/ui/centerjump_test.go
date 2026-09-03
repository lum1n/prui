package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

func TestCenterScrollStepIsOneLine(t *testing.T) {
	lines := testDiffLines(120)
	for i := range lines {
		lines[i].Text = " code line"
		lines[i].Anchor = domain.Anchor{Path: "a.go", Line: i + 1, Side: domain.SideRight}
	}
	fileList := list.New(nil, newFileDelegate(), 30, 20)
	configureList(&fileList, "Files", "file", "files")
	m := Model{
		screen: screenReview, pane: paneDiff,
		width: 100, height: 30, contentHeight: 27,
		leftWidth: 30, rightWidth: 70,
		cursorLine: 40, diffWinStart: -1, diffWinEnd: -1,
		fileDiff: &domain.FileDiff{Path: "a.go", Lines: lines},
		fileList: fileList,
		hl:       diff.NewHighlighter("dark"),
		comments: []domain.Comment{{
			ID: "1", Path: "a.go", Author: "alice",
			Body:   strings.Repeat("longcomment ", 20),
			Anchor: &domain.Anchor{Path: "a.go", Line: 45, Side: domain.SideRight},
		}},
	}
	m.diffVP.Width = 68
	m.diffVP.Height = 25
	m.renderDiff()

	for i := 0; i < 20; i++ {
		prevY := m.diffVP.YOffset
		prevT := m.contentLineForCursor()
		m.nudgeDiffCursor(1)
		dY := m.diffVP.YOffset - prevY
		dT := m.contentLineForCursor() - prevT
		if dY != dT {
			t.Fatalf("step %d cursor=%d: dY=%d dTarget=%d (desync)", i, m.cursorLine, dY, dT)
		}
		// Crossing a multi-line thread can advance by more than 1 content row; that's OK
		// as long as dY == dT (cursor stays put on screen). Flag huge jumps.
		if dY > 15 {
			t.Fatalf("step %d: huge jump dY=%d", i, dY)
		}
		t.Logf("step %d cursor=%d dY=%d", i, m.cursorLine, dY)
	}
}
