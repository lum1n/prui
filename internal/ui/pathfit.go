package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

const pathEllipsis = "…"

// fitPathKeepBase shortens a path to at most max display cells, preferring to
// keep the basename readable (left-truncate with an ellipsis).
func fitPathKeepBase(path string, max int) string {
	if max <= 0 {
		return ""
	}
	if runewidth.StringWidth(path) <= max {
		return path
	}
	base := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 && i+1 < len(path) {
		base = path[i+1:]
	}
	ellW := runewidth.StringWidth(pathEllipsis)
	if runewidth.StringWidth(base) >= max {
		if max <= ellW {
			return runewidth.Truncate(pathEllipsis, max, "")
		}
		return pathEllipsis + truncateLeft(base, max-ellW)
	}
	keep := max - ellW
	if keep <= 0 {
		return pathEllipsis
	}
	suffix := truncateLeft(path, keep)
	if idx := strings.IndexByte(suffix, '/'); idx >= 0 {
		return pathEllipsis + suffix[idx:]
	}
	withSlash := "/" + base
	if ellW+runewidth.StringWidth(withSlash) <= max {
		return pathEllipsis + withSlash
	}
	return pathEllipsis + base
}

// truncateLeft returns the rightmost portion of s with display width <= max.
func truncateLeft(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= max {
		return s
	}
	rs := []rune(s)
	w := 0
	start := len(rs)
	for i := len(rs) - 1; i >= 0; i-- {
		rw := runewidth.RuneWidth(rs[i])
		if w+rw > max {
			break
		}
		w += rw
		start = i
	}
	return string(rs[start:])
}
