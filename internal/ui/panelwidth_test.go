package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

func TestFocusedPanelsNeverExceedTermWidth(t *testing.T) {
	lines := testDiffLines(80)
	for i := range lines {
		lines[i].Text = " " + strings.Repeat("x", 60)
	}
	fileList := list.New(nil, newFileDelegate(), 0, 0)
	configureList(&fileList, "Files", "file", "files")
	m := Model{
		screen: screenReview, pane: paneFiles,
		width: 100, height: 30, contentHeight: 27,
		leftWidth: 33, rightWidth: 67,
		cursorLine: 20, diffWinStart: -1, diffWinEnd: -1,
		files:    []domain.FileChange{{Path: "very/long/path/to/some/file_name.go", Status: domain.FileModified}},
		fileDiff: &domain.FileDiff{Path: "very/long/path/to/some/file_name.go", Lines: lines},
		fileList: fileList,
		hl:       diff.NewHighlighter("dark"),
		status:   strings.Repeat("s", 120),
	}
	m.fileList.SetSize(31, 24)
	m.refreshFileList()
	m.diffVP.Width = 65
	m.diffVP.Height = 25
	m.renderDiff()

	for _, p := range []pane{paneFiles, paneDiff} {
		m.pane = p
		raw := m.View()
		for i, line := range strings.Split(raw, "\n") {
			w := ansi.StringWidth(line)
			lw := lipgloss.Width(line)
			if w > m.width || lw > m.width {
				t.Fatalf("pane=%v line %d ansiWidth=%d lipglossWidth=%d term=%d\n%s",
					p, i, w, lw, m.width, ansi.Cut(line, 0, 100))
			}
		}
		if lipgloss.Width(raw) > m.width {
			t.Fatalf("pane=%v frame width %d > %d", p, lipgloss.Width(raw), m.width)
		}
	}
}
