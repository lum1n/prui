package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/truncate"
	"github.com/muesli/reflow/wordwrap"
	"github.com/vegard/prui/internal/domain"
)

// Theme controls diff chrome colors (pierre/diffs-inspired, terminal-friendly).
type Theme struct {
	GutterFg         lipgloss.Color
	HunkFg           lipgloss.Color
	HunkBg           lipgloss.Color
	AddFg            lipgloss.Color
	AddBg            lipgloss.Color
	AddBar           lipgloss.Color
	DelFg            lipgloss.Color
	DelBg            lipgloss.Color
	DelBar           lipgloss.Color
	SelectedBg       lipgloss.Color
	SelectedBar      lipgloss.Color
	SelectedGutterBg lipgloss.Color
	SelectedGutterFg lipgloss.Color
	HeaderFg         lipgloss.Color
	HeaderBg         lipgloss.Color
	AnnotationFg     lipgloss.Color
	DraftFg          lipgloss.Color
	SepFg            lipgloss.Color
}

// DarkTheme matches a calm dark review surface.
func DarkTheme() Theme {
	return Theme{
		GutterFg:         lipgloss.Color("#5c6370"),
		HunkFg:           lipgloss.Color("#7f8490"),
		HunkBg:           lipgloss.Color("#1e222a"),
		AddFg:            lipgloss.Color("#98c379"),
		AddBg:            lipgloss.Color("#1e3a28"),
		AddBar:           lipgloss.Color("#3fa866"),
		DelFg:            lipgloss.Color("#e06c75"),
		DelBg:            lipgloss.Color("#3a1e22"),
		DelBar:           lipgloss.Color("#c44c55"),
		SelectedBg:       lipgloss.Color("#3a4559"), // cool blue-gray — contrasts green/red
		SelectedBar:      lipgloss.Color("#ffcc66"),
		SelectedGutterBg: lipgloss.Color("#ffcc66"),
		SelectedGutterFg: lipgloss.Color("#1a1d23"),
		HeaderFg:         lipgloss.Color("#dcdfe4"),
		HeaderBg:         lipgloss.Color("#21252b"),
		AnnotationFg:     lipgloss.Color("#61afef"),
		DraftFg:          lipgloss.Color("#e5c07b"),
		SepFg:            lipgloss.Color("#3e4451"),
	}
}

// LightTheme is a light review surface.
func LightTheme() Theme {
	return Theme{
		GutterFg:         lipgloss.Color("#6a737d"),
		HunkFg:           lipgloss.Color("#6a737d"),
		HunkBg:           lipgloss.Color("#f0f3f6"),
		AddFg:            lipgloss.Color("#22863a"),
		AddBg:            lipgloss.Color("#e6ffed"),
		AddBar:           lipgloss.Color("#28a745"),
		DelFg:            lipgloss.Color("#b31d28"),
		DelBg:            lipgloss.Color("#ffeef0"),
		DelBar:           lipgloss.Color("#d73a49"),
		SelectedBg:       lipgloss.Color("#cce0ff"),
		SelectedBar:      lipgloss.Color("#0969da"),
		SelectedGutterBg: lipgloss.Color("#0969da"),
		SelectedGutterFg: lipgloss.Color("#ffffff"),
		HeaderFg:         lipgloss.Color("#24292e"),
		HeaderBg:         lipgloss.Color("#f6f8fa"),
		AnnotationFg:     lipgloss.Color("#0366d6"),
		DraftFg:          lipgloss.Color("#b08800"),
		SepFg:            lipgloss.Color("#d1d5da"),
	}
}

// Highlighter tokenizes source lines with Chroma.
type Highlighter struct {
	style *chroma.Style
	theme string
}

// NewHighlighter creates a highlighter for dark|light.
func NewHighlighter(theme string) *Highlighter {
	styleName := "onedark"
	if theme == "light" {
		styleName = "github"
	}
	st := styles.Get(styleName)
	if st == nil {
		st = styles.Get("monokai")
	}
	if st == nil {
		st = styles.Fallback
	}
	return &Highlighter{style: st, theme: theme}
}

// ThemeFor returns chrome colors for the highlighter theme name.
func ThemeFor(name string) Theme {
	if name == "light" {
		return LightTheme()
	}
	return DarkTheme()
}

// HighlightLine returns a lipgloss-styled string for one source line.
func (h *Highlighter) HighlightLine(path, line string) string {
	return h.HighlightLineBG(path, line, "")
}

// HighlightLineBG highlights a line and paints every token with bg so nested
// ANSI resets do not wipe the add/remove row tint.
func (h *Highlighter) HighlightLineBG(path, line string, bg lipgloss.Color) string {
	if line == "" {
		return ""
	}
	lexer := lexers.Get(LanguageFromPath(path))
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, line+"\n")
	if err != nil {
		st := lipgloss.NewStyle()
		if bg != "" {
			st = st.Background(bg)
		}
		return st.Render(line)
	}
	var b strings.Builder
	for _, tok := range it.Tokens() {
		if tok.Value == "\n" {
			continue
		}
		b.WriteString(h.lipglossFor(tok.Type, bg).Render(tok.Value))
	}
	return b.String()
}

func (h *Highlighter) lipglossFor(t chroma.TokenType, bg lipgloss.Color) lipgloss.Style {
	entry := h.style.Get(t)
	st := lipgloss.NewStyle()
	if entry.Bold == chroma.Yes {
		st = st.Bold(true)
	}
	if entry.Italic == chroma.Yes {
		st = st.Italic(true)
	}
	if entry.Colour.IsSet() {
		st = st.Foreground(lipgloss.Color(entry.Colour.String()))
	}
	if bg != "" {
		st = st.Background(bg)
	}
	return st
}

// Options configures a single painted row.
type Options struct {
	Theme    Theme
	Width    int
	Selected bool
	Split    bool
}

// IsHunkHeader reports whether the line is a @@ hunk header.
func IsHunkHeader(line domain.DiffLine) bool {
	return strings.HasPrefix(line.Text, "@@")
}

// PaintFileHeader renders a named file section strip (pierre-style).
func PaintFileHeader(path string, status domain.FileStatus, th Theme, width int) string {
	mark := "±"
	switch status {
	case domain.FileAdded:
		mark = "+"
	case domain.FileRemoved:
		mark = "−"
	case domain.FileRenamed:
		mark = "→"
	}
	if width <= 0 {
		width = 80
	}
	rule := lipgloss.NewStyle().Foreground(th.SepFg).Render(strings.Repeat("─", width))
	label := fmt.Sprintf(" %s  %s ", mark, path)
	st := lipgloss.NewStyle().
		Foreground(th.HeaderFg).
		Background(th.HeaderBg).
		Bold(true).
		Width(width).
		MaxWidth(width)
	title := st.Render(truncate.StringWithTail(label, uint(width), "…"))
	return rule + "\n" + title + "\n" + rule
}

// PaintDiffLine paints one unified-diff row.
func PaintDiffLine(h *Highlighter, path string, line domain.DiffLine, selected bool, width int) string {
	return Paint(h, path, line, Options{Theme: ThemeFor(themeOf(h)), Width: width, Selected: selected})
}

// PaintSplitLine paints one split-diff row.
func PaintSplitLine(h *Highlighter, path string, line domain.DiffLine, selected bool, width int) string {
	return Paint(h, path, line, Options{Theme: ThemeFor(themeOf(h)), Width: width, Selected: selected, Split: true})
}

func themeOf(h *Highlighter) string {
	if h == nil {
		return "dark"
	}
	return h.theme
}

// Paint renders a diff row with pierre-inspired chrome.
func Paint(h *Highlighter, path string, line domain.DiffLine, opt Options) string {
	th := opt.Theme
	width := opt.Width
	if width <= 0 {
		width = 80
	}
	if IsHunkHeader(line) {
		return paintHunk(line.Text, th, width, opt.Selected)
	}
	if opt.Split {
		return paintSplit(h, path, line, th, width, opt.Selected)
	}
	return paintUnified(h, path, line, th, width, opt.Selected)
}

func paintHunk(text string, th Theme, width int, selected bool) string {
	rest := strings.TrimSpace(strings.TrimPrefix(text, "@@"))
	rest = strings.TrimSpace(strings.TrimSuffix(rest, "@@"))
	label := " ··· " + rest + " "
	st := lipgloss.NewStyle().Foreground(th.HunkFg).Background(th.HunkBg)
	if selected {
		st = st.Background(th.SelectedBg)
	}
	return st.Width(width).MaxWidth(width).Render(truncate.StringWithTail(label, uint(width), "…"))
}

func paintUnified(h *Highlighter, path string, line domain.DiffLine, th Theme, width int, selected bool) string {
	const gutterW = 5

	bar := "│"
	barCol := th.SepFg
	var rowBg lipgloss.Color
	sign := " "
	numFg := th.GutterFg

	switch line.Kind {
	case domain.LineAdded:
		bar = "┃"
		barCol = th.AddBar
		rowBg = th.AddBg
		sign = "+"
		numFg = th.AddFg
	case domain.LineRemoved:
		bar = "┃"
		barCol = th.DelBar
		rowBg = th.DelBg
		sign = "−"
		numFg = th.DelFg
	}

	if selected {
		// Always override add/remove tint — selection must read clearly on colored rows.
		bar = "▌"
		barCol = th.SelectedBar
		rowBg = th.SelectedBg
	}

	withBG := func(st lipgloss.Style) lipgloss.Style {
		if rowBg != "" {
			return st.Background(rowBg)
		}
		return st
	}

	gutterStyle := withBG(lipgloss.NewStyle().Foreground(th.GutterFg).Width(gutterW).Align(lipgloss.Right))
	numStyle := withBG(lipgloss.NewStyle().Foreground(numFg).Width(gutterW).Align(lipgloss.Right).Bold(selected))
	if selected {
		// Inverted gutter chip so the cursor line is obvious even on busy syntax.
		numStyle = lipgloss.NewStyle().
			Foreground(th.SelectedGutterFg).
			Background(th.SelectedGutterBg).
			Width(gutterW).
			Align(lipgloss.Right).
			Bold(true)
		gutterStyle = lipgloss.NewStyle().
			Foreground(th.SelectedGutterFg).
			Background(th.SelectedGutterBg).
			Width(gutterW).
			Align(lipgloss.Right)
	}

	var oldG, newG string
	switch line.Kind {
	case domain.LineAdded:
		oldG = gutterStyle.Render(" ")
		newG = numStyle.Render(gutterNum(line.NewNumber))
	case domain.LineRemoved:
		oldG = numStyle.Render(gutterNum(line.OldNumber))
		newG = gutterStyle.Render(" ")
	default:
		oldG = gutterStyle.Render(gutterNum(line.OldNumber))
		newG = numStyle.Render(gutterNum(line.NewNumber))
	}

	code := line.Text
	if h != nil {
		code = h.HighlightLineBG(path, line.Text, rowBg)
	} else if rowBg != "" {
		code = lipgloss.NewStyle().Background(rowBg).Render(code)
	}

	signSt := withBG(lipgloss.NewStyle().Width(1).Foreground(th.GutterFg).Bold(selected))
	switch line.Kind {
	case domain.LineAdded:
		signSt = withBG(lipgloss.NewStyle().Width(1).Foreground(th.AddFg).Bold(selected))
	case domain.LineRemoved:
		signSt = withBG(lipgloss.NewStyle().Width(1).Foreground(th.DelFg).Bold(selected))
	}
	if selected {
		signSt = withBG(lipgloss.NewStyle().Width(1).Foreground(th.SelectedBar).Bold(true))
	}

	chromeW := gutterW + 1 + gutterW + 1 + 1 + 1 + 1
	codeW := width - chromeW
	if codeW < 8 {
		codeW = 8
	}
	code = withBG(lipgloss.NewStyle().Width(codeW).MaxWidth(codeW).Bold(selected)).Render(code)

	sepCol := th.SepFg
	if selected {
		sepCol = th.SelectedBar
	}
	sep := withBG(lipgloss.NewStyle().Foreground(sepCol)).Render("│")
	barS := withBG(lipgloss.NewStyle().Width(1).Foreground(barCol).Bold(selected)).Render(bar)
	gap := withBG(lipgloss.NewStyle()).Render(" ")

	row := oldG + sep + newG + barS + signSt.Render(sign) + gap + code

	st := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if rowBg != "" {
		st = st.Background(rowBg)
	}
	return st.Render(row)
}

func paintSplit(h *Highlighter, path string, line domain.DiffLine, th Theme, width int, selected bool) string {
	half := width / 2
	if half < 16 {
		return paintUnified(h, path, line, th, width, selected)
	}
	leftW := half
	rightW := width - half

	empty := func(w int) string {
		bg := lipgloss.NewStyle().Width(w).MaxWidth(w)
		if selected {
			bg = bg.Background(th.SelectedBg)
		}
		return bg.Render("")
	}
	side := func(num int, kind domain.LineType, raw string, w int) string {
		barCol := th.SepFg
		var bg lipgloss.Color
		sign := " "
		numFg := th.GutterFg
		switch kind {
		case domain.LineAdded:
			barCol = th.AddBar
			bg = th.AddBg
			sign = "+"
			numFg = th.AddFg
		case domain.LineRemoved:
			barCol = th.DelBar
			bg = th.DelBg
			sign = "−"
			numFg = th.DelFg
		}
		if selected {
			barCol = th.SelectedBar
			bg = th.SelectedBg
		}
		withBG := func(st lipgloss.Style) lipgloss.Style {
			if bg != "" {
				return st.Background(bg)
			}
			return st
		}
		gStyle := withBG(lipgloss.NewStyle().Foreground(numFg).Width(4).Align(lipgloss.Right).Bold(selected))
		if selected {
			gStyle = lipgloss.NewStyle().
				Foreground(th.SelectedGutterFg).
				Background(th.SelectedGutterBg).
				Width(4).
				Align(lipgloss.Right).
				Bold(true)
		}
		g := gStyle.Render(gutterNum(num))
		bar := withBG(lipgloss.NewStyle().Foreground(barCol).Bold(selected)).Render("▌")
		if !selected {
			bar = withBG(lipgloss.NewStyle().Foreground(barCol)).Render("┃")
		}
		signS := withBG(lipgloss.NewStyle().Foreground(barCol).Bold(selected)).Render(sign)
		gap := withBG(lipgloss.NewStyle()).Render(" ")
		innerW := w - 4 - 1 - 1 - 1
		if innerW < 4 {
			innerW = 4
		}
		var body string
		if h != nil {
			body = h.HighlightLineBG(path, raw, bg)
		} else if bg != "" {
			body = lipgloss.NewStyle().Background(bg).Render(raw)
		} else {
			body = raw
		}
		body = withBG(lipgloss.NewStyle().Width(innerW).MaxWidth(innerW).Bold(selected)).Render(body)
		row := g + bar + signS + gap + body
		st := lipgloss.NewStyle().Width(w).MaxWidth(w)
		if bg != "" {
			st = st.Background(bg)
		}
		return st.Render(row)
	}

	var left, right string
	switch line.Kind {
	case domain.LineRemoved:
		left = side(line.OldNumber, domain.LineRemoved, line.Text, leftW)
		right = empty(rightW)
	case domain.LineAdded:
		left = empty(leftW)
		right = side(line.NewNumber, domain.LineAdded, line.Text, rightW)
	default:
		left = side(line.OldNumber, domain.LineContext, line.Text, leftW)
		right = side(line.NewNumber, domain.LineContext, line.Text, rightW)
	}
	return left + right
}

// PaintAnnotation renders a quiet annotation under a line, wrapping long bodies.
func PaintAnnotation(author, body string, draft, selected bool, th Theme, width int) string {
	prefix := "     └ "
	fg := th.AnnotationFg
	if draft {
		prefix = "     └ draft · "
		fg = th.DraftFg
	}
	if selected {
		prefix = "     ▸ "
	}
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	head := prefix + author + " · "
	st := lipgloss.NewStyle().Foreground(fg)
	if selected {
		st = st.Bold(true).Foreground(th.DraftFg)
	}
	if width <= 0 {
		return st.Render(head + strings.ReplaceAll(body, "\n", " "))
	}

	headW := runewidth.StringWidth(head)
	cont := strings.Repeat(" ", minInt(headW, width))
	bodyWidth := maxInt(8, width-headW)
	if headW >= width-4 {
		// Narrow pane: stack author then wrapped body under a short indent.
		head = prefix + author
		headW = runewidth.StringWidth(prefix)
		cont = strings.Repeat(" ", headW) + "  "
		bodyWidth = maxInt(8, width-runewidth.StringWidth(cont))
		var b strings.Builder
		b.WriteString(st.Render(truncate.StringWithTail(head, uint(width), "…")))
		for _, para := range strings.Split(body, "\n") {
			wrapped := wordwrap.String(para, bodyWidth)
			for _, line := range strings.Split(wrapped, "\n") {
				b.WriteByte('\n')
				b.WriteString(st.Render(cont + line))
			}
		}
		return b.String()
	}

	var b strings.Builder
	paras := strings.Split(body, "\n")
	first := true
	for _, para := range paras {
		wrapped := wordwrap.String(para, bodyWidth)
		for i, line := range strings.Split(wrapped, "\n") {
			if !first {
				b.WriteByte('\n')
			}
			first = false
			if i == 0 && b.Len() == 0 {
				b.WriteString(st.Render(head + line))
				continue
			}
			b.WriteString(st.Render(cont + line))
		}
	}
	if first {
		return st.Render(truncate.StringWithTail(head, uint(width), "…"))
	}
	return b.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func gutterNum(n int) string {
	if n <= 0 {
		return " "
	}
	return strconv.Itoa(n)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
