package ui

import (
	"sort"
	"strings"

	"github.com/vegard/prui/internal/diff"
	"github.com/vegard/prui/internal/domain"
)

type threadNode struct {
	Comment *domain.Comment
	Draft   *domain.DraftComment
	Depth   int
}

func sameLineAnchor(a *domain.Anchor, ln domain.DiffLine) bool {
	if a == nil || a.Line <= 0 {
		return false
	}
	sideOK := a.Side == "" || ln.Anchor.Side == "" || a.Side == ln.Anchor.Side
	if !sideOK {
		return false
	}
	if a.Line == ln.Anchor.Line {
		return true
	}
	if a.EndLine > a.Line && ln.Anchor.Line >= a.Line && ln.Anchor.Line <= a.EndLine {
		return true
	}
	return false
}

// lineThread flattens roots + replies for a diff line into display order.
func lineThread(all []domain.Comment, drafts []domain.DraftComment, path string, ln domain.DiffLine) []threadNode {
	onLineIDs := map[string]bool{}
	onLine := make([]domain.Comment, 0)
	for _, c := range all {
		if c.Path != path {
			continue
		}
		if sameLineAnchor(c.Anchor, ln) {
			onLine = append(onLine, c)
			onLineIDs[c.ID] = true
		}
	}
	// Pull in descendants that reply into this thread (may lack anchors).
	changed := true
	for changed {
		changed = false
		for _, c := range all {
			if onLineIDs[c.ID] || c.ParentID == "" || !onLineIDs[c.ParentID] {
				continue
			}
			onLine = append(onLine, c)
			onLineIDs[c.ID] = true
			changed = true
		}
	}

	byParent := map[string][]domain.Comment{}
	roots := make([]domain.Comment, 0)
	for _, c := range onLine {
		if c.ParentID == "" || !onLineIDs[c.ParentID] {
			roots = append(roots, c)
			continue
		}
		byParent[c.ParentID] = append(byParent[c.ParentID], c)
	}
	sort.SliceStable(roots, func(i, j int) bool { return roots[i].Created.Before(roots[j].Created) })
	for k := range byParent {
		sort.SliceStable(byParent[k], func(i, j int) bool {
			return byParent[k][i].Created.Before(byParent[k][j].Created)
		})
	}

	out := make([]threadNode, 0)
	var walk func(c domain.Comment, depth int)
	walk = func(c domain.Comment, depth int) {
		cc := c
		out = append(out, threadNode{Comment: &cc, Depth: depth})
		for i := range drafts {
			d := &drafts[i]
			if d.ParentID == c.ID {
				out = append(out, threadNode{Draft: d, Depth: depth + 1})
			}
		}
		for _, ch := range byParent[c.ID] {
			walk(ch, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	for i := range drafts {
		d := &drafts[i]
		if d.ParentID != "" || d.Path != path || d.Anchor == nil {
			continue
		}
		if sameLineAnchor(d.Anchor, ln) {
			out = append(out, threadNode{Draft: d, Depth: 0})
		}
	}
	return out
}

func paintThread(nodes []threadNode, th diff.Theme, width int) string {
	var b strings.Builder
	for _, n := range nodes {
		indent := strings.Repeat("  ", n.Depth)
		if n.Draft != nil {
			b.WriteString(diff.PaintAnnotation(indent+"you", n.Draft.Body, true, th, width) + "\n")
			continue
		}
		if n.Comment != nil {
			b.WriteString(diff.PaintAnnotation(indent+n.Comment.Author, n.Comment.Body, false, th, width) + "\n")
		}
	}
	return b.String()
}

// convEntry is one selectable row in the conversation / thread picker.
type convEntry struct {
	ID     string
	Author string
	Body   string
	Path   string
	Depth  int
	Draft  bool
	Parent string
}

func buildConversationEntries(comments []domain.Comment, drafts []domain.DraftComment) []convEntry {
	// Seed with general/file-level comments, then include their reply chains.
	seed := map[string]bool{}
	pool := map[string]domain.Comment{}
	for _, c := range comments {
		pool[c.ID] = c
		if isConversationComment(c) {
			seed[c.ID] = true
		}
	}
	include := map[string]bool{}
	for id := range seed {
		include[id] = true
	}
	changed := true
	for changed {
		changed = false
		for _, c := range comments {
			if include[c.ID] {
				continue
			}
			if c.ParentID != "" && include[c.ParentID] {
				include[c.ID] = true
				changed = true
			}
		}
	}

	byParent := map[string][]domain.Comment{}
	roots := make([]domain.Comment, 0)
	for id := range include {
		c := pool[id]
		if c.ParentID == "" || !include[c.ParentID] {
			roots = append(roots, c)
			continue
		}
		byParent[c.ParentID] = append(byParent[c.ParentID], c)
	}
	sort.SliceStable(roots, func(i, j int) bool { return roots[i].Created.Before(roots[j].Created) })
	for k := range byParent {
		sort.SliceStable(byParent[k], func(i, j int) bool {
			return byParent[k][i].Created.Before(byParent[k][j].Created)
		})
	}

	out := make([]convEntry, 0)
	var walk func(c domain.Comment, depth int)
	walk = func(c domain.Comment, depth int) {
		out = append(out, convEntry{
			ID: c.ID, Author: c.Author, Body: c.Body, Path: c.Path, Depth: depth, Parent: c.ParentID,
		})
		for i := range drafts {
			d := drafts[i]
			if d.ParentID == c.ID {
				out = append(out, convEntry{
					ID: d.ID, Author: "you", Body: d.Body, Path: d.Path, Depth: depth + 1, Draft: true, Parent: c.ID,
				})
			}
		}
		for _, ch := range byParent[c.ID] {
			walk(ch, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	for _, d := range drafts {
		if d.ParentID != "" || d.Anchor != nil {
			continue
		}
		out = append(out, convEntry{ID: d.ID, Author: "you", Body: d.Body, Depth: 0, Draft: true})
	}
	return out
}
