package ui

import (
	"testing"

	"github.com/vegard/prui/internal/domain"
)

func TestBuildFlatSections(t *testing.T) {
	files := []domain.FileChange{
		{Path: "a.go", Status: domain.FileModified},
		{Path: "b.go", Status: domain.FileAdded},
	}
	cache := map[string]*domain.FileDiff{
		"a.go": {Path: "a.go", Status: domain.FileModified, Lines: []domain.DiffLine{{Text: "one"}, {Text: "two"}}},
		"b.go": {Path: "b.go", Status: domain.FileAdded, Lines: []domain.DiffLine{{Text: "new"}}},
	}
	flat := buildFlat(files, cache)
	var headers int
	var lines int
	var paths []string
	for _, row := range flat {
		if row.header {
			headers++
			paths = append(paths, row.path)
			continue
		}
		lines++
	}
	if headers != 2 || lines != 3 {
		t.Fatalf("headers=%d lines=%d flat=%d", headers, lines, len(flat))
	}
	if paths[0] != "a.go" || paths[1] != "b.go" {
		t.Fatalf("paths %v", paths)
	}
}