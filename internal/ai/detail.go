package ai

import "strings"

// DetailLevel controls how long the summarize response should be.
type DetailLevel string

const (
	DetailShort  DetailLevel = "short"
	DetailMedium DetailLevel = "medium"
	DetailFull   DetailLevel = "full"
)

// ParseDetailLevel maps config/UI aliases to a DetailLevel (default medium).
func ParseDetailLevel(s string) DetailLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "short", "brief", "one":
		return DetailShort
	case "full", "long", "detailed":
		return DetailFull
	default:
		return DetailMedium
	}
}

func (d DetailLevel) String() string {
	switch d {
	case DetailShort:
		return "short"
	case DetailFull:
		return "full"
	default:
		return "medium"
	}
}

func (d DetailLevel) next() DetailLevel {
	switch d {
	case DetailShort:
		return DetailMedium
	case DetailMedium:
		return DetailFull
	default:
		return DetailShort
	}
}

// NextDetail cycles short → medium → full → short.
func NextDetail(d DetailLevel) DetailLevel {
	return ParseDetailLevel(d.String()).next()
}

const systemPromptBase = `You are an experienced code reviewer summarizing a pull request.
Be concise and factual. Use markdown.
Do not invent files, APIs, or behavior that are not in the provided context.`

// SystemPromptFor returns the summarize system prompt for the given detail level.
func SystemPromptFor(level DetailLevel) string {
	switch ParseDetailLevel(string(level)) {
	case DetailShort:
		return systemPromptBase + `

Length: SHORT.
Write exactly one short paragraph (roughly 2–4 sentences).
No headings, bullet lists, numbered lists, or section breaks.
Cover only the overall intent and outcome of the change.`
	case DetailFull:
		return systemPromptBase + `

Length: FULL.
No length restriction.
Cover: intent, main changes, risks/edge cases, and what to test.
Use markdown headings and bullets when they help clarity.`
	default: // medium
		return systemPromptBase + `

Length: MEDIUM.
Write at most three short paragraphs. Do not exceed three paragraphs.
Prefer prose; avoid long bullet lists (a few bullets inside the prose are OK if essential).
Cover intent, the main changes, and the most important risks or tests.`
	}
}

// SystemPrompt is the default (medium) reviewer brief.
// Prefer SystemPromptFor when a detail level is known.
var SystemPrompt = SystemPromptFor(DetailMedium)
