package diff

import (
	"fmt"
	"strings"

	"github.com/lum1n/prui/internal/domain"
)

// ExpandGap replaces the hunk header at headerIdx with context lines from
// fileContent (1-indexed new-side lines [GapFrom, GapTo)).
// Returns the new cursor index (first inserted line) and the header that was replaced.
func ExpandGap(fd *domain.FileDiff, headerIdx int, fileContent string) (cursor int, header domain.DiffLine, err error) {
	if fd == nil || headerIdx < 0 || headerIdx >= len(fd.Lines) {
		return 0, domain.DiffLine{}, fmt.Errorf("invalid gap index")
	}
	header = fd.Lines[headerIdx]
	if !IsHunkHeader(header) || header.GapBefore <= 0 {
		return 0, domain.DiffLine{}, fmt.Errorf("not an expandable gap")
	}
	lines := strings.Split(strings.ReplaceAll(fileContent, "\r\n", "\n"), "\n")
	// Drop trailing empty line from final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	from, to := header.GapFrom, header.GapTo
	if from < 1 || to <= from {
		return 0, domain.DiffLine{}, fmt.Errorf("invalid gap range %d–%d", from, to)
	}
	if to-1 > len(lines) {
		return 0, domain.DiffLine{}, fmt.Errorf("file has %d lines; need through %d", len(lines), to-1)
	}
	inserted := make([]domain.DiffLine, 0, to-from)
	for n := from; n < to; n++ {
		text := lines[n-1]
		inserted = append(inserted, domain.DiffLine{
			Kind:      domain.LineContext,
			OldNumber: 0, // unknown without base; display new side only
			NewNumber: n,
			Text:      text,
			HunkIndex: header.HunkIndex,
			Expanded:  true,
			GapBefore: header.GapBefore,
			GapFrom:   header.GapFrom,
			GapTo:     header.GapTo,
			Anchor: domain.Anchor{
				Path:     fd.Path,
				Side:     domain.SideRight,
				Line:     n,
				LineType: domain.LineContext,
			},
		})
	}
	out := make([]domain.DiffLine, 0, len(fd.Lines)-1+len(inserted))
	out = append(out, fd.Lines[:headerIdx]...)
	out = append(out, inserted...)
	out = append(out, fd.Lines[headerIdx+1:]...)
	fd.Lines = out
	return headerIdx, header, nil
}

// CollapseGap restores a previously expanded gap whose first expanded line is at startIdx.
func CollapseGap(fd *domain.FileDiff, startIdx int, header domain.DiffLine) (cursor int, err error) {
	if fd == nil || startIdx < 0 || startIdx >= len(fd.Lines) {
		return 0, fmt.Errorf("invalid collapse index")
	}
	if !fd.Lines[startIdx].Expanded {
		return 0, fmt.Errorf("not an expanded gap")
	}
	end := startIdx
	for end < len(fd.Lines) && fd.Lines[end].Expanded {
		end++
	}
	out := make([]domain.DiffLine, 0, len(fd.Lines)-(end-startIdx)+1)
	out = append(out, fd.Lines[:startIdx]...)
	out = append(out, header)
	out = append(out, fd.Lines[end:]...)
	fd.Lines = out
	return startIdx, nil
}

// ExpandableGap reports whether the line is a hunk header with omitted context.
func ExpandableGap(line domain.DiffLine) bool {
	return IsHunkHeader(line) && line.GapBefore > 0
}
