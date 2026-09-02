package ai

import (
	"fmt"
	"strings"

	"github.com/vegard/prui/internal/domain"
)

// SystemPrompt is the fixed reviewer brief for summarize.
const SystemPrompt = `You are an experienced code reviewer summarizing a pull request.
Be concise and factual. Use markdown.
Cover: intent, main changes, risks/edge cases, and what to test.
Do not invent files, APIs, or behavior that are not in the provided context.`

// ContextInput is PR state used to build the user prompt.
type ContextInput struct {
	PR              *domain.PullRequest
	Files           []domain.FileChange
	Tasks           []domain.Task
	Diffs           map[string]string // path -> unified patch / raw
	MaxContextBytes int
}

// PackContext builds the user prompt, truncating diffs to MaxContextBytes.
func PackContext(in ContextInput) string {
	max := in.MaxContextBytes
	if max <= 0 {
		max = 120000
	}
	var b strings.Builder
	pr := in.PR
	if pr == nil {
		b.WriteString("Pull request: (missing)\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Title: %s\n", pr.Title)
	fmt.Fprintf(&b, "Author: %s\n", pr.Author)
	state := pr.State
	if pr.Draft {
		state += " (draft)"
	}
	if pr.Blocked {
		state += " (blocked)"
	}
	fmt.Fprintf(&b, "State: %s\n", state)
	b.WriteString("\n## Description\n")
	desc := strings.TrimSpace(pr.Body)
	if desc == "" {
		b.WriteString("(none)\n")
	} else {
		b.WriteString(desc)
		b.WriteByte('\n')
	}

	if len(in.Tasks) > 0 {
		b.WriteString("\n## Tasks\n")
		for _, t := range in.Tasks {
			mark := "[ ]"
			if t.Done {
				mark = "[x]"
			}
			fmt.Fprintf(&b, "%s %s\n", mark, strings.TrimSpace(t.Body))
		}
	}

	b.WriteString("\n## Files\n")
	if len(in.Files) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, f := range in.Files {
			fmt.Fprintf(&b, "- %s (%s)\n", f.Path, f.Status)
		}
	}

	b.WriteString("\n## Diffs\n")
	budget := max - b.Len()
	if budget < 1024 {
		b.WriteString("(truncated: no room for diffs)\n")
		return b.String()
	}

	written := 0
	for _, f := range in.Files {
		patch := strings.TrimSpace(in.Diffs[f.Path])
		if patch == "" {
			patch = strings.TrimSpace(f.Patch)
		}
		if patch == "" {
			continue
		}
		header := fmt.Sprintf("\n### %s\n", f.Path)
		remain := budget - written
		if remain < len(header)+80 {
			b.WriteString("\n(truncated: remaining diffs omitted)\n")
			break
		}
		chunk := patch
		overhead := len(header)
		if overhead+len(chunk) > remain {
			keep := remain - overhead - len("\n…(truncated)\n")
			if keep < 40 {
				b.WriteString("\n(truncated: remaining diffs omitted)\n")
				break
			}
			chunk = chunk[:keep] + "\n…(truncated)\n"
		}
		b.WriteString(header)
		b.WriteString(chunk)
		if !strings.HasSuffix(chunk, "\n") {
			b.WriteByte('\n')
		}
		written += overhead + len(chunk)
	}
	if written == 0 {
		b.WriteString("(no patches available)\n")
	}
	return b.String()
}
