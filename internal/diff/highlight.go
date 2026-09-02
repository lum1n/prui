package diff

import (
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/vegard/prui/internal/domain"
)

// Highlighter tokenizes source lines with Chroma and maps to lipgloss styles.
type Highlighter struct {
	style *chroma.Style
	theme string
}

// NewHighlighter creates a highlighter for the given theme name (dark|light).
func NewHighlighter(theme string) *Highlighter {
	styleName := "monokai"
	if theme == "light" {
		styleName = "github"
	}
	st := styles.Get(styleName)
	if st == nil {
		st = styles.Fallback
	}
	return &Highlighter{style: st, theme: theme}
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
		style := h.lipglossFor(tok.Type)
		b.WriteString(style.Render(tok.Value))
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

// PaintDiffLine applies add/remove background and optional syntax highlight.
func PaintDiffLine(h *Highlighter, path string, line domain.DiffLine, selected bool, width int) string {
	prefix := " "
	var bg lipgloss.Color
	switch line.Kind {
	case domain.LineAdded:
		prefix = "+"
		bg = lipgloss.Color("#1a3d2a")
	case domain.LineRemoved:
		prefix = "-"
		bg = lipgloss.Color("#3d1a1a")
	default:
		if strings.HasPrefix(line.Text, "@@") || (line.OldNumber == 0 && line.NewNumber == 0) {
			prefix = " "
			bg = lipgloss.Color("#2a2a3d")
		}
	}

	oldN, newN := "    ", "    "
	if line.OldNumber > 0 {
		oldN = padNum(line.OldNumber)
	}
	if line.NewNumber > 0 {
		newN = padNum(line.NewNumber)
	}

	code := line.Text
	if h != nil && !strings.HasPrefix(line.Text, "@@") && (line.Kind != domain.LineContext || line.OldNumber > 0 || line.NewNumber > 0) {
		code = h.HighlightLine(path, line.Text)
	}

	gutter := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(oldN + " " + newN + " ")
	signStyle := lipgloss.NewStyle()
	switch line.Kind {
	case domain.LineAdded:
		signStyle = signStyle.Foreground(lipgloss.Color("#3dd68c"))
	case domain.LineRemoved:
		signStyle = signStyle.Foreground(lipgloss.Color("#f07178"))
	}
	sign := signStyle.Render(prefix)

	row := gutter + sign + " " + code
	st := lipgloss.NewStyle()
	if bg != "" {
		st = st.Background(bg)
	}
	if selected {
		st = st.Background(lipgloss.Color("#3a3a5c")).Bold(true)
	}
	if width > 0 {
		st = st.MaxWidth(width).Width(width)
	}
	return st.Render(row)
}

// PaintSplitLine renders a simplified side-by-side row (left | right).
func PaintSplitLine(h *Highlighter, path string, line domain.DiffLine, selected bool, width int) string {
	half := width / 2
	if half < 10 {
		return PaintDiffLine(h, path, line, selected, width)
	}
	left := strings.Repeat(" ", half-1)
	right := strings.Repeat(" ", half-1)
	code := line.Text
	if h != nil && !strings.HasPrefix(line.Text, "@@") {
		code = h.HighlightLine(path, line.Text)
	}
	switch line.Kind {
	case domain.LineRemoved:
		left = padNum(line.OldNumber) + " - " + code
	case domain.LineAdded:
		right = padNum(line.NewNumber) + " + " + code
	default:
		if strings.HasPrefix(line.Text, "@@") {
			return PaintDiffLine(h, path, line, selected, width)
		}
		left = padNum(line.OldNumber) + "   " + code
		right = padNum(line.NewNumber) + "   " + code
	}
	left = lipgloss.NewStyle().Width(half - 1).MaxWidth(half - 1).Render(left)
	right = lipgloss.NewStyle().Width(half - 1).MaxWidth(half - 1).Render(right)
	row := left + "│" + right
	st := lipgloss.NewStyle()
	if selected {
		st = st.Background(lipgloss.Color("#3a3a5c"))
	}
	return st.Width(width).MaxWidth(width).Render(row)
}

func padNum(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 4 {
		s = " " + s
	}
	if len(s) > 4 {
		return s[len(s)-4:]
	}
	return s
}
