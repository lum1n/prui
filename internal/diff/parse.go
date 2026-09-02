package diff

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/lum1n/prui/internal/domain"
)

var hunkHeader = regexp.MustCompile(`^@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`)

// ParseUnified parses a unified diff patch into a FileDiff.
// pathHint is used when the patch lacks a/b headers.
func ParseUnified(pathHint, patch string) (*domain.FileDiff, error) {
	fd := &domain.FileDiff{
		Path:   pathHint,
		Status: domain.FileModified,
		Raw:    patch,
	}

	sc := bufio.NewScanner(strings.NewReader(patch))
	// allow long lines
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	var (
		oldLine, newLine int
		hunkIdx          = -1
		inHunk           bool
	)

	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git"):
			// diff --git a/foo b/bar
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				fd.OldPath = strings.TrimPrefix(fields[2], "a/")
				fd.Path = strings.TrimPrefix(fields[3], "b/")
			}
		case strings.HasPrefix(line, "--- "):
			p := strings.TrimPrefix(line, "--- ")
			p = strings.TrimSpace(strings.Split(p, "\t")[0])
			if p == "/dev/null" {
				fd.Status = domain.FileAdded
			} else {
				fd.OldPath = strings.TrimPrefix(p, "a/")
			}
		case strings.HasPrefix(line, "+++ "):
			p := strings.TrimPrefix(line, "+++ ")
			p = strings.TrimSpace(strings.Split(p, "\t")[0])
			if p == "/dev/null" {
				fd.Status = domain.FileRemoved
				if fd.Path == "" && fd.OldPath != "" {
					fd.Path = fd.OldPath
				}
			} else {
				fd.Path = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "rename from "):
			fd.OldPath = strings.TrimPrefix(line, "rename from ")
			fd.Status = domain.FileRenamed
		case strings.HasPrefix(line, "rename to "):
			fd.Path = strings.TrimPrefix(line, "rename to ")
			fd.Status = domain.FileRenamed
		case strings.HasPrefix(line, "@@"):
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			hunkIdx++
			inHunk = true
			oldLine, _ = strconv.Atoi(m[1])
			newLine, _ = strconv.Atoi(m[3])
			fd.Lines = append(fd.Lines, domain.DiffLine{
				Kind:      domain.LineContext,
				Text:      line,
				HunkIndex: hunkIdx,
			})
		case !inHunk:
			continue
		case strings.HasPrefix(line, "+"):
			text := trimPrefixOne(line, "+")
			anchor := domain.Anchor{
				Path:     fd.Path,
				Side:     domain.SideRight,
				Line:     newLine,
				LineType: domain.LineAdded,
			}
			fd.Lines = append(fd.Lines, domain.DiffLine{
				Kind:      domain.LineAdded,
				NewNumber: newLine,
				Text:      text,
				HunkIndex: hunkIdx,
				Anchor:    anchor,
			})
			newLine++
		case strings.HasPrefix(line, "-"):
			text := trimPrefixOne(line, "-")
			path := fd.OldPath
			if path == "" {
				path = fd.Path
			}
			anchor := domain.Anchor{
				Path:     path,
				Side:     domain.SideLeft,
				Line:     oldLine,
				LineType: domain.LineRemoved,
			}
			fd.Lines = append(fd.Lines, domain.DiffLine{
				Kind:      domain.LineRemoved,
				OldNumber: oldLine,
				Text:      text,
				HunkIndex: hunkIdx,
				Anchor:    anchor,
			})
			oldLine++
		case strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file"
			fd.Lines = append(fd.Lines, domain.DiffLine{
				Kind:      domain.LineContext,
				Text:      line,
				HunkIndex: hunkIdx,
			})
		default:
			// context line (may start with space)
			text := trimPrefixOne(line, " ")
			anchor := domain.Anchor{
				Path:     fd.Path,
				Side:     domain.SideRight,
				Line:     newLine,
				LineType: domain.LineContext,
			}
			fd.Lines = append(fd.Lines, domain.DiffLine{
				Kind:      domain.LineContext,
				OldNumber: oldLine,
				NewNumber: newLine,
				Text:      text,
				HunkIndex: hunkIdx,
				Anchor:    anchor,
			})
			oldLine++
			newLine++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan patch: %w", err)
	}
	if fd.Path == "" {
		fd.Path = pathHint
	}
	return fd, nil
}

func trimPrefixOne(s, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}

// LanguageFromPath guesses a chroma lexer name from a file path.
func LanguageFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".cs":
		return "c#"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "c++"
	case ".swift":
		return "swift"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".sh", ".bash", ".zsh":
		return "bash"
	case ".css":
		return "css"
	case ".html", ".htm":
		return "html"
	case ".sql":
		return "sql"
	case ".xml":
		return "xml"
	default:
		return "plaintext"
	}
}

// Commentable returns true if the line can receive an inline comment.
func Commentable(line domain.DiffLine) bool {
	if IsHunkHeader(line) {
		return false
	}
	return line.Kind == domain.LineAdded || line.Kind == domain.LineRemoved ||
		(line.Kind == domain.LineContext && (line.NewNumber > 0 || line.OldNumber > 0))
}
