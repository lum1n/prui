package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPRListTabCycle(t *testing.T) {
	if tabOpen.next() != tabDrafts || tabDrafts.next() != tabMerged || tabMerged.next() != tabOpen {
		t.Fatal("next cycle broken")
	}
	if tabOpen.prev() != tabMerged || tabMerged.prev() != tabDrafts {
		t.Fatal("prev cycle broken")
	}
}

func TestPRListTabListState(t *testing.T) {
	if tabOpen.listState() != "open" || tabDrafts.listState() != "draft" || tabMerged.listState() != "merged" {
		t.Fatal("listState mismatch")
	}
}

func TestRenderPRTabsHeight(t *testing.T) {
	for _, tab := range []prListTab{tabOpen, tabDrafts, tabMerged} {
		view := renderPRTabs(tab, 80)
		if h := lipgloss.Height(view); h != prTabBarHeight {
			t.Fatalf("tab %v height=%d want %d\n%s", tab, h, prTabBarHeight, view)
		}
		if !strings.Contains(view, tab.label()) {
			t.Fatalf("missing label %q in:\n%s", tab.label(), view)
		}
	}
}
