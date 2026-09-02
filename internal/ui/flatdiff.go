package ui

import "github.com/vegard/prui/internal/domain"

// flatRow is one navigable row in the diff pane (file section or diff line).
type flatRow struct {
	header bool
	path   string
	status domain.FileStatus
	fd     *domain.FileDiff
	line   int // index into fd.Lines when !header
}

func buildFlat(files []domain.FileChange, cache map[string]*domain.FileDiff) []flatRow {
	if len(cache) == 0 {
		return nil
	}
	out := make([]flatRow, 0, len(files)*8)
	seen := map[string]bool{}
	appendFile := func(path string, status domain.FileStatus) {
		fd := cache[path]
		if fd == nil || seen[path] {
			return
		}
		seen[path] = true
		if status == "" && fd != nil {
			status = fd.Status
		}
		out = append(out, flatRow{header: true, path: path, status: status, fd: fd})
		if fd == nil {
			return
		}
		for i := range fd.Lines {
			out = append(out, flatRow{path: path, status: status, fd: fd, line: i})
		}
	}
	for _, f := range files {
		appendFile(f.Path, f.Status)
	}
	// Any cached paths not in the file list (defensive).
	for path, fd := range cache {
		if !seen[path] {
			st := domain.FileModified
			if fd != nil {
				st = fd.Status
			}
			appendFile(path, st)
		}
	}
	return out
}

func (m *Model) rebuildFlat() {
	m.flat = buildFlat(m.files, m.diffCache)
}

func (m *Model) flatPath(idx int) string {
	if idx < 0 || idx >= len(m.flat) {
		return ""
	}
	return m.flat[idx].path
}

func (m *Model) syncFileList(path string) {
	if path == "" {
		return
	}
	for i, it := range m.fileList.Items() {
		if fi, ok := it.(fileItem); ok && fi.file.Path == path {
			m.fileList.Select(i)
			break
		}
	}
	m.activePath = path
	if fd := m.diffCache[path]; fd != nil {
		m.fileDiff = fd
	}
}

func (m *Model) jumpFlatToPath(path string) int {
	for i, row := range m.flat {
		if row.path == path && !row.header {
			return i
		}
		if row.path == path && row.header {
			// Prefer first real line after header when present.
			if i+1 < len(m.flat) && m.flat[i+1].path == path && !m.flat[i+1].header {
				return i + 1
			}
			return i
		}
	}
	return 0
}

func (m *Model) moveFlatCursor(delta int) {
	if len(m.flat) == 0 {
		return
	}
	prevPath := m.flatPath(m.cursorLine)
	m.cursorLine += delta
	if m.cursorLine < 0 {
		m.cursorLine = 0
	}
	if m.cursorLine >= len(m.flat) {
		m.cursorLine = len(m.flat) - 1
	}
	// Skip pure headers when moving so j/k land on content when possible.
	if m.flat[m.cursorLine].header {
		next := m.cursorLine + delta
		if delta == 0 {
			next = m.cursorLine + 1
		}
		if next >= 0 && next < len(m.flat) && m.flat[next].path == m.flat[m.cursorLine].path {
			m.cursorLine = next
		}
	}
	path := m.flatPath(m.cursorLine)
	if path != "" && path != prevPath {
		m.syncFileList(path)
	}
}
