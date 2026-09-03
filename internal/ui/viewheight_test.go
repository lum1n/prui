package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

func TestViewHeightStableAcrossPaneAndScroll(t *testing.T) {
	lines := testDiffLines(200)
	for i := range lines {
		lines[i].Text = " " + strings.Repeat("x", 40)
		lines[i].Anchor = domain.Anchor{Path: "a.go", Line: i + 1, Side: domain.SideRight}
	}
	fileList := list.New(nil, newFileDelegate(), 38, 34)
	configureList(&fileList, "Files", "file", "files")

	m := Model{
		screen:        screenReview,
		pane:          paneFiles,
		width:         120,
		height:        40,
		contentHeight: 37,
		leftWidth:     40,
		rightWidth:    80,
		cursorLine:    80,
		diffWinStart:  -1,
		diffWinEnd:    -1,
		status:        "ready",
		files:         []domain.FileChange{{Path: "a.go", Status: domain.FileModified}},
		fileDiff:      &domain.FileDiff{Path: "a.go", Lines: lines},
		fileList:      fileList,
		hl:            diff.NewHighlighter("dark"),
		comments: []domain.Comment{
			{ID: "1", Path: "a.go", Body: "c1\nextra\nlines\nhere", Anchor: &domain.Anchor{Path: "a.go", Line: 50, Side: domain.SideRight}},
		},
	}
	m.refreshFileList()
	m.diffVP.Width = 78
	m.diffVP.Height = 35
	m.renderDiff()

	check := func(label string) string {
		raw := m.View()
		h := lipgloss.Height(raw)
		if h != m.height {
			t.Fatalf("%s: height %d want %d", label, h, m.height)
		}
		for i, line := range strings.Split(raw, "\n") {
			if w := lipgloss.Width(line); w > m.width {
				t.Fatalf("%s line %d width %d > %d", label, i, w, m.width)
			}
		}
		return raw
	}

	a := check("files")
	m.pane = paneDiff
	b := check("diff")
	if a == b {
		t.Log("views identical (unexpected — help/border should differ)")
	}
	// Diff the frames ignoring ANSI for structural newline alignment.
	as := strings.Split(a, "\n")
	bs := strings.Split(b, "\n")
	if len(as) != len(bs) {
		t.Fatalf("newline count files=%d diff=%d", len(as), len(bs))
	}

	yBefore := m.diffVP.YOffset
	m.pane = paneFiles
	_ = m.View()
	m.pane = paneDiff
	_ = m.View()
	if m.diffVP.YOffset != yBefore {
		t.Fatalf("tab changed YOffset %d → %d", yBefore, m.diffVP.YOffset)
	}

	prevCursorScreen := -1
	for i := 0; i < 15; i++ {
		m.nudgeDiffCursor(1)
		_ = check(fmt.Sprintf("scroll %d", i))
		target := m.contentLineForCursor()
		pos := target - m.diffVP.YOffset
		if prevCursorScreen >= 0 && (pos-prevCursorScreen > 1 || prevCursorScreen-pos > 1) {
			t.Fatalf("scroll %d: cursor screen pos jumped %d → %d (Y=%d target=%d)",
				i, prevCursorScreen, pos, m.diffVP.YOffset, target)
		}
		prevCursorScreen = pos
	}
}
