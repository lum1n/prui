package ai

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
)

// ReviewSystemPrompt asks the model for a structured review as JSON.
const ReviewSystemPrompt = `You are an experienced code reviewer reviewing a pull request.
Do not invent files, APIs, or behavior that are not in the provided context.

Each diff line is prefixed with a location: R<n> is line n of the new file,
L<n> is line n of the old file (only for removed lines).

Reply with JSON only, no prose and no code fences, in exactly this shape:
{
  "summary": "short overall review in markdown",
  "verdict": "COMMENT" | "APPROVE" | "REQUEST_CHANGES",
  "comments": [
    {"path": "file path from the diff", "line": 123, "side": "RIGHT" | "LEFT",
     "severity": "issue" | "suggestion" | "nit" | "question", "body": "markdown comment"}
  ]
}

Rules:
- Only comment on lines shown in the diff. Use the R/L number from the prefix:
  side RIGHT for R<n>, side LEFT for L<n>.
- Prefer real problems: bugs, missing error handling, security, broken edge cases.
  Keep nits few. Skip praise and restating what the code does.
- Each comment must stand alone and say what to change and why.
- Use REQUEST_CHANGES only for issues that should block merging.
- An empty comments list is fine when there is nothing worth saying.`

// PackReviewContext is PackContext with diffs annotated by line number, so
// the model can point at exact lines.
func PackReviewContext(in ContextInput) string {
	annotated := make(map[string]string, len(in.Files))
	for _, f := range in.Files {
		patch := in.Diffs[f.Path]
		if strings.TrimSpace(patch) == "" {
			patch = f.Patch
		}
		if strings.TrimSpace(patch) == "" {
			continue
		}
		annotated[f.Path] = AnnotateDiff(f.Path, patch)
	}
	in.Diffs = annotated
	return PackContext(in)
}

// AnnotateDiff prefixes each diff line with its R<new>/L<old> line number.
// It returns the patch unchanged if it cannot be parsed.
func AnnotateDiff(path, patch string) string {
	fd, err := diff.ParseUnified(path, patch)
	if err != nil || fd == nil || len(fd.Lines) == 0 {
		return patch
	}
	var b strings.Builder
	for _, l := range fd.Lines {
		if diff.IsHunkHeader(l) {
			b.WriteString(l.Text)
			b.WriteByte('\n')
			continue
		}
		switch l.Kind {
		case domain.LineAdded:
			fmt.Fprintf(&b, "R%d +%s\n", l.NewNumber, l.Text)
		case domain.LineRemoved:
			fmt.Fprintf(&b, "L%d -%s\n", l.OldNumber, l.Text)
		default:
			fmt.Fprintf(&b, "R%d  %s\n", l.NewNumber, l.Text)
		}
	}
	return b.String()
}

// ReviewComment is one inline comment as returned by the model.
type ReviewComment struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"`
	Severity string `json:"severity"`
	Body     string `json:"body"`
}

// ReviewResult is the parsed model response.
type ReviewResult struct {
	Summary  string              `json:"summary"`
	Verdict  domain.ReviewAction `json:"-"`
	Comments []ReviewComment     `json:"comments"`
}

// ParseReview extracts the JSON review from a model reply. Code fences and
// text around the JSON object are ignored.
func ParseReview(text string) (ReviewResult, error) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ReviewResult{}, fmt.Errorf("no JSON review in reply: %q", snippet(text))
	}
	var raw struct {
		ReviewResult
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &raw); err != nil {
		return ReviewResult{}, fmt.Errorf("parse review JSON: %v: %q", err, snippet(text))
	}
	out := raw.ReviewResult
	out.Summary = strings.TrimSpace(out.Summary)
	out.Verdict = ParseVerdict(raw.Verdict)
	kept := out.Comments[:0]
	for _, c := range out.Comments {
		c.Body = strings.TrimSpace(c.Body)
		if c.Body != "" {
			kept = append(kept, c)
		}
	}
	out.Comments = kept
	return out, nil
}

// ParseVerdict maps model verdict spellings to a ReviewAction (default comment).
func ParseVerdict(s string) domain.ReviewAction {
	v := strings.ToUpper(strings.TrimSpace(s))
	v = strings.NewReplacer(" ", "_", "-", "_").Replace(v)
	switch v {
	case "APPROVE", "APPROVED", "LGTM":
		return domain.ActionApprove
	case "REQUEST_CHANGES", "CHANGES_REQUESTED", "REQUEST_CHANGE", "NEEDS_WORK":
		return domain.ActionRequestChanges
	default:
		return domain.ActionComment
	}
}

// ResolveSuggestions anchors model comments to commentable diff lines. A
// comment that matches no line becomes a general comment that names the
// location in its body, so it is not lost.
func ResolveSuggestions(comments []ReviewComment, diffs map[string]*domain.FileDiff) []domain.AISuggestion {
	out := make([]domain.AISuggestion, 0, len(comments))
	for _, c := range comments {
		sg := domain.AISuggestion{
			Body:     c.Body,
			Severity: strings.ToLower(strings.TrimSpace(c.Severity)),
			Status:   domain.SuggestionPending,
		}
		path := strings.TrimSpace(c.Path)
		if a := findAnchor(diffs[path], c); a != nil {
			sg.Path = path
			sg.Anchor = a
		} else if path != "" {
			loc := path
			if c.Line > 0 {
				loc += ":" + strconv.Itoa(c.Line)
			}
			sg.Body = "`" + loc + "`: " + c.Body
		}
		out = append(out, sg)
	}
	return out
}

func findAnchor(fd *domain.FileDiff, c ReviewComment) *domain.Anchor {
	if fd == nil || c.Line <= 0 {
		return nil
	}
	left := strings.EqualFold(strings.TrimSpace(c.Side), string(domain.SideLeft))
	for _, l := range fd.Lines {
		if !diff.Commentable(l) {
			continue
		}
		if left {
			if l.Kind != domain.LineRemoved || l.OldNumber != c.Line {
				continue
			}
		} else if l.Kind == domain.LineRemoved || l.NewNumber != c.Line {
			continue
		}
		a := l.Anchor
		if a.Path == "" {
			a.Path = fd.Path
		}
		return &a
	}
	return nil
}

func snippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
