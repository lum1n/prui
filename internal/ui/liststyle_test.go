package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lum1n/prui/internal/domain"
	"github.com/muesli/termenv"
)

func TestFileStatusGlyph(t *testing.T) {
	cases := []struct {
		st   domain.FileStatus
		want string
	}{
		{domain.FileAdded, "A"},
		{domain.FileModified, "M"},
		{domain.FileRemoved, "D"},
		{domain.FileRenamed, "R"},
	}
	for _, tc := range cases {
		got, _ := fileStatusGlyph(tc.st)
		if got != tc.want {
			t.Fatalf("fileStatusGlyph(%q)=%q want %q", tc.st, got, tc.want)
		}
	}
}

func TestFormatReviewBadgeColored(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	rs := domain.ReviewStatus{
		Approvers:        []string{"Alice (a)"},
		ChangeRequesters: []string{"Bob (b)"},
		ViewerDecision:   domain.DecisionApproved,
	}
	out := formatReviewBadge(rs)
	plain := ansi.Strip(out)
	for _, part := range []string{"✓1", "✗1", "you✓"} {
		if !strings.Contains(plain, part) {
			t.Fatalf("missing %q in %q (ansi %q)", part, plain, out)
		}
	}
	if plain == out {
		t.Fatalf("expected ANSI color in badge, got plain %q", out)
	}
}
