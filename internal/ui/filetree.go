package ui

import (
	"path"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"

	"github.com/lum1n/prui/internal/domain"
)

// fileItem is one row in the files pane (a file, or a directory in tree mode).
type fileItem struct {
	file     domain.FileChange
	drafts   int
	comments int
	maxTitle int // list content width for path fitting; 0 = no fit

	isDir     bool
	dirPath   string // directory path without trailing slash
	depth     int
	name      string // basename (dirs keep trailing "/")
	collapsed bool   // directory row is collapsed
	last      bool   // last sibling — use └── vs ├──
	lasts     []bool // last-sibling flags from root to this node
}

func (i fileItem) FilterValue() string {
	if i.isDir {
		return i.dirPath
	}
	return i.file.Path
}

func (i fileItem) isFile() bool { return !i.isDir && i.file.Path != "" }

// buildFileListItems builds flat or tree rows for the files pane.
func buildFileListItems(
	files []domain.FileChange,
	tree bool,
	collapsed map[string]bool,
	draftCounts, commentCounts map[string]int,
	maxTitle int,
) []list.Item {
	if !tree {
		items := make([]list.Item, 0, len(files))
		for _, f := range files {
			items = append(items, fileItem{
				file:     f,
				drafts:   draftCounts[f.Path],
				comments: commentCounts[f.Path],
				maxTitle: maxTitle,
				name:     path.Base(f.Path),
			})
		}
		return items
	}
	return buildFileTreeItems(files, collapsed, draftCounts, commentCounts, maxTitle)
}

func buildFileTreeItems(
	files []domain.FileChange,
	collapsed map[string]bool,
	draftCounts, commentCounts map[string]int,
	maxTitle int,
) []list.Item {
	sorted := append([]domain.FileChange(nil), files...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})

	type node struct {
		name     string
		dirPath  string // non-empty for directories
		file     *domain.FileChange
		children []*node
	}
	root := &node{name: ""}

	ensureDir := func(parts []string) *node {
		cur := root
		for i, p := range parts {
			found := (*node)(nil)
			for _, ch := range cur.children {
				if ch.file == nil && ch.name == p {
					found = ch
					break
				}
			}
			if found == nil {
				found = &node{
					name:    p,
					dirPath: strings.Join(parts[:i+1], "/"),
				}
				cur.children = append(cur.children, found)
			}
			cur = found
		}
		return cur
	}

	for i := range sorted {
		f := &sorted[i]
		parts := strings.Split(f.Path, "/")
		if len(parts) == 1 {
			root.children = append(root.children, &node{name: parts[0], file: f})
			continue
		}
		parent := ensureDir(parts[:len(parts)-1])
		parent.children = append(parent.children, &node{name: parts[len(parts)-1], file: f})
	}

	var sortNodes func(n *node)
	sortNodes = func(n *node) {
		sort.SliceStable(n.children, func(i, j int) bool {
			a, b := n.children[i], n.children[j]
			aDir, bDir := a.file == nil, b.file == nil
			if aDir != bDir {
				return aDir // dirs first
			}
			return a.name < b.name
		})
		for _, ch := range n.children {
			if ch.file == nil {
				sortNodes(ch)
			}
		}
	}
	sortNodes(root)

	var items []list.Item
	var walk func(n *node, depth int, lasts []bool)
	walk = func(n *node, depth int, lasts []bool) {
		for i, ch := range n.children {
			last := i == len(n.children)-1
			childLasts := append(append([]bool{}, lasts...), last)
			if ch.file == nil {
				isCollapsed := collapsed[ch.dirPath]
				items = append(items, fileItem{
					isDir:     true,
					dirPath:   ch.dirPath,
					depth:     depth,
					name:      ch.name + "/",
					collapsed: isCollapsed,
					last:      last,
					lasts:     childLasts,
					maxTitle:  maxTitle,
				})
				if !isCollapsed {
					walk(ch, depth+1, childLasts)
				}
				continue
			}
			f := *ch.file
			items = append(items, fileItem{
				file:     f,
				drafts:   draftCounts[f.Path],
				comments: commentCounts[f.Path],
				maxTitle: maxTitle,
				depth:    depth,
				name:     ch.name,
				last:     last,
				lasts:    childLasts,
			})
		}
	}
	walk(root, 0, nil)
	return items
}

// treePrefix returns the leading branch/indent string for a tree row.
// lasts[i] is whether the node at depth i was the last sibling.
func treePrefix(lasts []bool, isDir, collapsed bool) string {
	if len(lasts) == 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(lasts)-1; i++ {
		if lasts[i] {
			b.WriteString("  ")
		} else {
			b.WriteString("│ ")
		}
	}
	last := lasts[len(lasts)-1]
	branch := "├─"
	if last {
		branch = "└─"
	}
	b.WriteString(branch)
	if isDir {
		if collapsed {
			b.WriteString("▸ ")
		} else {
			b.WriteString("▾ ")
		}
		return b.String()
	}
	b.WriteString(" ")
	return b.String()
}
