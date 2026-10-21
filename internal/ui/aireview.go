package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lum1n/prui/internal/ai"
	"github.com/lum1n/prui/internal/domain"
	"github.com/lum1n/prui/internal/provider"
	"github.com/lum1n/prui/internal/review"
)

// aiEditKind is what the shared textarea is editing on the AI review and
// submit screens.
type aiEditKind int

const (
	aiEditNone aiEditKind = iota
	aiEditSuggestion
	aiEditGeneral
	aiEditSummary
)

type loadedAIReviewMsg struct {
	gen    int
	review domain.AIReview
	diffs  map[string]*domain.FileDiff
	err    error
	label  string
}

// fetchDiffs returns parsed diffs for files, reusing cached entries.
func fetchDiffs(ctx context.Context, prov provider.Host, ref domain.PRRef, files []domain.FileChange, cached map[string]*domain.FileDiff) (map[string]*domain.FileDiff, error) {
	out := make(map[string]*domain.FileDiff, len(files))
	var firstErr error
	for _, f := range files {
		if fd, ok := cached[f.Path]; ok && fd != nil {
			out[f.Path] = fd
			continue
		}
		fd, err := prov.GetFileDiff(ctx, ref, f.Path)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out[f.Path] = fd
	}
	return out, firstErr
}

// openAIReview runs an AI review, or shows the stored one unless rerun is set.
func (m Model) openAIReview(rerun bool) (tea.Model, tea.Cmd) {
	if m.pr == nil {
		m.status = "No PR loaded"
		return m, nil
	}
	if m.reviewReadOnly() {
		return m.rejectWrite("AI review")
	}
	if m.session == nil {
		m.status = "No review session"
		return m, nil
	}
	if !rerun && m.session.AIReview != nil {
		m.enterAIReviewScreen()
		return m, nil
	}
	if m.opts.Config == nil || !m.opts.Config.AIConfigured() {
		m.status = "AI not configured — set ai.default and ai.providers in config.yaml"
		return m, nil
	}
	if m.aiReviewing {
		m.status = "AI review already running…"
		return m, nil
	}
	prov, _ := m.opts.Config.DefaultAIProvider()
	label := prov.Kind
	if prov.Model != "" {
		label += "/" + prov.Model
	}
	m.aiReviewGen++
	m.aiReviewing = true
	m.loading = true
	m.status = "AI reviewing with " + label + "…"
	return m, m.loadAIReview(m.aiReviewGen, label)
}

func (m Model) loadAIReview(gen int, label string) tea.Cmd {
	cfg := m.opts.Config
	host := m.opts.Host
	prov := m.opts.Provider
	pr := m.pr
	ref := domain.PRRef{Repo: m.opts.Repo, Number: m.pr.Ref.Number}
	files := append([]domain.FileChange(nil), m.files...)
	tasks := append([]domain.Task(nil), m.tasks...)
	cached := make(map[string]*domain.FileDiff, len(m.diffCache))
	for p, fd := range m.diffCache {
		cached[p] = fd
	}
	return func() tea.Msg {
		fail := func(err error) tea.Msg { return loadedAIReviewMsg{gen: gen, err: err, label: label} }
		p, err := cfg.DefaultAIProvider()
		if err != nil {
			return fail(err)
		}
		completer, err := ai.New(ai.Options{Provider: p, Config: cfg, ActiveHost: host})
		if err != nil {
			return fail(err)
		}
		timeout := cfg.AI.TimeoutSec
		if timeout <= 0 {
			timeout = 120
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		diffs, _ := fetchDiffs(ctx, prov, ref, files, cached)
		raw := map[string]string{}
		for path, fd := range diffs {
			if fd != nil && fd.Raw != "" {
				raw[path] = fd.Raw
			}
		}
		user := ai.PackReviewContext(ai.ContextInput{
			PR:              pr,
			Files:           files,
			Tasks:           tasks,
			Diffs:           raw,
			MaxContextBytes: cfg.AI.MaxContextBytes,
		})
		text, err := completer.Complete(ctx, ai.Request{System: ai.ReviewSystemPrompt, User: user, Model: p.Model})
		if err != nil {
			return loadedAIReviewMsg{gen: gen, err: err, diffs: diffs, label: label}
		}
		res, err := ai.ParseReview(text)
		if err != nil {
			return loadedAIReviewMsg{gen: gen, err: err, diffs: diffs, label: label}
		}
		return loadedAIReviewMsg{
			gen:   gen,
			diffs: diffs,
			label: label,
			review: domain.AIReview{
				Summary:     res.Summary,
				Verdict:     res.Verdict,
				Model:       label,
				Suggestions: ai.ResolveSuggestions(res.Comments, diffs),
				Created:     time.Now().UTC(),
			},
		}
	}
}

func (m Model) handleLoadedAIReview(msg loadedAIReviewMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.aiReviewGen {
		return m, nil
	}
	m.aiReviewing = false
	m.loading = false
	for p, fd := range msg.diffs {
		if _, ok := m.diffCache[p]; !ok && fd != nil {
			m.diffCache[p] = fd
		}
	}
	if m.showAll {
		m.rebuildFlat()
	}
	if msg.err != nil {
		m.status = "AI review failed: " + msg.err.Error()
		return m, nil
	}
	if m.session == nil {
		m.status = "No review session"
		return m, nil
	}
	if err := m.session.SetAIReview(msg.review); err != nil {
		m.status = "Saving AI review failed: " + err.Error()
		return m, nil
	}
	m.aiCursor = 0
	m.enterAIReviewScreen()
	m.status = fmt.Sprintf("AI review by %s · %d suggestion(s)", msg.label, len(msg.review.Suggestions))
	return m, nil
}

func (m *Model) enterAIReviewScreen() {
	m.screen = screenAIReview
	m.aiDetailOff = 0
	m.clampAICursor()
	m.layout()
}

func (m *Model) aiSuggestions() []domain.AISuggestion {
	if m.session == nil || m.session.AIReview == nil {
		return nil
	}
	return m.session.AIReview.Suggestions
}

// aiRowCount is the suggestions plus the trailing "continue" row.
func (m *Model) aiRowCount() int { return len(m.aiSuggestions()) + 1 }

func (m *Model) clampAICursor() {
	n := m.aiRowCount()
	if m.aiCursor >= n {
		m.aiCursor = n - 1
	}
	if m.aiCursor < 0 {
		m.aiCursor = 0
	}
}

// selectedSuggestion returns the suggestion under the cursor, or nil on the
// trailing row.
func (m *Model) selectedSuggestion() *domain.AISuggestion {
	sugs := m.aiSuggestions()
	if m.aiCursor < 0 || m.aiCursor >= len(sugs) {
		return nil
	}
	return m.session.Suggestion(sugs[m.aiCursor].ID)
}

func (m *Model) beginAIEdit(kind aiEditKind, id, value string) tea.Cmd {
	m.aiEdit = kind
	m.aiEditID = id
	m.comment.SetValue(value)
	m.comment.Focus()
	m.layout()
	switch kind {
	case aiEditSummary:
		m.status = "Editing review summary — ctrl+s save · esc cancel"
	case aiEditGeneral:
		m.status = "General comment — ctrl+s save · esc cancel"
	default:
		m.status = "Editing suggestion — ctrl+s save · esc cancel"
	}
	return textarea.Blink
}

func (m *Model) endAIEdit() {
	m.aiEdit = aiEditNone
	m.aiEditID = ""
	m.comment.Blur()
	m.layout()
}

// updateAIEdit drives the textarea on the AI review and submit screens.
// Enter inserts a newline (summaries are multi-line); ctrl+s saves.
func (m Model) updateAIEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.endAIEdit()
			m.status = "Edit cancelled"
			return m, nil
		case "ctrl+s":
			body := strings.TrimSpace(m.comment.Value())
			var err error
			switch m.aiEdit {
			case aiEditSummary:
				m.session.Draft.Summary = body
				err = review.Save(m.session)
				m.status = "Review summary saved"
			case aiEditGeneral:
				if body == "" {
					m.status = "Empty comment"
					return m, nil
				}
				err = m.session.AddComment(domain.DraftComment{Body: body})
				m.status = "General draft added"
			case aiEditSuggestion:
				if body == "" {
					m.status = "Empty comment — x deletes the suggestion"
					return m, nil
				}
				err = m.session.UpdateSuggestion(m.aiEditID, body)
				m.status = "Suggestion updated"
			}
			if err != nil {
				m.status = err.Error()
			}
			m.endAIEdit()
			m.refreshFileList()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.comment, cmd = m.comment.Update(msg)
	return m, cmd
}

func (m Model) updateAIReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.aiEdit != aiEditNone {
		return m.updateAIEdit(msg)
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	sg := m.selectedSuggestion()
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc":
		m.screen = screenReview
		m.layout()
		m.renderDiff()
		return m, nil
	case "j", "down":
		m.aiCursor++
		m.aiDetailOff = 0
		m.clampAICursor()
	case "k", "up":
		m.aiCursor--
		m.aiDetailOff = 0
		m.clampAICursor()
	case "g", "home":
		m.aiCursor = 0
		m.aiDetailOff = 0
	case "G", "end":
		m.aiCursor = m.aiRowCount() - 1
		m.aiDetailOff = 0
	case "ctrl+d":
		m.aiDetailOff += max(1, m.contentHeight/2)
	case "ctrl+u":
		m.aiDetailOff = max(0, m.aiDetailOff-max(1, m.contentHeight/2))
	case "a", "enter":
		if sg == nil {
			return m.openSubmit()
		}
		if sg.Status == domain.SuggestionAccepted {
			m.status = "Already accepted"
			return m, nil
		}
		if err := m.session.AcceptSuggestion(sg.ID, ""); err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.status = "Accepted → draft comment"
		m.refreshFileList()
		m.aiCursor++
		m.aiDetailOff = 0
		m.clampAICursor()
	case "ctrl+a":
		n := 0
		for _, s := range m.aiSuggestions() {
			if s.Status != domain.SuggestionAccepted {
				if err := m.session.AcceptSuggestion(s.ID, ""); err != nil {
					m.status = err.Error()
					return m, nil
				}
				n++
			}
		}
		m.refreshFileList()
		m.status = fmt.Sprintf("Accepted %d suggestion(s)", n)
	case "e":
		if sg == nil {
			return m, nil
		}
		return m, m.beginAIEdit(aiEditSuggestion, sg.ID, sg.Body)
	case "x", "d":
		if sg == nil {
			return m, nil
		}
		if err := m.session.RemoveSuggestion(sg.ID); err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.status = "Suggestion deleted"
		m.refreshFileList()
		m.aiDetailOff = 0
		m.clampAICursor()
	case "c":
		return m, m.beginAIEdit(aiEditGeneral, "", "")
	case "o":
		if sg == nil || sg.Anchor == nil {
			m.screen = screenReview
			m.layout()
			m.renderDiff()
			m.status = "Back to diff — c comment · A AI review · r submit"
			return m, nil
		}
		return m, m.jumpToAnchor(sg.Path, sg.Anchor)
	case "r":
		return m.openSubmit()
	case "ctrl+r":
		return m.openAIReview(true)
	case "?":
		m.screen = screenHelp
	}
	return m, nil
}

// openSubmit shows the submit screen with the AI verdict preselected.
func (m Model) openSubmit() (tea.Model, tea.Cmd) {
	if m.reviewReadOnly() {
		return m.rejectWrite("Submit")
	}
	m.screen = screenSubmit
	m.submitIdx = 0
	if m.session != nil && m.session.AIReview != nil {
		for i, a := range m.availableActions() {
			if a == m.session.AIReview.Verdict {
				m.submitIdx = i
			}
		}
	}
	m.layout()
	return m, nil
}

// jumpToAnchor shows path in the diff with the cursor on the anchored line.
func (m *Model) jumpToAnchor(path string, a *domain.Anchor) tea.Cmd {
	m.screen = screenReview
	m.layout()
	fd := m.diffCache[path]
	if fd == nil {
		return m.selectFile(path)
	}
	idx := -1
	for i, l := range fd.Lines {
		if l.Anchor.Side == a.Side && l.Anchor.Line == a.Line && !strings.HasPrefix(l.Text, "@@") {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = firstCommentable(fd)
	}
	m.activePath = path
	if m.showAll {
		m.rebuildFlat()
		m.cursorLine = m.jumpFlatToPath(path)
		for i, row := range m.flat {
			if !row.header && row.path == path && row.line == idx {
				m.cursorLine = i
				break
			}
		}
	} else {
		m.fileDiff = fd
		m.cursorLine = idx
	}
	m.syncFileList(path)
	m.pane = paneDiff
	m.rangeStart = -1
	m.status = fmt.Sprintf("%s:%d — A back to AI review", path, a.Line)
	m.resetDiffWindow()
	m.renderDiff()
	return nil
}

func suggestionLocation(s domain.AISuggestion) string {
	if s.Anchor == nil {
		return "general"
	}
	side := ""
	if s.Anchor.Side == domain.SideLeft {
		side = " (old)"
	}
	return fmt.Sprintf("%s:%d%s", s.Path, s.Anchor.Line, side)
}

func severityStyle(sev string) lipgloss.Style {
	switch sev {
	case "issue":
		return errorStyle
	case "nit":
		return mutedStyle
	default:
		return draftStyle
	}
}

func (m Model) aiReviewView() string {
	lw, rw := m.leftWidth, m.rightWidth
	if lw == 0 || rw == 0 || lw+rw != m.width {
		lw, rw = splitPanelWidths(m.width)
	}
	panelH := max(3, m.contentHeight)
	innerLW, innerRW, innerH := max(8, lw-2), max(8, rw-2), max(1, panelH-2)

	left := m.aiListView(innerLW, innerH)
	right := m.aiDetailView(innerRW)
	if m.aiEdit != aiEditNone {
		label := "✎ edit suggestion"
		if m.aiEdit == aiEditGeneral {
			label = "✎ new general comment"
		}
		box := lipgloss.JoinVertical(lipgloss.Left,
			draftStyle.Render(label+"  (ctrl+s save · esc cancel)"),
			m.comment.View(),
		)
		boxH := lipgloss.Height(box)
		right = clipLines(right, 0, max(1, innerH-boxH))
		right = lipgloss.JoinVertical(lipgloss.Left, right, box)
	} else {
		right = clipLines(right, m.aiDetailOff, innerH)
	}
	lsty, rsty := focusedPanel, panel
	if m.aiEdit != aiEditNone {
		lsty, rsty = panel, focusedPanel
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		renderBorderedPanel(lsty, lw, panelH, left),
		renderBorderedPanel(rsty, rw, panelH, right),
	)
}

func (m Model) aiListView(width, height int) string {
	ar := m.session.AIReview
	var head []string
	verdict := reviewActionLabel(ar.Verdict)
	head = append(head, titleStyle.Render("AI review")+" "+mutedStyle.Render(truncate(ar.Model, max(1, width-10))))
	head = append(head, "Verdict: "+draftStyle.Render(verdict))
	accepted := len(ar.Suggestions) - m.session.PendingSuggestions()
	head = append(head, mutedStyle.Render(fmt.Sprintf("%d suggestion(s) · %d accepted", len(ar.Suggestions), accepted)), "")

	var rows []string
	for i, s := range ar.Suggestions {
		mark := "[ ]"
		if s.Status == domain.SuggestionAccepted {
			mark = "[✓]"
		}
		sev := s.Severity
		if sev == "" {
			sev = "note"
		}
		first := strings.TrimSpace(strings.SplitN(s.Body, "\n", 2)[0])
		line := fmt.Sprintf("%s %s %s %s", mark, severityStyle(s.Severity).Render(sev), suggestionLocation(s), mutedStyle.Render(first))
		rows = append(rows, m.aiRow(i, line, width))
	}
	rows = append(rows, m.aiRow(len(ar.Suggestions), "➕ Add more comments / submit…", width))

	// Keep the cursor row visible under the header.
	avail := max(1, height-len(head))
	start := 0
	if m.aiCursor >= avail {
		start = m.aiCursor - avail + 1
	}
	end := min(len(rows), start+avail)
	return strings.Join(append(head, rows[start:end]...), "\n")
}

func (m Model) aiRow(i int, text string, width int) string {
	prefix := "  "
	if i == m.aiCursor {
		prefix = "▸ "
	}
	return truncateANSI(prefix+text, width)
}

func (m Model) aiDetailView(width int) string {
	ar := m.session.AIReview
	sg := m.selectedSuggestion()
	if sg == nil {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Review summary"))
		b.WriteString("\n")
		sum := strings.TrimSpace(m.session.Draft.Summary)
		if sum == "" {
			sum = strings.TrimSpace(ar.Summary)
		}
		if sum == "" {
			b.WriteString(mutedStyle.Render("(none)") + "\n")
		} else {
			b.WriteString(renderMarkdown(sum, width-2))
		}
		n := 0
		if m.session != nil {
			n = len(m.session.Draft.Comments)
		}
		fmt.Fprintf(&b, "\n%d draft comment(s) · %d pending suggestion(s)\n\n", n, m.session.PendingSuggestions())
		b.WriteString(mutedStyle.Render("c add general comment · o back to diff (c to comment on a line) · r/enter submit (s edits summary)"))
		return b.String()
	}
	var b strings.Builder
	status := "pending"
	if sg.Status == domain.SuggestionAccepted {
		status = "accepted → draft"
	}
	b.WriteString(titleStyle.Render(suggestionLocation(*sg)) + "  " + mutedStyle.Render(status) + "\n")
	if snip := m.anchorSnippet(sg, width); snip != "" {
		b.WriteString(snip + "\n")
	}
	b.WriteString(renderMarkdown(sg.Body, width-2))
	b.WriteString(mutedStyle.Render("a accept · e edit · x delete · o show in diff"))
	return b.String()
}

// anchorSnippet renders a few diff lines around the suggestion's anchor.
func (m Model) anchorSnippet(sg *domain.AISuggestion, width int) string {
	if sg.Anchor == nil {
		return ""
	}
	fd := m.diffCache[sg.Path]
	if fd == nil {
		return ""
	}
	idx := -1
	for i, l := range fd.Lines {
		if l.Anchor.Side == sg.Anchor.Side && l.Anchor.Line == sg.Anchor.Line && !strings.HasPrefix(l.Text, "@@") {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ""
	}
	var out []string
	for i := max(0, idx-3); i <= min(len(fd.Lines)-1, idx+3); i++ {
		l := fd.Lines[i]
		sign, num := " ", l.NewNumber
		switch l.Kind {
		case domain.LineAdded:
			sign = "+"
		case domain.LineRemoved:
			sign, num = "-", l.OldNumber
		}
		text := fmt.Sprintf("%4d %s %s", num, sign, l.Text)
		if strings.HasPrefix(l.Text, "@@") {
			text = l.Text
		}
		text = truncateANSI(text, width)
		switch {
		case i == idx:
			text = draftStyle.Render(text)
		case l.Kind == domain.LineAdded:
			text = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379")).Render(text)
		case l.Kind == domain.LineRemoved:
			text = errorStyle.Render(text)
		default:
			text = mutedStyle.Render(text)
		}
		out = append(out, text)
	}
	return strings.Join(out, "\n")
}

// clipLines returns at most height lines of s starting at offset.
func clipLines(s string, offset, height int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if offset > len(lines)-height {
		offset = max(0, len(lines)-height)
	}
	lines = lines[offset:]
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func aiReviewHelpLine(editing bool) string {
	if editing {
		return "enter newline · ctrl+s save · esc cancel"
	}
	return "j/k · a/enter accept · ctrl+a accept all · e edit · x delete · o show in diff · c general comment · r submit · ctrl+r rerun · ^d/^u scroll · esc back"
}

// truncateANSI cuts styled text to width cells.
func truncateANSI(s string, width int) string {
	return ansi.Truncate(strings.ReplaceAll(s, "\n", " "), width, "…")
}

// submitSummaryLines describes what will be sent on the submit screen.
func (m Model) submitSummaryLines() []string {
	if m.session == nil {
		return []string{"0 draft comment(s)", ""}
	}
	var lines []string
	sum := strings.TrimSpace(m.session.Draft.Summary)
	if sum == "" {
		lines = append(lines, mutedStyle.Render("Summary: (none — s to write one)"))
	} else {
		lines = append(lines, "Summary:")
		w := max(20, m.width-6)
		for _, l := range strings.Split(sum, "\n") {
			lines = append(lines, "  "+truncateANSI(l, w))
		}
	}
	lines = append(lines, "", fmt.Sprintf("%d draft comment(s)", len(m.session.Draft.Comments)))
	if n := m.session.PendingSuggestions(); n > 0 {
		lines = append(lines, draftStyle.Render(fmt.Sprintf("%d AI suggestion(s) not accepted — they will not be sent (A to review)", n)))
	}
	return append(lines, "")
}
