package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lum1n/prui/internal/domain"
)

func TestFileListRowsFitWidth(t *testing.T) {
	fileList := list.New(nil, newFileDelegate(), 40, 20)
	configureList(&fileList, "Files", "file", "files")
	m := Model{
		fileList:     fileList,
		fileTree:     true,
		diffWinStart: -1,
		files: []domain.FileChange{
			{Path: "internal/ui/very_long_file_name_that_should_truncate.go", Status: domain.FileModified},
			{Path: "internal/ui/app.go", Status: domain.FileAdded},
			{Path: "README.md", Status: domain.FileModified},
		},
	}
	m.refreshFileList()
	m.fileList.Select(1)
	view := listViewWithFooter(m.fileList)
	for i, line := range strings.Split(view, "\n") {
		w := ansi.StringWidth(line)
		if w > 40 {
			t.Fatalf("line %d width %d > 40: %q", i, w, line)
		}
	}
	t.Logf("list view height=%d", lipgloss.Height(view))
}
