package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/vegard/prui/internal/config"
	"github.com/vegard/prui/internal/diff"
	"github.com/vegard/prui/internal/domain"
	"github.com/vegard/prui/internal/provider"
	"github.com/vegard/prui/internal/review"
)

type screen int

const (
	screenPRList screen = iota
	screenReview
	screenSubmit
	screenHelp
	screenBody
)

type pane int

const (
	paneFiles pane = iota
	paneDiff
)

// Options configures the TUI entry.
type Options struct {
	Config   *config.Config
	Host     domain.Host
	Provider provider.Host
	Repo     domain.RepoRef
	PRNumber int // 0 = show list
}

type prItem struct {
	pr domain.PullRequest
}

func (i prItem) Title() string       { return fmt.Sprintf("#%d  %s", i.pr.Ref.Number, i.pr.Title) }
func (i prItem) Description() string { return fmt.Sprintf("%s · %s", i.pr.Author, i.pr.State) }
func (i prItem) FilterValue() string { return i.Title() + " " + i.Description() }

type fileItem struct {
	file     domain.FileChange
	drafts   int
	comments int
}

func (i fileItem) Title() string {
	mark := " "
	switch i.file.Status {
	case domain.FileAdded:
		mark = "+"
	case domain.FileRemoved:
		mark = "-"
	case domain.FileRenamed:
		mark = "→"
	}
	extra := ""
	if i.drafts > 0 || i.comments > 0 {
		extra = fmt.Sprintf("  [%d/%d]", i.drafts, i.comments)
	}
	return mark + " " + i.file.Path + extra
}
func (i fileItem) Description() string { return string(i.file.Status) }
func (i fileItem) FilterValue() string { return i.file.Path }

type Model struct {
	opts   Options
	screen screen
	pane   pane
	width  int
	height int

	prList   list.Model
	fileList list.Model
	diffVP   viewport.Model
	comment  textarea.Model

	prs      []domain.PullRequest
	files    []domain.FileChange
	comments []domain.Comment
	pr       *domain.PullRequest
	fileDiff *domain.FileDiff
	session  *review.Session

	cursorLine int
	rangeStart int // -1 = no visual range
	hl         *diff.Highlighter
	status     string
	errMsg     string
	submitIdx  int
	helpText   string
	loading    bool
	bodyVP     viewport.Model
	splitDiff  bool
	showAll    bool // all files in one scrollable view

	commenting    bool
	editingID     string // non-empty while editing an existing draft
	activePath    string // last requested / shown file path
	leftWidth     int
	rightWidth    int
	contentHeight int

	diffCache map[string]*domain.FileDiff
	flat      []flatRow
	allGen    int // bumps to cancel in-flight all-file loads
}

type loadedPRsMsg struct {
	prs []domain.PullRequest
	err error
}
type loadedReviewMsg struct {
	pr       *domain.PullRequest
	files    []domain.FileChange
	comments []domain.Comment
	session  *review.Session
	err      error
}
type loadedDiffMsg struct {
	path string
	fd   *domain.FileDiff
	err  error
}
type loadedAllDiffsMsg struct {
	gen   int
	diffs map[string]*domain.FileDiff
	err   error
}
type submitDoneMsg struct{ err error }

// NewModel creates the root TUI model.
func NewModel(opts Options) Model {
	delegate := list.NewDefaultDelegate()
	prList := list.New(nil, delegate, 0, 0)
	prList.Title = "Pull requests"
	prList.SetShowStatusBar(true)
	prList.SetFilteringEnabled(true)

	fileList := list.New(nil, delegate, 0, 0)
	fileList.Title = "Files"
	fileList.SetShowHelp(false)
	fileList.SetFilteringEnabled(true)

	ta := textarea.New()
	ta.Placeholder = "Comment on this line… (enter save, esc cancel)"
	ta.ShowLineNumbers = false
	ta.CharLimit = 4000
	ta.SetHeight(3)
	ta.Prompt = "▎ "

	theme := "dark"
	if opts.Config != nil {
		theme = opts.Config.UI.Theme
	}

	m := Model{
		opts:       opts,
		screen:     screenPRList,
		pane:       paneFiles,
		prList:     prList,
		fileList:   fileList,
		diffVP:     viewport.New(0, 0),
		bodyVP:     viewport.New(0, 0),
		comment:    ta,
		rangeStart: -1,
		hl:         diff.NewHighlighter(theme),
		helpText:   defaultHelp,
		loading:    true,
		splitDiff:  opts.Config != nil && opts.Config.UI.Diff == "split",
		showAll:    opts.Config != nil && opts.Config.UI.Files == "all",
		diffCache:  map[string]*domain.FileDiff{},
	}
	if opts.PRNumber > 0 {
		m.screen = screenReview
	}
	return m
}

const defaultHelp = `Keys
  j/k       move
  ctrl+d/u  page down / page up (half screen)
  tab       switch pane
  enter     open PR / load file
  c         new comment on line
  e         edit draft on line
  x         delete draft on line
  v         toggle range select
  r         submit review
  p         PR description (markdown)
  d         toggle unified/split layout
  a         toggle this-file / all-files view
  [ ]       prev/next hunk
  o         show PR URL in status
  O         open PR in browser
  ?         help
  q/esc     back / quit

Inline comment: enter save, esc cancel

Views
  this-file   only the selected path (default)
  all-files   every path in one scroll; file list + section headers track focus`

func (m Model) Init() tea.Cmd {
	if m.opts.PRNumber > 0 {
		return m.loadReview(m.opts.PRNumber)
	}
	return m.loadPRs()
}

func (m Model) loadPRs() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		prs, err := m.opts.Provider.ListPullRequests(ctx, m.opts.Repo, domain.ListOpts{State: "open"})
		return loadedPRsMsg{prs: prs, err: err}
	}
}

func (m Model) loadReview(num int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ref := domain.PRRef{Repo: m.opts.Repo, Number: num}
		pr, err := m.opts.Provider.GetPullRequest(ctx, ref)
		if err != nil {
			return loadedReviewMsg{err: err}
		}
		files, err := m.opts.Provider.ListFiles(ctx, ref)
		if err != nil {
			return loadedReviewMsg{err: err}
		}
		comments, _ := m.opts.Provider.ListComments(ctx, ref)
		session, _ := review.Load(m.opts.Host.Name, ref)
		if remote, err := m.opts.Provider.StartReview(ctx, ref); err == nil && remote != nil {
			session.Draft.RemoteID = remote.RemoteID
		}
		return loadedReviewMsg{pr: pr, files: files, comments: comments, session: session}
	}
}

func (m Model) loadDiff(path string) tea.Cmd {
	if path == "" || m.pr == nil {
		return nil
	}
	num := m.pr.Ref.Number
	repo := m.opts.Repo
	prov := m.opts.Provider
	return func() tea.Msg {
		ctx := context.Background()
		ref := domain.PRRef{Repo: repo, Number: num}
		fd, err := prov.GetFileDiff(ctx, ref, path)
		return loadedDiffMsg{path: path, fd: fd, err: err}
	}
}

func (m Model) loadAllDiffs(gen int) tea.Cmd {
	if m.pr == nil || len(m.files) == 0 {
		return nil
	}
	num := m.pr.Ref.Number
	repo := m.opts.Repo
	prov := m.opts.Provider
	files := append([]domain.FileChange(nil), m.files...)
	return func() tea.Msg {
		ctx := context.Background()
		ref := domain.PRRef{Repo: repo, Number: num}
		out := make(map[string]*domain.FileDiff, len(files))
		var firstErr error
		for _, f := range files {
			fd, err := prov.GetFileDiff(ctx, ref, f.Path)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			out[f.Path] = fd
		}
		return loadedAllDiffsMsg{gen: gen, diffs: out, err: firstErr}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case loadedPRsMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.prs = msg.prs
		items := make([]list.Item, len(msg.prs))
		for i, p := range msg.prs {
			items[i] = prItem{pr: p}
		}
		m.prList.SetItems(items)
		m.status = fmt.Sprintf("%d open PRs", len(msg.prs))
		return m, nil

	case loadedReviewMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.pr = msg.pr
		m.files = msg.files
		m.comments = msg.comments
		m.session = msg.session
		m.screen = screenReview
		m.diffCache = map[string]*domain.FileDiff{}
		m.flat = nil
		m.refreshFileList()
		m.status = fmt.Sprintf("PR #%d — %s", m.pr.Ref.Number, m.pr.Title)
		if len(m.files) == 0 {
			return m, nil
		}
		m.activePath = m.files[0].Path
		m.loading = true
		if m.showAll {
			m.allGen++
			return m, m.loadAllDiffs(m.allGen)
		}
		return m, m.loadDiff(m.files[0].Path)

	case loadedDiffMsg:
		m.loading = false
		// Ignore stale responses from a previous selection (single-file mode).
		if !m.showAll && msg.path != "" && m.activePath != "" && msg.path != m.activePath {
			return m, nil
		}
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.errMsg = ""
		if msg.fd != nil && msg.path != "" {
			m.diffCache[msg.path] = msg.fd
		}
		m.fileDiff = msg.fd
		m.activePath = msg.path
		if m.showAll {
			m.rebuildFlat()
			m.cursorLine = m.jumpFlatToPath(msg.path)
			m.syncFileList(msg.path)
		} else {
			m.cursorLine = firstCommentable(msg.fd)
		}
		if msg.fd != nil && len(msg.fd.Lines) == 0 {
			m.status = msg.path + " (no textual diff — binary or empty)"
		} else if m.showAll {
			m.status = fmt.Sprintf("all files · %s", msg.path)
		} else {
			m.status = msg.path
		}
		m.rangeStart = -1
		m.commenting = false
		m.editingID = ""
		m.layout()
		m.renderDiff()
		return m, nil

	case loadedAllDiffsMsg:
		if msg.gen != m.allGen || !m.showAll {
			return m, nil
		}
		m.loading = false
		if msg.err != nil && len(msg.diffs) == 0 {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.errMsg = ""
		for p, fd := range msg.diffs {
			m.diffCache[p] = fd
		}
		m.rebuildFlat()
		path := m.activePath
		if path == "" && len(m.files) > 0 {
			path = m.files[0].Path
		}
		m.cursorLine = m.jumpFlatToPath(path)
		m.syncFileList(path)
		m.status = fmt.Sprintf("all files · %d/%d loaded", len(msg.diffs), len(m.files))
		if msg.err != nil {
			m.status += " · some failed"
		}
		m.rangeStart = -1
		m.layout()
		m.renderDiff()
		return m, nil

	case submitDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.screen = screenReview
			return m, nil
		}
		_ = review.Clear(m.opts.Host.Name, domain.PRRef{Repo: m.opts.Repo, Number: m.pr.Ref.Number})
		m.session, _ = review.Load(m.opts.Host.Name, domain.PRRef{Repo: m.opts.Repo, Number: m.pr.Ref.Number})
		m.status = "Review submitted"
		m.screen = screenReview
		return m, m.loadReview(m.pr.Ref.Number)
	}

	switch m.screen {
	case screenPRList:
		return m.updatePRList(msg)
	case screenReview:
		return m.updateReview(msg)
	case screenSubmit:
		return m.updateSubmit(msg)
	case screenHelp:
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "q" || key.String() == "esc" || key.String() == "?" {
				m.screen = screenReview
				if m.pr == nil {
					m.screen = screenPRList
				}
			}
		}
		return m, nil
	case screenBody:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "q", "esc", "p":
				m.screen = screenReview
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.bodyVP, cmd = m.bodyVP.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updatePRList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if item, ok := m.prList.SelectedItem().(prItem); ok {
				m.loading = true
				m.errMsg = ""
				return m, m.loadReview(item.pr.Ref.Number)
			}
		case "O":
			if item, ok := m.prList.SelectedItem().(prItem); ok {
				if item.pr.URL == "" {
					m.status = "No PR URL"
					return m, nil
				}
				if err := openBrowser(item.pr.URL); err != nil {
					m.status = "Open failed: " + err.Error()
					return m, nil
				}
				m.status = "Opened in browser"
				return m, nil
			}
		case "?":
			m.screen = screenHelp
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.prList, cmd = m.prList.Update(msg)
	return m, cmd
}

func (m Model) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.commenting {
		return m.updateInlineComment(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			if m.opts.PRNumber > 0 {
				return m, tea.Quit
			}
			m.screen = screenPRList
			m.pr = nil
			m.fileDiff = nil
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.rangeStart = -1
			m.errMsg = ""
			return m, nil
		case "tab":
			if m.pane == paneFiles {
				m.pane = paneDiff
			} else {
				m.pane = paneFiles
			}
			return m, nil
		case "?":
			m.screen = screenHelp
			return m, nil
		case "p":
			m.showBody()
			m.screen = screenBody
			return m, nil
		case "d":
			m.splitDiff = !m.splitDiff
			m.renderDiff()
			if m.splitDiff {
				m.status = "Split layout"
			} else {
				m.status = "Unified layout"
			}
			return m, nil
		case "a":
			m.showAll = !m.showAll
			m.rangeStart = -1
			m.commenting = false
			m.editingID = ""
			if m.showAll {
				m.status = "All-files view"
				m.loading = true
				m.allGen++
				return m, m.loadAllDiffs(m.allGen)
			}
			m.status = "This-file view"
			m.flat = nil
			path := m.activePath
			if path == "" && len(m.files) > 0 {
				path = m.files[0].Path
			}
			if path == "" {
				return m, nil
			}
			if fd, ok := m.diffCache[path]; ok {
				m.fileDiff = fd
				m.cursorLine = firstCommentable(fd)
				m.layout()
				m.renderDiff()
				return m, nil
			}
			m.loading = true
			return m, m.selectFile(path)
		case "o":
			if m.pr != nil {
				m.status = m.pr.URL
			}
			return m, nil
		case "O":
			if m.pr == nil || m.pr.URL == "" {
				m.status = "No PR URL"
				return m, nil
			}
			if err := openBrowser(m.pr.URL); err != nil {
				m.status = "Open failed: " + err.Error()
				return m, nil
			}
			m.status = "Opened in browser"
			return m, nil
		case "r":
			m.screen = screenSubmit
			m.submitIdx = 0
			return m, nil
		case "c":
			if !m.cursorCommentable() {
				m.status = "Select a commentable line"
				return m, nil
			}
			return m, m.beginComment("")
		case "e":
			d := m.draftOnCursor()
			if d == nil {
				m.status = "No draft on this line"
				return m, nil
			}
			return m, m.beginComment(d.ID)
		case "x":
			d := m.draftOnCursor()
			if d == nil {
				m.status = "No draft on this line"
				return m, nil
			}
			_ = m.session.RemoveComment(d.ID)
			m.refreshFileList()
			m.status = "Draft deleted"
			m.renderDiff()
			return m, nil
		case "v":
			if m.rangeStart < 0 {
				m.rangeStart = m.cursorLine
				m.status = "Range start set — move and press c"
			} else {
				m.rangeStart = -1
				m.status = "Range cleared"
			}
			m.renderDiff()
			return m, nil
		}

		if m.pane == paneDiff && (m.fileDiff != nil || (m.showAll && len(m.flat) > 0)) {
			switch msg.String() {
			case "j", "down":
				m.moveCursor(1)
				return m, nil
			case "k", "up":
				m.moveCursor(-1)
				return m, nil
			case "ctrl+d", "pgdown":
				m.pageCursor(1)
				return m, nil
			case "ctrl+u", "pgup":
				m.pageCursor(-1)
				return m, nil
			case "]":
				m.jumpHunk(1)
				return m, nil
			case "[":
				m.jumpHunk(-1)
				return m, nil
			case "g":
				if m.showAll {
					m.cursorLine = 0
					m.moveFlatCursor(0)
				} else {
					m.cursorLine = firstCommentable(m.fileDiff)
				}
				m.renderDiff()
				return m, nil
			case "G":
				if m.showAll && len(m.flat) > 0 {
					m.cursorLine = len(m.flat) - 1
					m.syncFileList(m.flatPath(m.cursorLine))
				} else {
					m.cursorLine = lastCommentable(m.fileDiff)
				}
				m.renderDiff()
				return m, nil
			}
		}

		if m.pane == paneFiles {
			prevPath := ""
			if item, ok := m.fileList.SelectedItem().(fileItem); ok {
				prevPath = item.file.Path
			}
			switch msg.String() {
			case "enter", "l":
				if item, ok := m.fileList.SelectedItem().(fileItem); ok {
					return m, m.selectFile(item.file.Path)
				}
			}
			var cmd tea.Cmd
			m.fileList, cmd = m.fileList.Update(msg)
			if item, ok := m.fileList.SelectedItem().(fileItem); ok {
				if item.file.Path != prevPath && item.file.Path != m.activePath {
					return m, tea.Batch(cmd, m.selectFile(item.file.Path))
				}
			}
			return m, cmd
		}
	}
	return m, nil
}

func (m *Model) selectFile(path string) tea.Cmd {
	if path == "" {
		return nil
	}
	if m.showAll {
		if _, ok := m.diffCache[path]; ok {
			m.rebuildFlat()
			m.cursorLine = m.jumpFlatToPath(path)
			m.syncFileList(path)
			m.pane = paneDiff
			m.status = fmt.Sprintf("all files · %s", path)
			m.renderDiff()
			return nil
		}
		m.activePath = path
		m.loading = true
		m.status = "Loading " + path + "…"
		return m.loadDiff(path)
	}
	if path == m.activePath && m.fileDiff != nil && m.errMsg == "" && !m.loading {
		return nil
	}
	m.activePath = path
	m.loading = true
	m.errMsg = ""
	m.commenting = false
	m.editingID = ""
	m.status = "Loading " + path + "…"
	return m.loadDiff(path)
}

// draftOnCursor returns the last draft anchored on the cursor line, if any.
func (m *Model) draftOnCursor() *domain.DraftComment {
	if m.session == nil {
		return nil
	}
	row, ok := m.cursorRow()
	if !ok || row.header || row.fd == nil || row.line < 0 || row.line >= len(row.fd.Lines) {
		return nil
	}
	ln := row.fd.Lines[row.line]
	var found *domain.DraftComment
	for i := range m.session.Draft.Comments {
		d := &m.session.Draft.Comments[i]
		if d.Path != row.path || d.Anchor == nil {
			continue
		}
		if d.Anchor.Line == ln.Anchor.Line && (d.Anchor.Side == "" || d.Anchor.Side == ln.Anchor.Side) {
			found = d
		}
	}
	return found
}

func (m *Model) beginComment(editID string) tea.Cmd {
	m.pane = paneDiff
	m.editingID = editID
	if editID != "" && m.session != nil {
		for _, d := range m.session.Draft.Comments {
			if d.ID == editID {
				m.comment.SetValue(d.Body)
				break
			}
		}
		m.status = "Editing draft — enter save, esc cancel"
	} else {
		m.comment.SetValue("")
		m.status = "Commenting — enter save, esc cancel"
	}
	m.comment.Focus()
	m.commenting = true
	m.layout()
	return textarea.Blink
}

func (m Model) updateInlineComment(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.commenting = false
			m.editingID = ""
			m.comment.Blur()
			m.layout()
			m.status = "Comment cancelled"
			return m, nil
		case "enter", "ctrl+s":
			body := strings.TrimSpace(m.comment.Value())
			if body == "" {
				m.status = "Empty comment"
				return m, nil
			}
			if m.session == nil {
				m.status = "No review session"
				return m, nil
			}
			path := ""
			if m.fileDiff != nil {
				path = m.fileDiff.Path
			}
			if m.editingID != "" {
				if err := m.session.UpdateComment(m.editingID, body); err != nil {
					m.status = err.Error()
					return m, nil
				}
				m.status = "Draft updated"
			} else {
				anchor := m.selectedAnchor()
				_ = m.session.AddComment(domain.DraftComment{
					Body:   body,
					Path:   path,
					Anchor: anchor,
				})
				m.status = "Draft comment added"
			}
			m.refreshFileList()
			m.rangeStart = -1
			m.commenting = false
			m.editingID = ""
			m.comment.Blur()
			m.layout()
			m.renderDiff()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.comment, cmd = m.comment.Update(msg)
	return m, cmd
}

func (m Model) updateSubmit(msg tea.Msg) (tea.Model, tea.Cmd) {
	actions := m.availableActions()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.screen = screenReview
			return m, nil
		case "j", "down":
			m.submitIdx = (m.submitIdx + 1) % len(actions)
		case "k", "up":
			m.submitIdx = (m.submitIdx - 1 + len(actions)) % len(actions)
		case "enter":
			action := actions[m.submitIdx]
			m.session.Draft.Action = action
			m.loading = true
			draft := m.session.Draft
			return m, func() tea.Msg {
				ctx := context.Background()
				ref := domain.PRRef{Repo: m.opts.Repo, Number: m.pr.Ref.Number}
				err := m.opts.Provider.SubmitReview(ctx, ref, draft)
				return submitDoneMsg{err: err}
			}
		}
	}
	return m, nil
}

func (m Model) availableActions() []domain.ReviewAction {
	acts := []domain.ReviewAction{domain.ActionComment, domain.ActionApprove}
	if provider.SupportsRequestChanges(m.opts.Host.Kind) {
		acts = append(acts, domain.ActionRequestChanges)
	}
	return acts
}

func (m *Model) cursorRow() (flatRow, bool) {
	if m.showAll {
		if m.cursorLine < 0 || m.cursorLine >= len(m.flat) {
			return flatRow{}, false
		}
		return m.flat[m.cursorLine], true
	}
	if m.fileDiff == nil || m.cursorLine < 0 || m.cursorLine >= len(m.fileDiff.Lines) {
		return flatRow{}, false
	}
	return flatRow{fd: m.fileDiff, path: m.fileDiff.Path, line: m.cursorLine}, true
}

func (m *Model) cursorCommentable() bool {
	row, ok := m.cursorRow()
	if !ok || row.header || row.fd == nil {
		return false
	}
	if row.line < 0 || row.line >= len(row.fd.Lines) {
		return false
	}
	return diff.Commentable(row.fd.Lines[row.line])
}

func (m *Model) selectedAnchor() *domain.Anchor {
	row, ok := m.cursorRow()
	if !ok || row.header || row.fd == nil || row.line < 0 || row.line >= len(row.fd.Lines) {
		return nil
	}
	line := row.fd.Lines[row.line]
	a := line.Anchor
	if m.rangeStart < 0 || m.rangeStart == m.cursorLine {
		return &a
	}

	// Resolve start/end rows across this-file and all-files views.
	startIdx, endIdx := m.rangeStart, m.cursorLine
	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
	}
	var startLine, endLine domain.DiffLine
	if m.showAll {
		if startIdx < 0 || endIdx >= len(m.flat) {
			return &a
		}
		sr, er := m.flat[startIdx], m.flat[endIdx]
		if sr.header || er.header || sr.fd == nil || er.fd == nil || sr.path != er.path {
			// Range must stay within one file.
			return &a
		}
		startLine = sr.fd.Lines[sr.line]
		endLine = er.fd.Lines[er.line]
	} else {
		if m.fileDiff == nil || startIdx < 0 || endIdx >= len(m.fileDiff.Lines) {
			return &a
		}
		startLine = m.fileDiff.Lines[startIdx]
		endLine = m.fileDiff.Lines[endIdx]
	}
	a.Line = startLine.Anchor.Line
	a.EndLine = endLine.Anchor.Line
	if a.EndLine < a.Line {
		a.Line, a.EndLine = a.EndLine, a.Line
	}
	// Prefer the side of the end (cursor) line when mixed; keep start side if empty.
	if a.Side == "" {
		a.Side = startLine.Anchor.Side
	}
	return &a
}

func anchorHint(a *domain.Anchor) string {
	if a == nil || a.Line <= 0 {
		return ""
	}
	if a.EndLine > a.Line {
		return fmt.Sprintf(" lines %d–%d", a.Line, a.EndLine)
	}
	return fmt.Sprintf(" line %d", a.Line)
}

func (m *Model) moveCursor(delta int) {
	if m.showAll {
		if len(m.flat) == 0 {
			return
		}
		m.moveFlatCursor(delta)
		m.renderDiff()
		return
	}
	if m.fileDiff == nil || len(m.fileDiff.Lines) == 0 {
		return
	}
	n := len(m.fileDiff.Lines)
	for i := 0; i < n; i++ {
		m.cursorLine = (m.cursorLine + delta + n) % n
		if diff.Commentable(m.fileDiff.Lines[m.cursorLine]) || strings.HasPrefix(m.fileDiff.Lines[m.cursorLine].Text, "@@") {
			break
		}
	}
	m.renderDiff()
}

// pageCursor jumps by half the viewport height (vim ctrl+d / ctrl+u).
func (m *Model) pageCursor(dir int) {
	step := m.diffVP.Height / 2
	if step < 1 {
		step = 10
	}
	delta := dir * step
	if m.showAll {
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
		if path := m.flatPath(m.cursorLine); path != "" && path != prevPath {
			m.syncFileList(path)
		}
		m.renderDiff()
		return
	}
	if m.fileDiff == nil || len(m.fileDiff.Lines) == 0 {
		return
	}
	m.cursorLine += delta
	if m.cursorLine < 0 {
		m.cursorLine = 0
	}
	if m.cursorLine >= len(m.fileDiff.Lines) {
		m.cursorLine = len(m.fileDiff.Lines) - 1
	}
	m.renderDiff()
}

func (m *Model) jumpHunk(dir int) {
	if m.showAll {
		if len(m.flat) == 0 {
			return
		}
		start := m.cursorLine
		for i := start + dir; i >= 0 && i < len(m.flat); i += dir {
			row := m.flat[i]
			if row.header || row.fd == nil {
				continue
			}
			ln := row.fd.Lines[row.line]
			if strings.HasPrefix(ln.Text, "@@") {
				m.cursorLine = i
				m.syncFileList(row.path)
				m.renderDiff()
				return
			}
		}
		return
	}
	if m.fileDiff == nil || m.cursorLine < 0 || m.cursorLine >= len(m.fileDiff.Lines) {
		return
	}
	cur := m.fileDiff.Lines[m.cursorLine].HunkIndex
	target := cur + dir
	for i, ln := range m.fileDiff.Lines {
		if ln.HunkIndex == target && strings.HasPrefix(ln.Text, "@@") {
			m.cursorLine = i
			m.renderDiff()
			return
		}
	}
}

func (m *Model) refreshFileList() {
	items := make([]list.Item, len(m.files))
	draftCounts := map[string]int{}
	commentCounts := map[string]int{}
	if m.session != nil {
		for _, d := range m.session.Draft.Comments {
			draftCounts[d.Path]++
		}
	}
	for _, c := range m.comments {
		if c.Path != "" {
			commentCounts[c.Path]++
		}
	}
	for i, f := range m.files {
		items[i] = fileItem{file: f, drafts: draftCounts[f.Path], comments: commentCounts[f.Path]}
	}
	m.fileList.SetItems(items)
}

func (m *Model) layout() {
	titleH := 1
	statusH := 1
	contentH := m.height - titleH - statusH
	if contentH < 3 {
		contentH = 3
	}
	m.contentHeight = contentH
	m.prList.SetSize(m.width, contentH)

	leftW := m.width / 3
	if leftW < 24 {
		leftW = 24
	}
	if leftW > 48 {
		leftW = 48
	}
	rightW := m.width - leftW
	if rightW < 20 {
		rightW = 20
	}
	m.leftWidth = leftW
	m.rightWidth = rightW

	commentH := 0
	if m.commenting {
		commentH = 5 // label + textarea
	}
	diffH := contentH - commentH
	if diffH < 3 {
		diffH = 3
	}

	m.fileList.SetSize(leftW-2, contentH-2) // account for panel border later
	m.diffVP.Width = rightW - 2
	m.diffVP.Height = diffH - 2
	m.bodyVP.Width = m.width
	m.bodyVP.Height = contentH
	m.comment.SetWidth(rightW - 4)
	m.comment.SetHeight(3)
	m.renderDiff()
}

func (m *Model) showBody() {
	raw := ""
	if m.pr != nil {
		raw = m.pr.Body
	}
	if raw == "" {
		raw = "_No description_"
	}
	rendered := renderMarkdown(raw, m.width-2)
	m.bodyVP.SetContent(rendered)
	m.bodyVP.GotoTop()
}

func renderMarkdown(src string, width int) string {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return src
	}
	out, err := r.Render(src)
	if err != nil {
		return src
	}
	return out
}

func (m *Model) renderDiff() {
	width := m.diffVP.Width
	if width <= 0 {
		width = 80
	}
	th := diff.ThemeFor("dark")
	if m.opts.Config != nil {
		th = diff.ThemeFor(m.opts.Config.UI.Theme)
	}

	if m.showAll {
		m.renderFlatDiff(th, width)
		return
	}
	if m.fileDiff == nil {
		m.diffVP.SetContent("  select a file")
		return
	}

	var b strings.Builder
	b.WriteString(diff.PaintFileHeader(m.fileDiff.Path, m.fileDiff.Status, th, width))
	b.WriteByte('\n')

	start, end := 0, len(m.fileDiff.Lines)
	const margin = 200
	if len(m.fileDiff.Lines) > margin*2 {
		start = m.cursorLine - margin
		if start < 0 {
			start = 0
		}
		end = m.cursorLine + margin
		if end > len(m.fileDiff.Lines) {
			end = len(m.fileDiff.Lines)
		}
		if start > 0 {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d lines above ···", start)) + "\n")
		}
	}
	for i := start; i < end; i++ {
		m.writeDiffLine(&b, m.fileDiff, m.fileDiff.Lines[i], i == m.cursorLine || m.lineInRange(i), th, width)
	}
	if end < len(m.fileDiff.Lines) {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d lines below ···", len(m.fileDiff.Lines)-end)) + "\n")
	}
	m.diffVP.SetContent(b.String())
	m.diffVP.GotoTop()
	// File header is 3 lines (rules + title).
	if start == 0 {
		offset := m.cursorLine + 3 - m.diffVP.Height/2
		if offset > 0 {
			m.diffVP.LineDown(offset)
		}
	}
}

func (m *Model) renderFlatDiff(th diff.Theme, width int) {
	if len(m.flat) == 0 {
		m.diffVP.SetContent("  loading files…")
		return
	}
	var b strings.Builder
	start, end := 0, len(m.flat)
	const margin = 200
	if len(m.flat) > margin*2 {
		start = m.cursorLine - margin
		if start < 0 {
			start = 0
		}
		end = m.cursorLine + margin
		if end > len(m.flat) {
			end = len(m.flat)
		}
		if start > 0 {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d rows above ···", start)) + "\n")
		}
	}
	for i := start; i < end; i++ {
		row := m.flat[i]
		sel := i == m.cursorLine || m.lineInRange(i)
		if row.header {
			hdr := diff.PaintFileHeader(row.path, row.status, th, width)
			if sel {
				hdr = lipgloss.NewStyle().Background(th.SelectedBg).Render(hdr)
			}
			b.WriteString(hdr)
			b.WriteByte('\n')
			continue
		}
		if row.fd == nil || row.line < 0 || row.line >= len(row.fd.Lines) {
			continue
		}
		m.writeDiffLine(&b, row.fd, row.fd.Lines[row.line], sel, th, width)
	}
	if end < len(m.flat) {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d rows below ···", len(m.flat)-end)) + "\n")
	}
	m.diffVP.SetContent(b.String())
	m.diffVP.GotoTop()
	if start == 0 {
		offset := m.cursorLine - m.diffVP.Height/2
		if offset > 0 {
			m.diffVP.LineDown(offset)
		}
	}
}

func (m *Model) lineInRange(i int) bool {
	if m.rangeStart < 0 {
		return false
	}
	lo, hi := m.rangeStart, m.cursorLine
	if lo > hi {
		lo, hi = hi, lo
	}
	return i >= lo && i <= hi
}

func (m *Model) writeDiffLine(b *strings.Builder, fd *domain.FileDiff, ln domain.DiffLine, sel bool, th diff.Theme, width int) {
	opt := diff.Options{Theme: th, Width: width, Selected: sel, Split: m.splitDiff}
	b.WriteString(diff.Paint(m.hl, fd.Path, ln, opt))
	b.WriteByte('\n')
	for _, c := range m.comments {
		if c.Anchor != nil && c.Path == fd.Path && c.Anchor.Line == ln.Anchor.Line && c.Anchor.Side == ln.Anchor.Side {
			b.WriteString(diff.PaintAnnotation(c.Author, c.Body, false, th, width) + "\n")
		}
	}
	if m.session != nil {
		for _, d := range m.session.Draft.Comments {
			if d.Anchor != nil && d.Path == fd.Path && d.Anchor.Line == ln.Anchor.Line {
				b.WriteString(diff.PaintAnnotation("you", d.Body, true, th, width) + "\n")
			}
		}
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	title := titleStyle.Render("prui") + "  " + mutedStyle.Render(fmt.Sprintf("%s/%s", m.opts.Repo.Owner, m.opts.Repo.Name))
	if m.loading {
		title += mutedStyle.Render("  …")
	}

	var body string
	switch m.screen {
	case screenPRList:
		body = m.prList.View()
	case screenReview:
		leftInner := m.fileList.View()
		rightInner := m.diffVP.View()
		if m.commenting {
			label := "✎ comment" + anchorHint(m.selectedAnchor()) + "  (enter save · esc cancel)"
			if m.editingID != "" {
				label = "✎ edit draft" + anchorHint(m.selectedAnchor()) + "  (enter save · esc cancel)"
			}
			box := lipgloss.JoinVertical(lipgloss.Left,
				draftStyle.Render(label),
				m.comment.View(),
			)
			rightInner = lipgloss.JoinVertical(lipgloss.Left, rightInner, box)
		}
		lw, rw := m.leftWidth, m.rightWidth
		if lw == 0 {
			lw = m.width / 3
		}
		if rw == 0 {
			rw = m.width - lw
		}
		var left, right string
		if m.pane == paneFiles && !m.commenting {
			left = focusedPanel.Width(lw).Render(leftInner)
			right = panel.Width(rw).Render(rightInner)
		} else {
			left = panel.Width(lw).Render(leftInner)
			right = focusedPanel.Width(rw).Render(rightInner)
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	case screenSubmit:
		var lines []string
		lines = append(lines, titleStyle.Render("Submit review"), "")
		if !provider.SupportsRequestChanges(m.opts.Host.Kind) {
			lines = append(lines, mutedStyle.Render("Note: this host has no request-changes event; comments will still post."), "")
		}
		n := 0
		if m.session != nil {
			n = len(m.session.Draft.Comments)
		}
		lines = append(lines, fmt.Sprintf("%d draft comment(s)", n), "")
		for i, a := range m.availableActions() {
			prefix := "  "
			if i == m.submitIdx {
				prefix = "> "
			}
			lines = append(lines, prefix+string(a))
		}
		lines = append(lines, "", mutedStyle.Render("enter submit · esc cancel"))
		body = strings.Join(lines, "\n")
	case screenHelp:
		body = m.helpText
	case screenBody:
		body = m.bodyVP.View()
	}

	status := m.status
	if m.errMsg != "" {
		status = errorStyle.Render(m.errMsg)
	}
	statusBar := statusStyle.Width(m.width).Render(truncate(status, m.width))
	contentH := m.height - 2
	if contentH < 1 {
		contentH = 1
	}
	body = lipgloss.NewStyle().Height(contentH).MaxHeight(contentH).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, title, body, statusBar)
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e6c07b"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7a7a7a"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f07178"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#abb2bf")).Background(lipgloss.Color("#1e1e2e"))
	panel        = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#333344"))
	focusedPanel = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#e6c07b"))
	draftStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6c07b"))
)

func firstCommentable(fd *domain.FileDiff) int {
	if fd == nil {
		return 0
	}
	for i, ln := range fd.Lines {
		if diff.Commentable(ln) {
			return i
		}
	}
	return 0
}

func lastCommentable(fd *domain.FileDiff) int {
	if fd == nil {
		return 0
	}
	for i := len(fd.Lines) - 1; i >= 0; i-- {
		if diff.Commentable(fd.Lines[i]) {
			return i
		}
	}
	return 0
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if n <= 0 || len(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return s[:n-1] + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Run starts the Bubbletea program.
func Run(opts Options) error {
	m := NewModel(opts)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
