package ui

import "testing"

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
