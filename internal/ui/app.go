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
	screenComment
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
	fd  *domain.FileDiff
	err error
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
	ta.Placeholder = "Comment…"
	ta.ShowLineNumbers = false
	ta.SetHeight(6)

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
	}
	if opts.PRNumber > 0 {
		m.screen = screenReview
	}
	return m
}

const defaultHelp = `Keys
  j/k       move
  tab       switch pane
  enter     open PR / load file
  c         comment on line
  v         toggle range select
  r         submit review
  p         PR description (markdown)
  d         toggle unified/split diff
  [ ]       prev/next hunk
  o         show PR URL in status
  ?         help
  q/esc     back / quit

Comment editor: ctrl+s save draft, esc cancel`

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
	return func() tea.Msg {
		ctx := context.Background()
		ref := domain.PRRef{Repo: m.opts.Repo, Number: m.pr.Ref.Number}
		fd, err := m.opts.Provider.GetFileDiff(ctx, ref, path)
		return loadedDiffMsg{fd: fd, err: err}
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
		m.refreshFileList()
		m.status = fmt.Sprintf("PR #%d — %s", m.pr.Ref.Number, m.pr.Title)
		if len(m.files) > 0 {
			return m, m.loadDiff(m.files[0].Path)
		}
		return m, nil

	case loadedDiffMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.fileDiff = msg.fd
		m.cursorLine = firstCommentable(msg.fd)
		m.rangeStart = -1
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
	case screenComment:
		return m.updateComment(msg)
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
				m.status = "Split diff"
			} else {
				m.status = "Unified diff"
			}
			return m, nil
		case "o":
			if m.pr != nil {
				m.status = m.pr.URL
			}
			return m, nil
		case "r":
			m.screen = screenSubmit
			m.submitIdx = 0
			return m, nil
		case "c":
			if m.fileDiff == nil || m.cursorLine < 0 || m.cursorLine >= len(m.fileDiff.Lines) || !diff.Commentable(m.fileDiff.Lines[m.cursorLine]) {
				m.status = "Select a commentable line"
				return m, nil
			}
			m.comment.SetValue("")
			m.comment.Focus()
			m.screen = screenComment
			return m, textarea.Blink
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

		if m.pane == paneDiff && m.fileDiff != nil {
			switch msg.String() {
			case "j", "down":
				m.moveCursor(1)
				return m, nil
			case "k", "up":
				m.moveCursor(-1)
				return m, nil
			case "]":
				m.jumpHunk(1)
				return m, nil
			case "[":
				m.jumpHunk(-1)
				return m, nil
			case "g":
				m.cursorLine = firstCommentable(m.fileDiff)
				m.renderDiff()
				return m, nil
			case "G":
				m.cursorLine = lastCommentable(m.fileDiff)
				m.renderDiff()
				return m, nil
			}
		}

		if m.pane == paneFiles {
			switch msg.String() {
			case "enter", "l":
				if item, ok := m.fileList.SelectedItem().(fileItem); ok {
					m.loading = true
					return m, m.loadDiff(item.file.Path)
				}
			}
			var cmd tea.Cmd
			m.fileList, cmd = m.fileList.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) updateComment(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenReview
			return m, nil
		case "ctrl+s", "ctrl+enter":
			body := strings.TrimSpace(m.comment.Value())
			if body == "" {
				m.status = "Empty comment"
				return m, nil
			}
			anchor := m.selectedAnchor()
			_ = m.session.AddComment(domain.DraftComment{
				Body:   body,
				Path:   m.fileDiff.Path,
				Anchor: anchor,
			})
			m.refreshFileList()
			m.rangeStart = -1
			m.screen = screenReview
			m.status = "Draft comment added"
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

func (m *Model) selectedAnchor() *domain.Anchor {
	if m.fileDiff == nil || m.cursorLine < 0 || m.cursorLine >= len(m.fileDiff.Lines) {
		return nil
	}
	line := m.fileDiff.Lines[m.cursorLine]
	a := line.Anchor
	if m.rangeStart >= 0 && m.rangeStart != m.cursorLine {
		start, end := m.rangeStart, m.cursorLine
		if start > end {
			start, end = end, start
		}
		sa := m.fileDiff.Lines[start].Anchor
		ea := m.fileDiff.Lines[end].Anchor
		a.Line = sa.Line
		a.EndLine = ea.Line
		if a.EndLine < a.Line {
			a.Line, a.EndLine = a.EndLine, a.Line
		}
	}
	return &a
}

func (m *Model) moveCursor(delta int) {
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

func (m *Model) jumpHunk(dir int) {
	if m.fileDiff == nil {
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
	m.prList.SetSize(m.width, contentH)

	leftW := m.width / 3
	if leftW < 24 {
		leftW = 24
	}
	if leftW > 48 {
		leftW = 48
	}
	rightW := m.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}
	m.fileList.SetSize(leftW, contentH)
	m.diffVP.Width = rightW
	m.diffVP.Height = contentH
	m.bodyVP.Width = m.width
	m.bodyVP.Height = contentH
	m.comment.SetWidth(min(80, m.width-4))
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
	if m.fileDiff == nil {
		m.diffVP.SetContent("(no file selected)")
		return
	}
	width := m.diffVP.Width
	if width <= 0 {
		width = 80
	}
	var b strings.Builder
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
			b.WriteString(fmt.Sprintf("… %d lines above …\n", start))
		}
	}
	for i := start; i < end; i++ {
		ln := m.fileDiff.Lines[i]
		sel := i == m.cursorLine
		if m.rangeStart >= 0 {
			lo, hi := m.rangeStart, m.cursorLine
			if lo > hi {
				lo, hi = hi, lo
			}
			if i >= lo && i <= hi {
				sel = true
			}
		}
		if m.splitDiff {
			b.WriteString(diff.PaintSplitLine(m.hl, m.fileDiff.Path, ln, sel, width))
		} else {
			b.WriteString(diff.PaintDiffLine(m.hl, m.fileDiff.Path, ln, sel, width))
		}
		b.WriteByte('\n')
		for _, c := range m.comments {
			if c.Anchor != nil && c.Path == m.fileDiff.Path && c.Anchor.Line == ln.Anchor.Line && c.Anchor.Side == ln.Anchor.Side {
				b.WriteString(annotationStyle.Render("  💬 "+c.Author+": "+truncate(c.Body, width-6)) + "\n")
			}
		}
		if m.session != nil {
			for _, d := range m.session.Draft.Comments {
				if d.Anchor != nil && d.Path == m.fileDiff.Path && d.Anchor.Line == ln.Anchor.Line {
					b.WriteString(draftStyle.Render("  ✎ draft: "+truncate(d.Body, width-10)) + "\n")
				}
			}
		}
	}
	if end < len(m.fileDiff.Lines) {
		b.WriteString(fmt.Sprintf("… %d lines below …\n", len(m.fileDiff.Lines)-end))
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
		left := m.fileList.View()
		right := m.diffVP.View()
		if m.pane == paneFiles {
			left = focusedPanel.Render(left)
			right = panel.Render(right)
		} else {
			left = panel.Render(left)
			right = focusedPanel.Render(right)
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
		if m.pr != nil && m.pr.Body != "" && m.fileDiff == nil {
			body = lipgloss.JoinVertical(lipgloss.Left, mutedStyle.Render(truncate(m.pr.Body, m.width)), body)
		}
	case screenComment:
		body = lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Add comment  (ctrl+s to save, esc cancel)"),
			m.comment.View(),
		)
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
	annotationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#61afef"))
	draftStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6c07b"))
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
