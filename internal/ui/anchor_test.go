package ui

import (
	"testing"

	"github.com/vegard/prui/internal/domain"
)

func TestAnchorHint(t *testing.T) {
	if got := anchorHint(nil); got != "" {
		t.Fatalf("%q", got)
	}
	if got := anchorHint(&domain.Anchor{Line: 12}); got != " line 12" {
		t.Fatalf("%q", got)
	}
	if got := anchorHint(&domain.Anchor{Line: 10, EndLine: 18}); got != " lines 10–18" {
		t.Fatalf("%q", got)
	}
}
