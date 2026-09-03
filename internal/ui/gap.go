package ui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

func gapKey(path string, gapFrom int) string {
	return path + "\x00" + strconv.Itoa(gapFrom)
}

type loadedGapContentMsg struct {
	path    string
	gapFrom int
	content string
	err     error
}

// toggleGapAtCursor expands or collapses the hunk gap under the cursor.
func (m Model) toggleGapAtCursor() (tea.Model, tea.Cmd) {
	fd, lineIdx, ok := m.diffLineAtCursor()
	if !ok {
		return m, nil
	}
	ln := fd.Lines[lineIdx]
	if ln.Expanded {
		return m.collapseGapAt(fd, lineIdx)
	}
	if !diff.ExpandableGap(ln) {
		return m, nil
	}
	return m.beginExpandGap(fd, lineIdx, ln)
}

func (m *Model) diffLineAtCursor() (fd *domain.FileDiff, lineIdx int, ok bool) {
	if m.showAll {
		if m.cursorLine < 0 || m.cursorLine >= len(m.flat) {
			return nil, 0, false
		}
		row := m.flat[m.cursorLine]
		if row.header || row.fd == nil {
			return nil, 0, false
		}
		if row.line < 0 || row.line >= len(row.fd.Lines) {
			return nil, 0, false
		}
		return row.fd, row.line, true
	}
	if m.fileDiff == nil || m.cursorLine < 0 || m.cursorLine >= len(m.fileDiff.Lines) {
		return nil, 0, false
	}
	return m.fileDiff, m.cursorLine, true
}

func (m Model) beginExpandGap(fd *domain.FileDiff, lineIdx int, ln domain.DiffLine) (tea.Model, tea.Cmd) {
	if m.pr == nil || m.pr.HeadSHA == "" {
		m.status = "No head commit to load file content"
		return m, nil
	}
	path := fd.Path
	if cached, ok := m.fileContentCache[path]; ok {
		return m.applyExpandGap(fd, lineIdx, cached)
	}
	m.loading = true
	m.status = "Loading context…"
	gapFrom := ln.GapFrom
	sha := m.pr.HeadSHA
	ref := m.pr.Ref
	prov := m.opts.Provider
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		content, err := prov.GetFileContent(ctx, ref, path, sha)
		return loadedGapContentMsg{path: path, gapFrom: gapFrom, content: content, err: err}
	}
}

func (m Model) applyExpandGap(fd *domain.FileDiff, lineIdx int, content string) (tea.Model, tea.Cmd) {
	cur, header, err := diff.ExpandGap(fd, lineIdx, content)
	if err != nil {
		m.errMsg = err.Error()
		m.loading = false
		return m, nil
	}
	if m.gapHeaders == nil {
		m.gapHeaders = map[string]domain.DiffLine{}
	}
	m.gapHeaders[gapKey(fd.Path, header.GapFrom)] = header
	m.loading = false
	m.errMsg = ""
	m.status = fmt.Sprintf("Expanded %d unchanged lines", header.GapBefore)
	m.syncDiffAfterGapEdit(fd)
	m.cursorLine = m.cursorAfterFileLineChange(fd.Path, cur)
	m.renderDiff()
	return m, nil
}

func (m Model) collapseGapAt(fd *domain.FileDiff, lineIdx int) (tea.Model, tea.Cmd) {
	ln := fd.Lines[lineIdx]
	// Walk to the start of this expanded run.
	start := lineIdx
	for start > 0 && fd.Lines[start-1].Expanded && fd.Lines[start-1].GapFrom == ln.GapFrom {
		start--
	}
	header, ok := m.gapHeaders[gapKey(fd.Path, ln.GapFrom)]
	if !ok {
		header = domain.DiffLine{
			Kind:      domain.LineContext,
			Text:      fmt.Sprintf("@@ -%d +%d @@", ln.GapFrom, ln.GapTo),
			OldNumber: ln.GapFrom,
			NewNumber: ln.GapTo,
			HunkIndex: ln.HunkIndex,
			GapBefore: ln.GapBefore,
			GapFrom:   ln.GapFrom,
			GapTo:     ln.GapTo,
		}
	}
	cur, err := diff.CollapseGap(fd, start, header)
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	delete(m.gapHeaders, gapKey(fd.Path, ln.GapFrom))
	m.status = "Collapsed unchanged lines"
	m.syncDiffAfterGapEdit(fd)
	m.cursorLine = m.cursorAfterFileLineChange(fd.Path, cur)
	m.renderDiff()
	return m, nil
}

func (m *Model) syncDiffAfterGapEdit(fd *domain.FileDiff) {
	if fd == nil {
		return
	}
	m.diffCache[fd.Path] = fd
	if !m.showAll {
		m.fileDiff = fd
	} else {
		m.rebuildFlat()
	}
	m.renderDiff()
}

// cursorAfterFileLineChange maps a line index inside fd to the review cursor
// (flat index when showAll).
func (m Model) cursorAfterFileLineChange(path string, lineIdx int) int {
	if !m.showAll {
		return lineIdx
	}
	for i, row := range m.flat {
		if !row.header && row.path == path && row.line == lineIdx {
			return i
		}
	}
	// Flat not rebuilt yet — approximate after rebuild in caller.
	return m.cursorLine
}

func (m Model) handleLoadedGapContent(msg loadedGapContentMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	if msg.err != nil {
		m.errMsg = msg.err.Error()
		return m, nil
	}
	if m.fileContentCache == nil {
		m.fileContentCache = map[string]string{}
	}
	m.fileContentCache[msg.path] = msg.content

	fd := m.diffCache[msg.path]
	if fd == nil && m.fileDiff != nil && m.fileDiff.Path == msg.path {
		fd = m.fileDiff
	}
	if fd == nil {
		m.errMsg = "file diff no longer loaded"
		return m, nil
	}
	for i, ln := range fd.Lines {
		if diff.ExpandableGap(ln) && ln.GapFrom == msg.gapFrom {
			return m.applyExpandGap(fd, i, msg.content)
		}
	}
	m.status = "Gap already expanded or gone"
	return m, nil
}
