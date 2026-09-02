package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
	"github.com/vegard/prui/internal/domain"
)

// Theme controls diff chrome colors (pierre/diffs-inspired, terminal-friendly).
type Theme struct {
	GutterFg     lipgloss.Color
	HunkFg       lipgloss.Color
	HunkBg       lipgloss.Color
	AddFg        lipgloss.Color
	AddBg        lipgloss.Color
	AddBar       lipgloss.Color
	DelFg        lipgloss.Color
	DelBg        lipgloss.Color
	DelBar       lipgloss.Color
	SelectedBg   lipgloss.Color
	SelectedBar  lipgloss.Color
	HeaderFg     lipgloss.Color
	HeaderBg     lipgloss.Color
	AnnotationFg lipgloss.Color
	DraftFg      lipgloss.Color
	SepFg        lipgloss.Color
}

// DarkTheme matches a calm dark review surface.
func DarkTheme() Theme {
	return Theme{
		GutterFg:     lipgloss.Color("#5c6370"),
		HunkFg:       lipgloss.Color("#7f8490"),
		HunkBg:       lipgloss.Color("#1e222a"),
		AddFg:        lipgloss.Color("#98c379"),
		AddBg:        lipgloss.Color("#1b2a1f"),
		AddBar:       lipgloss.Color("#3fa866"),
		DelFg:        lipgloss.Color("#e06c75"),
		DelBg:        lipgloss.Color("#2b1d1f"),
		DelBar:       lipgloss.Color("#c44c55"),
		SelectedBg:   lipgloss.Color("#2c313a"),
		SelectedBar:  lipgloss.Color("#e5c07b"),
		HeaderFg:     lipgloss.Color("#dcdfe4"),
		HeaderBg:     lipgloss.Color("#21252b"),
		AnnotationFg: lipgloss.Color("#61afef"),
		DraftFg:      lipgloss.Color("#e5c07b"),
		SepFg:        lipgloss.Color("#3e4451"),
	}
}

// LightTheme is a light review surface.
func LightTheme() Theme {
	return Theme{
		GutterFg:     lipgloss.Color("#6a737d"),
		HunkFg:       lipgloss.Color("#6a737d"),
		HunkBg:       lipgloss.Color("#f0f3f6"),
		AddFg:        lipgloss.Color("#22863a"),
		AddBg:        lipgloss.Color("#e6ffed"),
		AddBar:       lipgloss.Color("#28a745"),
		DelFg:        lipgloss.Color("#b31d28"),
		DelBg:        lipgloss.Color("#ffeef0"),
		DelBar:       lipgloss.Color("#d73a49"),
		SelectedBg:   lipgloss.Color("#fff8c5"),
		SelectedBar:  lipgloss.Color("#dbab09"),
		HeaderFg:     lipgloss.Color("#24292e"),
		HeaderBg:     lipgloss.Color("#f6f8fa"),
		AnnotationFg: lipgloss.Color("#0366d6"),
		DraftFg:      lipgloss.Color("#b08800"),
		SepFg:        lipgloss.Color("#d1d5da"),
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
		return line
	}
	var b strings.Builder
	for _, tok := range it.Tokens() {
		if tok.Value == "\n" {
			continue
		}
		b.WriteString(h.lipglossFor(tok.Type).Render(tok.Value))
	}
	return b.String()
}

func (h *Highlighter) lipglossFor(t chroma.TokenType) lipgloss.Style {
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

// PaintFileHeader renders a compact path strip.
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
	label := fmt.Sprintf(" %s %s ", mark, path)
	st := lipgloss.NewStyle().
		Foreground(th.HeaderFg).
		Background(th.HeaderBg).
		Bold(true)
	if width > 0 {
		st = st.Width(width).MaxWidth(width)
		return st.Render(truncate.StringWithTail(label, uint(maxInt(1, width)), "…"))
	}
	return st.Render(label)
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
	gutterStyle := lipgloss.NewStyle().Foreground(th.GutterFg).Width(gutterW).Align(lipgloss.Right)

	oldG := gutterStyle.Render(gutterNum(line.OldNumber))
	newG := gutterStyle.Render(gutterNum(line.NewNumber))

	bar := "│"
	barStyle := lipgloss.NewStyle().Width(1).Foreground(th.SepFg)
	var rowBg lipgloss.Color
	sign := " "

	switch line.Kind {
	case domain.LineAdded:
		bar = "┃"
		barStyle = lipgloss.NewStyle().Width(1).Foreground(th.AddBar)
		rowBg = th.AddBg
		sign = "+"
		newG = lipgloss.NewStyle().Foreground(th.AddFg).Width(gutterW).Align(lipgloss.Right).Render(gutterNum(line.NewNumber))
		oldG = gutterStyle.Render(" ")
	case domain.LineRemoved:
		bar = "┃"
		barStyle = lipgloss.NewStyle().Width(1).Foreground(th.DelBar)
		rowBg = th.DelBg
		sign = "−"
		oldG = lipgloss.NewStyle().Foreground(th.DelFg).Width(gutterW).Align(lipgloss.Right).Render(gutterNum(line.OldNumber))
		newG = gutterStyle.Render(" ")
	}

	if selected {
		bar = "▌"
		barStyle = lipgloss.NewStyle().Width(1).Foreground(th.SelectedBar)
		rowBg = th.SelectedBg
	}

	code := line.Text
	if h != nil {
		code = h.HighlightLine(path, line.Text)
	}

	signSt := lipgloss.NewStyle().Width(1).Foreground(th.GutterFg)
	switch line.Kind {
	case domain.LineAdded:
		signSt = lipgloss.NewStyle().Width(1).Foreground(th.AddFg)
	case domain.LineRemoved:
		signSt = lipgloss.NewStyle().Width(1).Foreground(th.DelFg)
	}

	chromeW := gutterW + 1 + gutterW + 1 + 1 + 1 + 1
	codeW := width - chromeW
	if codeW < 8 {
		codeW = 8
	}
	code = lipgloss.NewStyle().MaxWidth(codeW).Render(code)

	row := oldG + lipgloss.NewStyle().Foreground(th.SepFg).Render("│") + newG +
		barStyle.Render(bar) + signSt.Render(sign) + " " + code

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

	code := line.Text
	if h != nil {
		code = h.HighlightLine(path, line.Text)
	}

	empty := func(w int) string {
		return lipgloss.NewStyle().Width(w).MaxWidth(w).Render("")
	}
	side := func(num int, kind domain.LineType, content string, w int) string {
		g := lipgloss.NewStyle().Foreground(th.GutterFg).Width(4).Align(lipgloss.Right).Render(gutterNum(num))
		barCol := th.SepFg
		var bg lipgloss.Color
		sign := " "
		switch kind {
		case domain.LineAdded:
			barCol = th.AddBar
			bg = th.AddBg
			sign = "+"
		case domain.LineRemoved:
			barCol = th.DelBar
			bg = th.DelBg
			sign = "−"
		}
		if selected {
			barCol = th.SelectedBar
			bg = th.SelectedBg
		}
		bar := lipgloss.NewStyle().Foreground(barCol).Render("┃")
		signS := lipgloss.NewStyle().Foreground(barCol).Render(sign)
		innerW := w - 4 - 1 - 1 - 1
		if innerW < 4 {
			innerW = 4
		}
		body := lipgloss.NewStyle().MaxWidth(innerW).Render(content)
		row := g + bar + signS + " " + body
		st := lipgloss.NewStyle().Width(w).MaxWidth(w)
		if bg != "" {
			st = st.Background(bg)
		}
		return st.Render(row)
	}

	var left, right string
	switch line.Kind {
	case domain.LineRemoved:
		left = side(line.OldNumber, domain.LineRemoved, code, leftW)
		right = empty(rightW)
	case domain.LineAdded:
		left = empty(leftW)
		right = side(line.NewNumber, domain.LineAdded, code, rightW)
	default:
		left = side(line.OldNumber, domain.LineContext, code, leftW)
		right = side(line.NewNumber, domain.LineContext, code, rightW)
	}
	return left + right
}

// PaintAnnotation renders a quiet annotation under a line.
func PaintAnnotation(author, body string, draft bool, th Theme, width int) string {
	prefix := "     └ "
	fg := th.AnnotationFg
	if draft {
		prefix = "     └ draft · "
		fg = th.DraftFg
	}
	label := prefix + author + " · " + strings.ReplaceAll(body, "\n", " ")
	st := lipgloss.NewStyle().Foreground(fg)
	if width > 0 {
		st = st.Width(width).MaxWidth(width)
		return st.Render(truncate.StringWithTail(label, uint(maxInt(1, width)), "…"))
	}
	return st.Render(label)
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
