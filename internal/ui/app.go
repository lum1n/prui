package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/lum1n/prui/internal/ai"
	"github.com/lum1n/prui/internal/config"
	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
	"github.com/lum1n/prui/internal/provider"
	"github.com/lum1n/prui/internal/review"
)

type screen int

const (
	screenPRList screen = iota
	screenReview
	screenSubmit
	screenHelp
	screenOverview
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

func (i prItem) Title() string { return fmt.Sprintf("#%d  %s", i.pr.Ref.Number, i.pr.Title) }
func (i prItem) Description() string {
	// Used only as a plain fallback; prDelegate renders badges with color.
	return fmt.Sprintf("%s · %s", i.pr.Author, i.pr.State)
}
func (i prItem) FilterValue() string {
	return fmt.Sprintf("#%d %s %s %s", i.pr.Ref.Number, i.pr.Title, i.pr.Author, i.pr.State)
}

type fileItem struct {
	file     domain.FileChange
	drafts   int
	comments int
	maxTitle int // list content width for path fitting; 0 = no fit
}

func (i fileItem) FilterValue() string { return i.file.Path }

type Model struct {
	opts   Options
	screen screen
	pane   pane
	width  int
	height int

	prList   list.Model
	fileList list.Model
	prTab    prListTab
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

	commenting     bool
	editingID      string // non-empty while editing an existing draft
	commentGeneral bool   // true when drafting a PR-level (non-line) comment
	replyParentID  string // non-empty when drafting a reply in a thread
	threadTargetID string // remote comment selected as reply target on the cursor line
	convCursor     int    // selected row in conversation thread list
	tasks          []domain.Task
	taskCursor     int
	overviewSec    overviewSection
	activePath     string // last requested / shown file path
	leftWidth      int
	rightWidth     int
	contentHeight  int
	diffClickMap   []int // viewport content line → cursor index (-1 = non-selectable)

	diffCache map[string]*domain.FileDiff
	flat      []flatRow
	allGen    int // bumps to cancel in-flight all-file loads

	summary      string
	summaryErr   string
	summarizing  bool
	summaryGen   int // bumps to ignore stale summarize results
	summaryDetail ai.DetailLevel
}

type loadedPRsMsg struct {
	prs []domain.PullRequest
	err error
}
type loadedReviewMsg struct {
	pr       *domain.PullRequest
	files    []domain.FileChange
	comments []domain.Comment
	tasks    []domain.Task
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
type loadedTasksMsg struct {
	tasks []domain.Task
	err   error
}
type taskToggledMsg struct {
	err error
}
type loadedReviewStatusMsg struct {
	status domain.ReviewStatus
	err    error
}
type loadedSummaryMsg struct {
	gen    int
	text   string
	err    error
	kind   string
	model  string
	detail ai.DetailLevel
}

// NewModel creates the root TUI model.
func NewModel(opts Options) Model {
	prList := list.New(nil, newPRDelegate(), 0, 0)
	configureList(&prList, "Pull requests", "PR", "PRs")
	prList.SetShowTitle(false)

	fileList := list.New(nil, newFileDelegate(), 0, 0)
	configureList(&fileList, "Files", "file", "files")

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
	if opts.Config != nil {
		m.summaryDetail = ai.ParseDetailLevel(opts.Config.AI.SummaryDetail)
	} else {
		m.summaryDetail = ai.DetailMedium
	}
	if opts.PRNumber > 0 {
		m.screen = screenReview
	}
	return m
}

const defaultHelp = `Keys
  j/k       move
  ctrl+d/u  page down / page up (half screen)
  tab       PR list: next tab · review: switch pane
  ←/→       PR list: switch Open / Drafts / Merged
  enter     open PR / load file
  c         new comment on line (not on merged)
  R         reply to selected comment (diff: ▸ target · overview conversation)
  ,/.       prev/next reply target on line
  1-9       jump to reply target #N on line
  e         edit draft on line
  x         delete draft on line
  v         toggle range select
  y         yank plain code (cursor line or range)
  r         submit review (comment / approve / request changes)
  p         PR overview (status · reviews · tasks · description · summary · conversation)
  C         PR overview → conversation section
  S         AI summarize (config ai.default)
  s         cycle summary detail (short → medium → full)
  d         toggle unified/split layout
  a         toggle this-file / all-files view
  [ ]       prev/next hunk
  o         show PR URL in status
  O         open PR in browser
  ?         help
  q/esc     back / quit

Inline comment: enter save, esc cancel
Diff thread: ,/. or 1-9 pick target · R reply
Overview: reviews · tab section · j/k · space toggle task · S summarize · s detail · R reply · c new

Views
  this-file   only the selected path (default)
  all-files   every path in one scroll; file list + section headers track focus
  overview    status, reviews, tasks, description, summary, conversation (key p)
  merged      view-only: browse diff/overview, no comments or submit`

func (m Model) Init() tea.Cmd {
	if m.opts.PRNumber > 0 {
		return m.loadReview(m.opts.PRNumber)
	}
	return m.loadPRs()
}

func (m Model) loadPRs() tea.Cmd {
	tab := m.prTab
	return func() tea.Msg {
		ctx := context.Background()
		prs, err := m.opts.Provider.ListPullRequests(ctx, m.opts.Repo, domain.ListOpts{State: tab.listState()})
		if err != nil {
			return loadedPRsMsg{err: err}
		}
		// Enrich review badges (and resolve "you" against the signed-in user).
		for i := range prs {
			st, err := m.opts.Provider.GetReviewStatus(ctx, domain.PRRef{Repo: m.opts.Repo, Number: prs[i].Ref.Number})
			if err == nil {
				prs[i].Reviews = st
			}
		}
		return loadedPRsMsg{prs: prs}
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
		tasks, _ := m.opts.Provider.ListTasks(ctx, ref)
		var session *review.Session
		if pr.ViewOnly() {
			session = &review.Session{HostName: m.opts.Host.Name, Repo: ref.Repo, Number: ref.Number}
		} else {
			session, _ = review.Load(m.opts.Host.Name, ref)
			if remote, err := m.opts.Provider.StartReview(ctx, ref); err == nil && remote != nil {
				session.Draft.RemoteID = remote.RemoteID
			}
		}
		return loadedReviewMsg{pr: pr, files: files, comments: comments, tasks: tasks, session: session}
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
		m.syncPRListItemName()
		m.status = ""
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
		m.tasks = msg.tasks
		m.taskCursor = 0
		if m.pr != nil {
			for _, t := range m.tasks {
				if t.Required && !t.Done {
					m.pr.Blocked = true
					break
				}
			}
		}
		m.session = msg.session
		m.screen = screenReview
		m.diffCache = map[string]*domain.FileDiff{}
		m.flat = nil
		m.summary = ""
		m.summaryErr = ""
		m.summarizing = false
		m.summaryGen++
		m.layout()
		m.refreshFileList()
		m.status = fmt.Sprintf("PR #%d — %s", m.pr.Ref.Number, m.pr.Title)
		if m.pr.ViewOnly() {
			m.status += " · view only"
		}
		if badge := formatReviewBadge(m.pr.Reviews); badge != "" {
			m.status += " · " + badge
		}
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

	case loadedTasksMsg:
		m.loading = false
		if msg.err != nil {
			m.status = "Tasks: " + msg.err.Error()
			m.tasks = nil
		} else {
			m.tasks = msg.tasks
			if m.taskCursor >= len(m.tasks) {
				m.taskCursor = 0
			}
			if m.pr != nil {
				m.pr.Blocked = false
				for _, t := range m.tasks {
					if t.Required && !t.Done {
						m.pr.Blocked = true
						break
					}
				}
			}
		}
		if m.screen == screenOverview {
			m.renderOverview()
		}
		return m, nil

	case loadedReviewStatusMsg:
		if msg.err == nil && m.pr != nil {
			m.pr.Reviews = msg.status
			if m.screen == screenOverview {
				m.renderOverview()
			}
		}
		return m, nil

	case taskToggledMsg:
		m.loading = false
		if msg.err != nil {
			m.status = "Task update failed: " + msg.err.Error()
			return m, nil
		}
		m.status = "Task updated"
		return m, m.loadTasks()

	case loadedSummaryMsg:
		if msg.gen != m.summaryGen {
			return m, nil
		}
		m.summarizing = false
		m.loading = false
		if msg.err != nil {
			m.summaryErr = msg.err.Error()
			m.summary = ""
			m.status = "Summarize failed: " + msg.err.Error()
		} else {
			m.summaryErr = ""
			m.summary = msg.text
			label := msg.kind
			if msg.model != "" {
				label += "/" + msg.model
			}
			detail := msg.detail.String()
			if detail == "" {
				detail = m.summaryDetail.String()
			}
			m.status = "Summarized (" + detail + ") with " + label
		}
		m.overviewSec = sectionSummary
		m.screen = screenOverview
		m.renderOverview()
		return m, nil
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
	case screenOverview:
		return m.updateOverview(msg)
	}
	return m, nil
}

func (m Model) updatePRList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.handlePRListMouse(msg)
	case tea.KeyMsg:
		filtering := m.prList.FilterState() == list.Filtering
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			if filtering {
				break
			}
			m.prTab = m.prTab.next()
			m.loading = true
			m.errMsg = ""
			m.status = ""
			return m, m.loadPRs()
		case "shift+tab", "left", "h":
			if filtering {
				break
			}
			m.prTab = m.prTab.prev()
			m.loading = true
			m.errMsg = ""
			m.status = ""
			return m, m.loadPRs()
		case "1":
			if filtering {
				break
			}
			return m.switchPRTab(tabOpen)
		case "2":
			if filtering {
				break
			}
			return m.switchPRTab(tabDrafts)
		case "3":
			if filtering {
				break
			}
			return m.switchPRTab(tabMerged)
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
			if filtering {
				break
			}
			m.screen = screenHelp
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.prList, cmd = m.prList.Update(msg)
	return m, cmd
}

func (m Model) switchPRTab(tab prListTab) (tea.Model, tea.Cmd) {
	if m.prTab == tab {
		return m, nil
	}
	m.prTab = tab
	m.loading = true
	m.errMsg = ""
	m.status = ""
	return m, m.loadPRs()
}

func (m Model) reviewReadOnly() bool {
	return m.pr != nil && m.pr.ViewOnly()
}

func (m Model) rejectWrite(action string) (tea.Model, tea.Cmd) {
	m.status = action + " unavailable on " + strings.ToLower(m.pr.State) + " PRs"
	return m, nil
}

func (m Model) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.commenting {
		return m.updateInlineComment(msg)
	}

	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.handleReviewMouse(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			if m.opts.PRNumber > 0 {
				return m, tea.Quit
			}
			m.screen = screenPRList
			m.pr = nil
			m.fileDiff = nil
			m.status = ""
			m.layout()
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
			m.overviewSec = sectionTasks
			m.screen = screenOverview
			m.loading = true
			m.renderOverview()
			return m, tea.Batch(m.loadTasks(), m.loadReviewStatus())
		case "C":
			m.overviewSec = sectionConversation
			m.screen = screenOverview
			m.loading = true
			m.renderOverview()
			return m, tea.Batch(m.loadTasks(), m.loadReviewStatus())
		case "s":
			return m.cycleSummaryDetail()
		case "S":
			return m.beginSummarize()
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
			if m.reviewReadOnly() {
				return m.rejectWrite("Submit")
			}
			m.screen = screenSubmit
			m.submitIdx = 0
			return m, nil
		case "c":
			if m.reviewReadOnly() {
				return m.rejectWrite("Comment")
			}
			if !m.cursorCommentable() {
				m.status = "Select a commentable line"
				return m, nil
			}
			return m, m.beginComment("")
		case "R":
			if m.reviewReadOnly() {
				return m.rejectWrite("Reply")
			}
			m.syncThreadTarget()
			parent := m.threadTargetID
			if parent == "" {
				m.status = "No comment to reply to on this line"
				return m, nil
			}
			return m, m.beginReply(parent, false)
		case ",":
			if m.cycleThreadTarget(-1) {
				m.renderDiff()
			}
			return m, nil
		case ".":
			if m.cycleThreadTarget(1) {
				m.renderDiff()
			}
			return m, nil
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			n := int(msg.String()[0] - '0')
			if m.pickThreadTarget(n) {
				m.renderDiff()
			}
			return m, nil
		case "e":
			if m.reviewReadOnly() {
				return m.rejectWrite("Edit")
			}
			d := m.draftOnCursor()
			if d == nil {
				m.status = "No draft on this line"
				return m, nil
			}
			return m, m.beginComment(d.ID)
		case "x":
			if m.reviewReadOnly() {
				return m.rejectWrite("Delete")
			}
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
				m.status = "Range start set — move and press c or y"
			} else {
				m.rangeStart = -1
				m.status = "Range cleared"
			}
			m.renderDiff()
			return m, nil
		case "y":
			text, n, err := m.yankSelection()
			if err != nil {
				m.status = err.Error()
				return m, nil
			}
			if err := copyClipboard(text); err != nil {
				m.status = "Yank failed: " + err.Error()
				return m, nil
			}
			m.status = fmt.Sprintf("Yanked %d line(s)", n)
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

// draftOnCursor returns the last draft on the cursor line's thread, if any.
func (m *Model) draftOnCursor() *domain.DraftComment {
	if m.session == nil {
		return nil
	}
	row, ok := m.cursorRow()
	if !ok || row.header || row.fd == nil || row.line < 0 || row.line >= len(row.fd.Lines) {
		return nil
	}
	ln := row.fd.Lines[row.line]
	nodes := lineThread(m.comments, m.session.Draft.Comments, row.path, ln)
	var found *domain.DraftComment
	for _, n := range nodes {
		if n.Draft != nil {
			found = n.Draft
		}
	}
	return found
}

// cursorThreadNodes returns the comment thread under the cursor line.
func (m *Model) cursorThreadNodes() []threadNode {
	row, ok := m.cursorRow()
	if !ok || row.header || row.fd == nil || row.line < 0 || row.line >= len(row.fd.Lines) {
		return nil
	}
	var drafts []domain.DraftComment
	if m.session != nil {
		drafts = m.session.Draft.Comments
	}
	return lineThread(m.comments, drafts, row.path, row.fd.Lines[row.line])
}

// syncThreadTarget keeps threadTargetID valid for the cursor line (default: last).
func (m *Model) syncThreadTarget() {
	ids := replyableIDs(m.cursorThreadNodes())
	if len(ids) == 0 {
		m.threadTargetID = ""
		return
	}
	for _, id := range ids {
		if id == m.threadTargetID {
			return
		}
	}
	m.threadTargetID = ids[len(ids)-1]
}

func (m *Model) cycleThreadTarget(delta int) bool {
	ids := replyableIDs(m.cursorThreadNodes())
	if len(ids) == 0 {
		m.threadTargetID = ""
		m.status = "No comments on this line"
		return false
	}
	idx := len(ids) - 1
	for i, id := range ids {
		if id == m.threadTargetID {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(ids)) % len(ids)
	m.threadTargetID = ids[idx]
	m.status = fmt.Sprintf("Reply target %d/%d", idx+1, len(ids))
	return true
}

func (m *Model) pickThreadTarget(n int) bool {
	ids := replyableIDs(m.cursorThreadNodes())
	if len(ids) == 0 {
		m.status = "No comments on this line"
		return false
	}
	if n < 1 || n > len(ids) {
		m.status = fmt.Sprintf("No comment #%d (1–%d)", n, len(ids))
		return false
	}
	m.threadTargetID = ids[n-1]
	m.status = fmt.Sprintf("Reply target %d/%d", n, len(ids))
	return true
}

func (m *Model) beginComment(editID string) tea.Cmd {
	m.editingID = editID
	m.commentGeneral = false
	m.replyParentID = ""
	if editID != "" && m.session != nil {
		for _, d := range m.session.Draft.Comments {
			if d.ID == editID {
				m.comment.SetValue(d.Body)
				m.commentGeneral = d.Anchor == nil
				m.replyParentID = d.ParentID
				break
			}
		}
		m.status = "Editing draft — enter save, esc cancel"
	} else {
		m.comment.SetValue("")
		m.status = "Commenting — enter save, esc cancel"
	}
	if m.screen == screenOverview {
		m.commentGeneral = true
	} else {
		m.pane = paneDiff
	}
	m.comment.Focus()
	m.commenting = true
	m.layout()
	if m.screen == screenOverview {
		m.renderOverview()
	}
	return textarea.Blink
}

func (m *Model) beginReply(parentID string, general bool) tea.Cmd {
	m.editingID = ""
	m.replyParentID = parentID
	m.commentGeneral = general
	m.comment.SetValue("")
	m.comment.Focus()
	m.commenting = true
	m.status = "Reply — enter save, esc cancel"
	if general {
		m.overviewSec = sectionConversation
		m.layout()
		m.renderOverview()
	} else {
		m.pane = paneDiff
		m.layout()
	}
	return textarea.Blink
}

func (m *Model) beginGeneralComment() tea.Cmd {
	m.editingID = ""
	m.commentGeneral = true
	m.replyParentID = ""
	m.comment.SetValue("")
	m.comment.Focus()
	m.commenting = true
	m.status = "General comment — enter save, esc cancel"
	m.overviewSec = sectionConversation
	m.layout()
	m.renderOverview()
	return textarea.Blink
}

func (m *Model) conversationEntries() []convEntry {
	return buildConversationEntries(m.comments, m.sessionDrafts())
}

func (m Model) loadTasks() tea.Cmd {
	if m.pr == nil {
		return nil
	}
	num := m.pr.Ref.Number
	repo := m.opts.Repo
	prov := m.opts.Provider
	return func() tea.Msg {
		ctx := context.Background()
		tasks, err := prov.ListTasks(ctx, domain.PRRef{Repo: repo, Number: num})
		return loadedTasksMsg{tasks: tasks, err: err}
	}
}

func (m Model) loadReviewStatus() tea.Cmd {
	if m.pr == nil {
		return nil
	}
	num := m.pr.Ref.Number
	repo := m.opts.Repo
	prov := m.opts.Provider
	return func() tea.Msg {
		ctx := context.Background()
		st, err := prov.GetReviewStatus(ctx, domain.PRRef{Repo: repo, Number: num})
		return loadedReviewStatusMsg{status: st, err: err}
	}
}

func (m Model) toggleSelectedTask() tea.Cmd {
	if m.taskCursor < 0 || m.taskCursor >= len(m.tasks) || m.pr == nil {
		return nil
	}
	t := m.tasks[m.taskCursor]
	done := !t.Done
	num := m.pr.Ref.Number
	repo := m.opts.Repo
	prov := m.opts.Provider
	id := t.ID
	return func() tea.Msg {
		ctx := context.Background()
		err := prov.SetTaskDone(ctx, domain.PRRef{Repo: repo, Number: num}, id, done)
		return taskToggledMsg{err: err}
	}
}

func (m Model) cycleSummaryDetail() (tea.Model, tea.Cmd) {
	m.summaryDetail = ai.NextDetail(m.summaryDetail)
	m.status = "Summary detail: " + m.summaryDetail.String() + " (S to run)"
	if m.screen == screenOverview {
		m.renderOverview()
	}
	return m, nil
}

func (m Model) beginSummarize() (tea.Model, tea.Cmd) {
	if m.pr == nil {
		m.status = "No PR loaded"
		return m, nil
	}
	if m.opts.Config == nil || !m.opts.Config.AIConfigured() {
		m.status = "AI not configured — set ai.default and ai.providers in config.yaml"
		return m, nil
	}
	if m.summarizing {
		m.status = "Already summarizing…"
		return m, nil
	}
	m.summaryGen++
	m.summarizing = true
	m.summaryErr = ""
	m.overviewSec = sectionSummary
	m.screen = screenOverview
	m.loading = true
	prov, _ := m.opts.Config.DefaultAIProvider()
	label := prov.Kind
	if prov.Model != "" {
		label += "/" + prov.Model
	}
	m.status = "Summarizing (" + m.summaryDetail.String() + ") with " + label + "…"
	m.renderOverview()
	return m, m.loadSummary(m.summaryGen)
}

func (m Model) loadSummary(gen int) tea.Cmd {
	cfg := m.opts.Config
	host := m.opts.Host
	pr := m.pr
	detail := m.summaryDetail
	files := append([]domain.FileChange(nil), m.files...)
	tasks := append([]domain.Task(nil), m.tasks...)
	diffs := map[string]string{}
	for path, fd := range m.diffCache {
		if fd != nil && fd.Raw != "" {
			diffs[path] = fd.Raw
		}
	}
	for _, f := range files {
		if _, ok := diffs[f.Path]; !ok && f.Patch != "" {
			diffs[f.Path] = f.Patch
		}
	}
	return func() tea.Msg {
		p, err := cfg.DefaultAIProvider()
		if err != nil {
			return loadedSummaryMsg{gen: gen, err: err, detail: detail}
		}
		completer, err := ai.New(ai.Options{
			Provider:   p,
			Config:     cfg,
			ActiveHost: host,
		})
		if err != nil {
			return loadedSummaryMsg{gen: gen, err: err, kind: p.Kind, model: p.Model, detail: detail}
		}
		user := ai.PackContext(ai.ContextInput{
			PR:              pr,
			Files:           files,
			Tasks:           tasks,
			Diffs:           diffs,
			MaxContextBytes: cfg.AI.MaxContextBytes,
		})
		timeout := cfg.AI.TimeoutSec
		if timeout <= 0 {
			timeout = 120
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		text, err := completer.Complete(ctx, ai.Request{
			System: ai.SystemPromptFor(detail),
			User:   user,
			Model:  p.Model,
		})
		return loadedSummaryMsg{gen: gen, text: text, err: err, kind: completer.Kind(), model: p.Model, detail: detail}
	}
}

func (m Model) updateOverview(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.commenting {
		return m.updateInlineComment(msg)
	}
	entries := m.conversationEntries()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "p", "C":
			m.screen = screenReview
			m.commenting = false
			m.commentGeneral = false
			m.replyParentID = ""
			m.layout()
			return m, nil
		case "tab":
			m.overviewSec = m.overviewSec.next()
			m.renderOverview()
			return m, nil
		case "shift+tab":
			m.overviewSec = m.overviewSec.prev()
			m.renderOverview()
			return m, nil
		case "j", "down":
			switch m.overviewSec {
			case sectionTasks:
				if len(m.tasks) == 0 {
					return m, nil
				}
				m.taskCursor = (m.taskCursor + 1) % len(m.tasks)
				m.renderOverview()
				return m, nil
			case sectionConversation:
				if len(entries) == 0 {
					return m, nil
				}
				m.convCursor = (m.convCursor + 1) % len(entries)
				m.renderOverview()
				return m, nil
			case sectionDescription, sectionSummary:
				m.bodyVP.LineDown(1)
				return m, nil
			}
		case "k", "up":
			switch m.overviewSec {
			case sectionTasks:
				if len(m.tasks) == 0 {
					return m, nil
				}
				m.taskCursor = (m.taskCursor - 1 + len(m.tasks)) % len(m.tasks)
				m.renderOverview()
				return m, nil
			case sectionConversation:
				if len(entries) == 0 {
					return m, nil
				}
				m.convCursor = (m.convCursor - 1 + len(entries)) % len(entries)
				m.renderOverview()
				return m, nil
			case sectionDescription, sectionSummary:
				m.bodyVP.LineUp(1)
				return m, nil
			}
		case "ctrl+d", "pgdown":
			m.bodyVP.HalfPageDown()
			return m, nil
		case "ctrl+u", "pgup":
			m.bodyVP.HalfPageUp()
			return m, nil
		case "g", "home":
			m.bodyVP.GotoTop()
			return m, nil
		case "G", "end":
			m.bodyVP.GotoBottom()
			return m, nil
		case " ", "enter":
			if m.overviewSec != sectionTasks {
				return m, nil
			}
			if m.reviewReadOnly() {
				return m.rejectWrite("Task updates")
			}
			if len(m.tasks) == 0 {
				m.status = "No tasks"
				return m, nil
			}
			m.loading = true
			return m, m.toggleSelectedTask()
		case "c":
			if m.reviewReadOnly() {
				return m.rejectWrite("Comment")
			}
			m.overviewSec = sectionConversation
			return m, m.beginGeneralComment()
		case "R":
			if m.reviewReadOnly() {
				return m.rejectWrite("Reply")
			}
			m.overviewSec = sectionConversation
			if len(entries) == 0 {
				m.status = "No comment to reply to"
				return m, nil
			}
			if m.convCursor < 0 || m.convCursor >= len(entries) {
				m.convCursor = 0
			}
			e := entries[m.convCursor]
			parent := e.ID
			if e.Draft {
				if e.Parent == "" {
					m.status = "Select a remote comment to reply to"
					return m, nil
				}
				parent = e.Parent
			}
			return m, m.beginReply(parent, true)
		case "e":
			if m.reviewReadOnly() {
				return m.rejectWrite("Edit")
			}
			m.overviewSec = sectionConversation
			if len(entries) == 0 {
				m.status = "No drafts"
				return m, nil
			}
			if m.convCursor < 0 || m.convCursor >= len(entries) {
				m.convCursor = 0
			}
			e := entries[m.convCursor]
			if !e.Draft {
				m.status = "Select a draft to edit"
				return m, nil
			}
			return m, m.beginComment(e.ID)
		case "x":
			if m.reviewReadOnly() {
				return m.rejectWrite("Delete")
			}
			m.overviewSec = sectionConversation
			if len(entries) == 0 {
				m.status = "No drafts"
				return m, nil
			}
			if m.convCursor < 0 || m.convCursor >= len(entries) {
				m.convCursor = 0
			}
			e := entries[m.convCursor]
			if !e.Draft {
				m.status = "Select a draft to delete"
				return m, nil
			}
			_ = m.session.RemoveComment(e.ID)
			m.status = "Draft deleted"
			if m.convCursor > 0 {
				m.convCursor--
			}
			m.renderOverview()
			return m, nil
		case "s":
			return m.cycleSummaryDetail()
		case "S":
			return m.beginSummarize()
		}
	}
	var cmd tea.Cmd
	m.bodyVP, cmd = m.bodyVP.Update(msg)
	return m, cmd
}

func (m *Model) sessionDrafts() []domain.DraftComment {
	if m.session == nil {
		return nil
	}
	return m.session.Draft.Comments
}

func (m Model) updateInlineComment(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			wasGeneral := m.commentGeneral
			m.commenting = false
			m.editingID = ""
			m.commentGeneral = false
			m.replyParentID = ""
			m.comment.Blur()
			m.layout()
			m.status = "Comment cancelled"
			if wasGeneral {
				m.screen = screenOverview
				m.overviewSec = sectionConversation
				m.renderOverview()
			}
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
			wasGeneral := m.commentGeneral
			replyParent := m.replyParentID
			path := ""
			if !wasGeneral {
				if row, ok := m.cursorRow(); ok && !row.header {
					path = row.path
				} else if m.fileDiff != nil {
					path = m.fileDiff.Path
				}
			}
			if m.editingID != "" {
				if err := m.session.UpdateComment(m.editingID, body); err != nil {
					m.status = err.Error()
					return m, nil
				}
				m.status = "Draft updated"
			} else if replyParent != "" {
				dc := domain.DraftComment{Body: body, ParentID: replyParent}
				if !wasGeneral {
					dc.Path = path
					// Keep a soft anchor so the reply stays visible on this line
					// even before parent lookup on reload.
					if a := m.selectedAnchor(); a != nil {
						dc.Anchor = a
					}
				}
				_ = m.session.AddComment(dc)
				m.status = "Reply draft added"
			} else if wasGeneral {
				_ = m.session.AddComment(domain.DraftComment{Body: body})
				m.status = "General draft added"
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
			m.commentGeneral = false
			m.replyParentID = ""
			m.comment.Blur()
			m.layout()
			if wasGeneral {
				m.screen = screenOverview
				m.overviewSec = sectionConversation
				m.renderOverview()
			} else {
				m.renderDiff()
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.comment, cmd = m.comment.Update(msg)
	return m, cmd
}

func (m Model) updateSubmit(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.reviewReadOnly() {
		m.screen = screenReview
		return m.rejectWrite("Submit")
	}
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

func reviewActionLabel(a domain.ReviewAction) string {
	switch a {
	case domain.ActionApprove:
		return "Approve"
	case domain.ActionRequestChanges:
		return "Request changes"
	default:
		return "Comment only"
	}
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

// yankSelection returns plain code for the cursor line or visual range.
func (m *Model) yankSelection() (text string, lines int, err error) {
	sel, err := m.selectionDiffLines()
	if err != nil {
		return "", 0, err
	}
	text = yankLines(sel)
	if text == "" {
		return "", 0, fmt.Errorf("nothing to yank")
	}
	n := strings.Count(text, "\n") + 1
	return text, n, nil
}

func (m *Model) selectionDiffLines() ([]domain.DiffLine, error) {
	if m.showAll {
		if len(m.flat) == 0 {
			return nil, fmt.Errorf("no diff loaded")
		}
		lo, hi := m.cursorLine, m.cursorLine
		if m.rangeStart >= 0 {
			lo, hi = m.rangeStart, m.cursorLine
			if lo > hi {
				lo, hi = hi, lo
			}
		}
		if lo < 0 {
			lo = 0
		}
		if hi >= len(m.flat) {
			hi = len(m.flat) - 1
		}
		var out []domain.DiffLine
		var path string
		for i := lo; i <= hi; i++ {
			row := m.flat[i]
			if row.header || row.fd == nil || row.line < 0 || row.line >= len(row.fd.Lines) {
				continue
			}
			if path == "" {
				path = row.path
			} else if row.path != path {
				return nil, fmt.Errorf("yank range must stay in one file")
			}
			out = append(out, row.fd.Lines[row.line])
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("select a code line")
		}
		return out, nil
	}
	if m.fileDiff == nil || len(m.fileDiff.Lines) == 0 {
		return nil, fmt.Errorf("no diff loaded")
	}
	lo, hi := m.cursorLine, m.cursorLine
	if m.rangeStart >= 0 {
		lo, hi = m.rangeStart, m.cursorLine
		if lo > hi {
			lo, hi = hi, lo
		}
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(m.fileDiff.Lines) {
		hi = len(m.fileDiff.Lines) - 1
	}
	return m.fileDiff.Lines[lo : hi+1], nil
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

func (m *Model) replyTargetLabel(width int) string {
	id := m.replyParentID
	if id == "" {
		id = m.threadTargetID
	}
	var author, body string
	for _, c := range m.comments {
		if c.ID == id {
			author, body = c.Author, c.Body
			break
		}
	}
	line := strings.TrimSpace(anchorHint(m.selectedAnchor()))
	if author == "" {
		if line != "" {
			return "✎ reply · " + line + "  (enter save · esc cancel)"
		}
		return "✎ reply  (enter save · esc cancel)"
	}
	snippet := strings.Join(strings.Fields(body), " ")
	if snippet == "" {
		snippet = "(empty)"
	}
	head := "✎ reply to " + author
	if line != "" {
		head += " · " + line
	}
	head += ":"
	if width <= 0 {
		width = 60
	}
	// Keep label readable: wrap author/line then a short quoted snippet.
	maxSnippet := width - 4
	if maxSnippet < 20 {
		maxSnippet = 20
	}
	snippet = truncate(snippet, maxSnippet)
	return wrapWidth(head+"\n  “"+snippet+"”  (enter save · esc cancel)", width)
}

func (m *Model) commentEditorLabel(width int) string {
	var label string
	switch {
	case m.editingID != "":
		label = "✎ edit draft" + anchorHint(m.selectedAnchor()) + "  (enter save · esc cancel)"
	case m.replyParentID != "":
		return m.replyTargetLabel(width)
	case m.commentGeneral:
		label = "✎ general comment  (enter save · esc cancel)"
	default:
		label = "✎ comment" + anchorHint(m.selectedAnchor()) + "  (enter save · esc cancel)"
	}
	return wrapWidth(label, width)
}

func (m *Model) moveCursor(delta int) {
	if m.showAll {
		if len(m.flat) == 0 {
			return
		}
		m.moveFlatCursor(delta)
		m.threadTargetID = ""
		m.syncThreadTarget()
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
	m.threadTargetID = ""
	m.syncThreadTarget()
	m.renderDiff()
}

// pageCursor jumps by half the viewport height (vim ctrl+d / ctrl+u).
func (m *Model) pageCursor(dir int) {
	step := m.diffVP.Height / 2
	if step < 1 {
		step = 10
	}
	m.nudgeDiffCursor(dir * step)
}

// nudgeDiffCursor moves the selection by delta lines (clamped), keeping the
// viewport centered via renderDiff. Used by page keys and mouse wheel.
func (m *Model) nudgeDiffCursor(delta int) {
	if delta == 0 {
		return
	}
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
		m.threadTargetID = ""
		m.syncThreadTarget()
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
	m.threadTargetID = ""
	m.syncThreadTarget()
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
	titleW := m.fileListTitleWidth()
	for i, f := range m.files {
		items[i] = fileItem{
			file:     f,
			drafts:   draftCounts[f.Path],
			comments: commentCounts[f.Path],
			maxTitle: titleW,
		}
	}
	m.fileList.SetItems(items)
}

func (m *Model) fileListTitleWidth() int {
	w := m.fileList.Width()
	if w <= 0 {
		w = m.leftWidth - 2
	}
	// DefaultDelegate pads titles by 2; leave that for the list truncator.
	w -= 2
	if w < 8 {
		w = 8
	}
	return w
}

func (m *Model) layout() {
	titleH := 1
	statusH := 1
	helpH := 0
	if m.screen == screenReview || m.screen == screenSubmit || m.screen == screenOverview || m.screen == screenPRList {
		helpH = 1
	}
	contentH := m.height - titleH - statusH - helpH
	if contentH < 3 {
		contentH = 3
	}
	m.contentHeight = contentH
	// Reserve rows for tab bar on the PR list screen.
	prH := contentH
	if m.screen == screenPRList {
		prH = contentH - prTabBarHeight
	}
	if prH < 3 {
		prH = 3
	}
	m.prList.SetSize(m.width, prH)

	// Outer panel widths must sum to terminal width (borders are outside Width()).
	leftW, rightW := splitPanelWidths(m.width)
	m.leftWidth = leftW
	m.rightWidth = rightW

	commentH := 0
	if m.commenting {
		commentH = 7 // label (up to 2 lines) + textarea
	}
	diffH := contentH - commentH
	if diffH < 3 {
		diffH = 3
	}

	// Inner sizes: lipgloss Width/Height are content-box; borders add 2.
	innerW := leftW - 2
	if innerW < 8 {
		innerW = 8
	}
	innerH := contentH - 2
	if innerH < 1 {
		innerH = 1
	}
	fileH := innerH - 1 // list + item-count footer
	if fileH < 1 {
		fileH = 1
	}
	m.fileList.SetSize(innerW, fileH)
	if len(m.files) > 0 {
		m.refreshFileList()
	}

	diffInnerW := rightW - 2
	if diffInnerW < 8 {
		diffInnerW = 8
	}
	diffInnerH := diffH - 2
	if diffInnerH < 1 {
		diffInnerH = 1
	}
	m.diffVP.Width = diffInnerW
	m.diffVP.Height = diffInnerH
	m.bodyVP.Width = m.width
	bodyH := contentH
	if m.screen == screenOverview && m.commenting {
		bodyH = contentH - commentH
		if bodyH < 3 {
			bodyH = 3
		}
		m.comment.SetWidth(m.width - 4)
	} else {
		m.comment.SetWidth(diffInnerW - 2)
		if m.comment.Width() < 10 {
			m.comment.SetWidth(10)
		}
	}
	m.bodyVP.Height = bodyH
	m.comment.SetHeight(3)
	m.renderDiff()
}

// splitPanelWidths returns outer left/right widths that always sum to total.
func splitPanelWidths(total int) (left, right int) {
	if total < 40 {
		left = total / 3
		if left < 12 {
			left = 12
		}
	} else {
		left = total / 3
		if left < 24 {
			left = 24
		}
		if left > 48 {
			left = 48
		}
	}
	if left >= total-10 {
		left = total / 3
		if left < 1 {
			left = 1
		}
	}
	right = total - left
	if right < 10 {
		right = 10
		left = total - right
		if left < 1 {
			left = 1
			right = total - left
		}
	}
	return left, right
}

// renderBorderedPanel draws a bordered box with exact outer dimensions.
// Lipgloss Width/Height are content-box (borders add 2). Do not set MaxWidth
// to the content width — that clamps the final box and shrinks the outer
// width by the border. MaxHeight(outerH) clamps the final box including border.
func renderBorderedPanel(sty lipgloss.Style, outerW, outerH int, inner string) string {
	if outerW < 2 {
		outerW = 2
	}
	if outerH < 2 {
		outerH = 2
	}
	return sty.Width(outerW - 2).Height(outerH - 2).MaxHeight(outerH).Render(inner)
}

func (m *Model) renderOverview() {
	entries := m.conversationEntries()
	if len(entries) == 0 {
		m.convCursor = 0
	} else if m.convCursor >= len(entries) {
		m.convCursor = len(entries) - 1
	} else if m.convCursor < 0 {
		m.convCursor = 0
	}
	if len(m.tasks) == 0 {
		m.taskCursor = 0
	} else if m.taskCursor >= len(m.tasks) {
		m.taskCursor = len(m.tasks) - 1
	}

	raw := ""
	if m.pr != nil {
		raw = m.pr.Body
	}
	desc := renderMarkdown(raw, m.width-4)
	sum := ""
	if strings.TrimSpace(m.summary) != "" {
		sum = renderMarkdown(m.summary, m.width-4)
	}
	content, focusLine := formatOverview(
		m.pr, m.tasks, m.taskCursor, entries, m.convCursor, m.overviewSec,
		desc, sum, m.summaryErr, m.summarizing, m.summaryDetail.String(), m.width-2,
	)
	m.bodyVP.SetContent(content)
	if focusLine >= 0 {
		m.scrollBodyToLine(focusLine)
	}
}

// scrollBodyToLine keeps line visible inside the overview viewport.
func (m *Model) scrollBodyToLine(line int) {
	h := m.bodyVP.Height
	if h <= 0 {
		return
	}
	y := m.bodyVP.YOffset
	pad := 1
	if line < y+pad {
		m.bodyVP.SetYOffset(max(0, line-pad))
		return
	}
	if line >= y+h-pad {
		m.bodyVP.SetYOffset(max(0, line-h+pad+1))
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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

	m.resetDiffClickMap()
	if m.showAll {
		m.renderFlatDiff(th, width)
		return
	}
	if m.fileDiff == nil {
		m.diffVP.SetContent("  select a file")
		m.finalizeDiffClickMap("  select a file")
		return
	}

	var b strings.Builder
	hdr := diff.PaintFileHeader(m.fileDiff.Path, m.fileDiff.Status, th, width) + "\n"
	b.WriteString(hdr)
	m.noteDiffLines(hdr, -1)

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
			elide := mutedStyle.Render(fmt.Sprintf("  ··· %d lines above ···", start)) + "\n"
			b.WriteString(elide)
			m.noteDiffLines(elide, -1)
		}
	}
	for i := start; i < end; i++ {
		m.writeDiffLine(&b, m.fileDiff, m.fileDiff.Lines[i], i == m.cursorLine || m.lineInRange(i), i == m.cursorLine, th, width, i)
	}
	if end < len(m.fileDiff.Lines) {
		elide := mutedStyle.Render(fmt.Sprintf("  ··· %d lines below ···", len(m.fileDiff.Lines)-end)) + "\n"
		b.WriteString(elide)
		m.noteDiffLines(elide, -1)
	}
	content := b.String()
	m.diffVP.SetContent(content)
	m.finalizeDiffClickMap(content)
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
		m.finalizeDiffClickMap("  loading files…")
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
			elide := mutedStyle.Render(fmt.Sprintf("  ··· %d rows above ···", start)) + "\n"
			b.WriteString(elide)
			m.noteDiffLines(elide, -1)
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
			chunk := hdr + "\n"
			b.WriteString(chunk)
			m.noteDiffLines(chunk, i)
			continue
		}
		if row.fd == nil || row.line < 0 || row.line >= len(row.fd.Lines) {
			continue
		}
		m.writeDiffLine(&b, row.fd, row.fd.Lines[row.line], sel, i == m.cursorLine, th, width, i)
	}
	if end < len(m.flat) {
		elide := mutedStyle.Render(fmt.Sprintf("  ··· %d rows below ···", len(m.flat)-end)) + "\n"
		b.WriteString(elide)
		m.noteDiffLines(elide, -1)
	}
	content := b.String()
	m.diffVP.SetContent(content)
	m.finalizeDiffClickMap(content)
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

func (m *Model) writeDiffLine(b *strings.Builder, fd *domain.FileDiff, ln domain.DiffLine, sel, focus bool, th diff.Theme, width, cursorIdx int) {
	opt := diff.Options{Theme: th, Width: width, Selected: sel, Split: m.splitDiff}
	line := diff.Paint(m.hl, fd.Path, ln, opt) + "\n"
	b.WriteString(line)
	m.noteDiffLines(line, cursorIdx)
	var drafts []domain.DraftComment
	if m.session != nil {
		drafts = m.session.Draft.Comments
	}
	nodes := lineThread(m.comments, drafts, fd.Path, ln)
	selectedID := ""
	number := false
	if focus {
		m.syncThreadTarget()
		selectedID = m.threadTargetID
		number = len(replyableIDs(nodes)) > 1
	}
	thread := paintThread(nodes, selectedID, number, th, width)
	b.WriteString(thread)
	m.noteDiffLines(thread, cursorIdx)
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	title := titleStyle.Render("prui") + "  " + mutedStyle.Render(fmt.Sprintf("%s/%s", m.opts.Repo.Owner, m.opts.Repo.Name))
	if m.loading {
		title += mutedStyle.Render("  …")
	}
	title = lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).MaxHeight(1).Render(title)

	var body string
	switch m.screen {
	case screenPRList:
		body = lipgloss.JoinVertical(lipgloss.Left,
			renderPRTabs(m.prTab, m.width),
			m.prList.View(),
		)
	case screenReview:
		leftInner := listViewWithFooter(m.fileList)
		rightInner := m.diffVP.View()
		if m.commenting {
			labelW := m.rightWidth - 4
			if labelW < 20 {
				labelW = 20
			}
			label := m.commentEditorLabel(labelW)
			box := lipgloss.JoinVertical(lipgloss.Left,
				draftStyle.Width(labelW).MaxWidth(labelW).Render(label),
				m.comment.View(),
			)
			rightInner = lipgloss.JoinVertical(lipgloss.Left, rightInner, box)
		}
		lw, rw := m.leftWidth, m.rightWidth
		if lw == 0 || rw == 0 || lw+rw != m.width {
			lw, rw = splitPanelWidths(m.width)
		}
		panelH := m.contentHeight
		if panelH < 3 {
			panelH = 3
		}
		var left, right string
		if m.pane == paneFiles && !m.commenting {
			left = renderBorderedPanel(focusedPanel, lw, panelH, leftInner)
			right = renderBorderedPanel(panel, rw, panelH, rightInner)
		} else {
			left = renderBorderedPanel(panel, lw, panelH, leftInner)
			right = renderBorderedPanel(focusedPanel, rw, panelH, rightInner)
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	case screenSubmit:
		var lines []string
		lines = append(lines, titleStyle.Render("Submit review"), "")
		if !provider.SupportsRequestChanges(m.opts.Host.Kind) {
			lines = append(lines, mutedStyle.Render("Note: this host has no request-changes action; comments will still post."), "")
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
			lines = append(lines, prefix+reviewActionLabel(a))
		}
		lines = append(lines, "", mutedStyle.Render("enter submit · esc cancel"))
		body = strings.Join(lines, "\n")
	case screenHelp:
		body = m.helpText
	case screenOverview:
		inner := m.bodyVP.View()
		if m.commenting {
			labelW := m.width - 4
			if labelW < 20 {
				labelW = 40
			}
			label := m.commentEditorLabel(labelW)
			inner = lipgloss.JoinVertical(lipgloss.Left,
				inner,
				draftStyle.Width(labelW).Render(label),
				m.comment.View(),
			)
		}
		body = inner
	}

	status := m.status
	if m.errMsg != "" {
		status = errorStyle.Render(m.errMsg)
	} else if status == "" && m.screen == screenPRList {
		status = listCountLabel(m.prList)
	}
	statusBar := statusStyle.Width(m.width).MaxWidth(m.width).MaxHeight(1).Render(truncate(status, m.width))

	helpH := 0
	var helpBar string
	switch m.screen {
	case screenReview, screenSubmit, screenOverview:
		helpH = 1
		helpBar = helpBarStyle.Width(m.width).MaxWidth(m.width).MaxHeight(1).Render(truncate(m.reviewHelpLine(), m.width))
	case screenPRList:
		helpH = 1
		helpBar = helpBarStyle.Width(m.width).MaxWidth(m.width).MaxHeight(1).Render(truncate(prListHelpLine(), m.width))
	}

	// Title + help + status are fixed chrome; body fills the rest.
	chrome := 1 + 1 + helpH // title + status (+ help)
	contentH := m.height - chrome
	if contentH < 1 {
		contentH = 1
	}
	// Do not set Width here — it reflows JoinHorizontal panels and destroys columns.
	body = lipgloss.NewStyle().Height(contentH).MaxHeight(contentH).Render(body)

	parts := []string{title, body}
	if helpH > 0 {
		parts = append(parts, helpBar)
	}
	parts = append(parts, statusBar)
	out := lipgloss.JoinVertical(lipgloss.Left, parts...)
	// Hard clamp so a single mis-sized child never clips terminal borders.
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(out)
}

func (m Model) reviewHelpLine() string {
	if m.screen == screenSubmit {
		return "j/k choose · enter submit · esc cancel"
	}
	if m.screen == screenOverview {
		if m.commenting {
			return "enter save · esc cancel"
		}
		sec := "tasks"
		switch m.overviewSec {
		case sectionDescription:
			sec = "description"
		case sectionSummary:
			sec = "summary"
		case sectionConversation:
			sec = "conversation"
		}
		open := openTaskCount(m.tasks)
		if m.reviewReadOnly() {
			return fmt.Sprintf("overview · %s · view only · tab section · j/k · ctrl+d/u scroll · S summarize · s detail · %d open tasks · p/esc back", sec, open)
		}
		return fmt.Sprintf("overview · %s · tab section · j/k · ctrl+d/u scroll · space toggle · S summarize · s detail · R reply · c add · %d open tasks · p/esc back", sec, open)
	}
	if m.commenting {
		if m.editingID != "" {
			return "enter save edit · esc cancel"
		}
		if m.replyParentID != "" {
			return "reply · enter save · esc cancel"
		}
		return "enter save · esc cancel"
	}
	view := "this-file"
	if m.showAll {
		view = "all-files"
	}
	layout := "unified"
	if m.splitDiff {
		layout = "split"
	}
	pane := "files"
	if m.pane == paneDiff {
		pane = "diff"
	}
	open := openTaskCount(m.tasks)
	nConv := len(conversationComments(m.comments))
	if m.reviewReadOnly() {
		return fmt.Sprintf(
			"tab pane(%s) · j/k · ^d/^u · [/] hunk · y yank · a %s · d %s · p overview(%d/%d) · S summarize · s detail · O open · view only · ? · q",
			pane, view, layout, open, nConv,
		)
	}
	return fmt.Sprintf(
		"tab pane(%s) · j/k · ^d/^u · [/] hunk · c comment · ,/.|# target · R reply · e edit · x del · v range · y yank · a %s · d %s · p overview(%d/%d) · S summarize · s detail · r submit · O open · ? · q",
		pane, view, layout, open, nConv,
	)
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c8a35a"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#abb2bf"))
	helpBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b919a"))
	panel        = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#2e3440"))
	focusedPanel = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#c8a35a"))
	draftStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#c8a35a"))
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
