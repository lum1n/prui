package ui

import (
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestClickMapAlignsWithViewportLines(t *testing.T) {
	m := Model{
		cursorLine:   10,
		diffWinStart: -1,
		diffWinEnd:   -1,
		fileDiff:     &domain.FileDiff{Path: "a.go", Lines: testDiffLines(40)},
	}
	m.diffVP.Width = 80
	m.diffVP.Height = 12
	m.renderDiff()

	contentLines := strings.Split(m.diffVP.View(), "\n") // this is only visible
	// Peek at total lines via TotalLineCount
	total := m.diffVP.TotalLineCount()
	t.Logf("clickMap=%d totalLineCount=%d Height=%d YOffset=%d", len(m.diffClickMap), total, m.diffVP.Height, m.diffVP.YOffset)

	target := m.contentLineForCursor()
	if target < 0 || target >= total {
		t.Fatalf("target %d out of range [0,%d)", target, total)
	}
	// Map length vs total
	if d := len(m.diffClickMap) - total; d > 1 || d < -1 {
		t.Fatalf("clickMap len %d vs total %d — misaligned", len(m.diffClickMap), total)
	}
	_ = contentLines
}
