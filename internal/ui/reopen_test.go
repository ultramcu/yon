package ui

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// TestCloseTab_AllowsReopenViaSidebar guards the fix where a closed tab left its
// sidebar row selected, so re-clicking it (a no-op Select) never reopened it.
func TestCloseTab_AllowsReopenViaSidebar(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{{Name: "R", Method: model.MethodGet, URL: "http://x/y"}}
	w := newTestWindow(coll)

	w.sidebar.Select(0)
	if len(w.openTabs) != 1 {
		t.Fatalf("select should open a tab, got %d", len(w.openTabs))
	}
	w.closeTab(w.openTabs[0].tab)
	if len(w.openTabs) != 0 {
		t.Fatal("close should remove the tab")
	}
	if w.selectedID != -1 {
		t.Fatalf("selectedID should reset on close, got %d", w.selectedID)
	}
	w.sidebar.Select(0) // same row again → must reopen
	if len(w.openTabs) != 1 {
		t.Fatalf("re-selecting the row should reopen the tab, got %d", len(w.openTabs))
	}
}
