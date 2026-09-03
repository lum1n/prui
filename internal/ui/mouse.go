package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lum1n/prui/internal/diff"
)

const mouseWheelLines = 3

// handleReviewMouse scrolls or clicks the file list / diff under the pointer.
func (m Model) handleReviewMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	// Body sits below the title row.
	if m.contentHeight > 0 && (msg.Y < 1 || msg.Y >= 1+m.contentHeight) {
		return m, nil
	}
	bodyY := msg.Y - 1

	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		delta := mouseWheelLines
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -mouseWheelLines
		}
		if m.leftWidth > 0 && msg.X < m.leftWidth {
			return m, m.scrollFileListBy(delta)
		}
		if m.fileDiff == nil && !(m.showAll && len(m.flat) > 0) {
			return m, nil
		}
		m.pane = paneDiff
		m.nudgeDiffCursor(delta)
		return m, nil

	case tea.MouseButtonLeft:
		if m.leftWidth > 0 && msg.X < m.leftWidth {
			return m, m.clickFileList(bodyY)
		}
		return m.clickDiff(bodyY)
	}
	return m, nil
}

func (m Model) handlePRListMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if m.prList.FilterState() == list.Filtering {
		return m, nil
	}
	if m.contentHeight > 0 && (msg.Y < 1 || msg.Y >= 1+m.contentHeight) {
		return m, nil
	}
	bodyY := msg.Y - 1
	if bodyY < prTabBarHeight {
		return m.clickPRTab(msg.X)
	}
	return m, m.clickPRList(bodyY - prTabBarHeight)
}

func (m Model) clickPRTab(x int) (tea.Model, tea.Cmd) {
	activeStyle := lipgloss.NewStyle().Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().Padding(0, 1)
	cursor := 0
	for i := prListTab(0); i < prListTabCount; i++ {
		if i > 0 {
			cursor += 2 // gap
		}
		var cell string
		if i == m.prTab {
			cell = activeStyle.Render(i.label())
		} else {
			cell = inactiveStyle.Render(i.label())
		}
		w := lipgloss.Width(cell)
		if x >= cursor && x < cursor+w {
			return m.switchPRTab(i)
		}
		cursor += w
	}
	return m, nil
}

func (m *Model) clickPRList(localY int) tea.Cmd {
	titleH := 1
	if !m.prList.ShowTitle() {
		titleH = 0
	}
	row := localY - titleH
	if row < 0 {
		return nil
	}
	itemH := 2 // prDelegate.Height()
	idx := m.prList.Paginator.Page*m.prList.Paginator.PerPage + row/itemH
	items := m.prList.VisibleItems()
	if idx < 0 || idx >= len(items) {
		return nil
	}
	m.prList.Select(idx)
	if item, ok := m.prList.SelectedItem().(prItem); ok {
		m.loading = true
		m.errMsg = ""
		return m.loadReview(item.pr.Ref.Number)
	}
	return nil
}

func (m *Model) clickFileList(bodyY int) tea.Cmd {
	m.pane = paneFiles
	// Inside the bordered panel: skip top border.
	localY := bodyY - 1
	if localY < 0 {
		return nil
	}
	titleH := 1
	if !m.fileList.ShowTitle() && m.fileList.FilterState() != list.Filtering {
		titleH = 0
	}
	row := localY - titleH
	if row < 0 {
		return nil
	}
	idx := m.fileList.Paginator.Page*m.fileList.Paginator.PerPage + row
	items := m.fileList.VisibleItems()
	if idx < 0 || idx >= len(items) {
		return nil
	}
	m.fileList.Select(idx)
	if item, ok := m.fileList.SelectedItem().(fileItem); ok {
		if item.isDir {
			m.toggleDirCollapse(item.dirPath)
			return nil
		}
		if item.file.Path != m.activePath {
			return m.selectFile(item.file.Path)
		}
	}
	return nil
}

func (m Model) clickDiff(bodyY int) (tea.Model, tea.Cmd) {
	if m.fileDiff == nil && !(m.showAll && len(m.flat) > 0) {
		m.pane = paneDiff
		return m, nil
	}
	m.pane = paneDiff
	localY := bodyY - 1 // top border
	if localY < 0 || localY >= m.diffVP.Height {
		return m, nil
	}
	line := localY + m.diffVP.YOffset
	if line < 0 || line >= len(m.diffClickMap) {
		return m, nil
	}
	idx := m.diffClickMap[line]
	if idx < 0 {
		return m, nil
	}
	m.jumpToDiffLine(idx)
	fd, lineIdx, ok := m.diffLineAtCursor()
	if !ok {
		return m, nil
	}
	ln := fd.Lines[lineIdx]
	if diff.ExpandableGap(ln) || ln.Expanded {
		return m.toggleGapAtCursor()
	}
	return m, nil
}

func (m *Model) jumpToDiffLine(idx int) {
	if m.showAll {
		if idx < 0 || idx >= len(m.flat) {
			return
		}
		prevPath := m.flatPath(m.cursorLine)
		m.cursorLine = idx
		if path := m.flatPath(m.cursorLine); path != "" && path != prevPath {
			m.syncFileList(path)
		}
	} else {
		if m.fileDiff == nil || idx < 0 || idx >= len(m.fileDiff.Lines) {
			return
		}
		m.cursorLine = idx
	}
	m.threadTargetID = ""
	m.syncThreadTarget()
	m.renderDiff()
}

func (m *Model) scrollFileListBy(delta int) tea.Cmd {
	n := len(m.fileList.Items())
	if n == 0 {
		return nil
	}
	m.pane = paneFiles
	step := 1
	if delta < 0 {
		step = -1
	}
	notches := delta / mouseWheelLines
	if notches == 0 {
		notches = step
	}
	idx := m.fileList.Index() + notches
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	if idx == m.fileList.Index() {
		return nil
	}
	prevPath := ""
	if item, ok := m.fileList.SelectedItem().(fileItem); ok && item.isFile() {
		prevPath = item.file.Path
	}
	m.fileList.Select(idx)
	if item, ok := m.fileList.SelectedItem().(fileItem); ok && item.isFile() {
		if item.file.Path != prevPath && item.file.Path != m.activePath {
			return m.selectFile(item.file.Path)
		}
	}
	return nil
}

func (m *Model) resetDiffClickMap() {
	m.diffClickMap = m.diffClickMap[:0]
}

// noteDiffLines records click targets for a content fragment.
// Counts newlines so the map stays aligned with viewport strings.Split.
func (m *Model) noteDiffLines(s string, cursorIdx int) {
	n := strings.Count(s, "\n")
	if n == 0 {
		if s == "" {
			return
		}
		n = 1
	}
	for i := 0; i < n; i++ {
		m.diffClickMap = append(m.diffClickMap, cursorIdx)
	}
}

func (m *Model) finalizeDiffClickMap(content string) {
	want := len(strings.Split(content, "\n"))
	for len(m.diffClickMap) < want {
		m.diffClickMap = append(m.diffClickMap, -1)
	}
}
