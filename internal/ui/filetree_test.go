package ui

import (
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestBuildFileTreeItems(t *testing.T) {
	files := []domain.FileChange{
		{Path: "internal/ui/app.go", Status: domain.FileModified},
		{Path: "internal/ui/mouse.go", Status: domain.FileAdded},
		{Path: "README.md", Status: domain.FileModified},
		{Path: "internal/config/config.go", Status: domain.FileModified},
	}
	items := buildFileListItems(files, true, nil, nil, nil, 40)
	var labels []string
	for _, it := range items {
		fi := it.(fileItem)
		labels = append(labels, fi.name)
	}
	// Dirs first under each parent; README at root with dirs.
	want := []string{"internal/", "config/", "config.go", "ui/", "app.go", "mouse.go", "README.md"}
	if len(labels) != len(want) {
		t.Fatalf("got %v want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("got %v want %v", labels, want)
		}
	}
}

func TestBuildFileTreeCollapse(t *testing.T) {
	files := []domain.FileChange{
		{Path: "a/b.go", Status: domain.FileModified},
		{Path: "a/c.go", Status: domain.FileModified},
		{Path: "d.go", Status: domain.FileAdded},
	}
	collapsed := map[string]bool{"a": true}
	items := buildFileListItems(files, true, collapsed, nil, nil, 40)
	var labels []string
	for _, it := range items {
		fi := it.(fileItem)
		labels = append(labels, fi.name)
		if fi.isDir && fi.dirPath == "a" && !fi.collapsed {
			t.Fatal("dir a should be collapsed")
		}
	}
	want := []string{"a/", "d.go"}
	if len(labels) != len(want) {
		t.Fatalf("got %v want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("got %v want %v", labels, want)
		}
	}
}

func TestBuildFileListFlat(t *testing.T) {
	files := []domain.FileChange{
		{Path: "z.go", Status: domain.FileModified},
		{Path: "a.go", Status: domain.FileAdded},
	}
	items := buildFileListItems(files, false, nil, nil, nil, 40)
	if len(items) != 2 {
		t.Fatalf("len=%d", len(items))
	}
	// Flat preserves input order.
	if items[0].(fileItem).file.Path != "z.go" {
		t.Fatalf("%+v", items[0])
	}
}

func TestTreePrefix(t *testing.T) {
	got := treePrefix([]bool{false, true}, false, false)
	if got != "│ └─ " {
		t.Fatalf("%q", got)
	}
	got = treePrefix([]bool{true}, true, true)
	if got != "└─▸ " {
		t.Fatalf("%q", got)
	}
}
