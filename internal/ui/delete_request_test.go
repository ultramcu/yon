package ui

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// fourReqColl builds a collection of four named GET requests (a/b/c/d) for the
// delete + re-index cases below.
func fourReqColl() model.Collection {
	return model.Collection{
		Version: 1,
		Name:    "Del",
		Requests: []model.Request{
			{Method: model.MethodGet, Name: "a"},
			{Method: model.MethodGet, Name: "b"},
			{Method: model.MethodGet, Name: "c"},
			{Method: model.MethodGet, Name: "d"},
		},
	}
}

// Deleting a request before an open tab must shift that tab's openTabs key down
// by one and keep it pointing at the SAME *requestTab (the trap: a naive in-place
// rekey could clobber or lose the entry). Here we open the tab for "c" (idx 2),
// delete "a" (idx 0), and expect the same tab object to now live at key 1 — and
// for its own back-reference rt.idx to follow.
func TestDeleteRequest_ReindexesOpenTab(t *testing.T) {
	w := newScopeWindow(t, fourReqColl())
	w.openRequestTab(2) // open "c"
	rtC := w.openTabs[2]
	if rtC == nil {
		t.Fatal("setup: openRequestTab(2) did not register a tab")
	}

	w.deleteRequest(0) // remove "a"; everything after shifts down by one

	if _, stale := w.openTabs[2]; stale {
		t.Errorf("openTabs still has stale key 2 after deleting idx 0")
	}
	got, ok := w.openTabs[1]
	if !ok {
		t.Fatalf("openTabs missing re-indexed key 1; have keys %v", keysOf(w.openTabs))
	}
	if got != rtC {
		t.Errorf("openTabs[1] = %p, want the original *requestTab %p", got, rtC)
	}
	if got.idx != 1 {
		t.Errorf("re-indexed tab's rt.idx = %d, want 1", got.idx)
	}
	// The shifted request really is "c" now at slice index 1.
	if name := w.coll.Requests[1].DisplayName(); name != "c" {
		t.Errorf("Requests[1] = %q, want %q", name, "c")
	}
	if len(w.coll.Requests) != 3 {
		t.Errorf("len(Requests) = %d, want 3", len(w.coll.Requests))
	}
}

// Deleting the selected request clears the selection (selectedID → -1). Deleting
// an open tab also drops it from openTabs.
func TestDeleteRequest_SelectedClearedAndTabDropped(t *testing.T) {
	w := newScopeWindow(t, fourReqColl())
	w.sidebar.Select(1) // select + open "b" (idx 1)
	if w.selectedID != 1 {
		t.Fatalf("setup: selectedID = %d, want 1", w.selectedID)
	}
	if _, ok := w.openTabs[1]; !ok {
		t.Fatal("setup: selecting idx 1 did not open its tab")
	}

	w.deleteRequest(1)

	if w.selectedID != -1 {
		t.Errorf("after deleting the selected row, selectedID = %d, want -1", w.selectedID)
	}
	if _, ok := w.openTabs[1]; ok {
		t.Errorf("openTabs still has the deleted request's tab at key 1")
	}
	if len(w.openTabs) != 0 {
		t.Errorf("openTabs = %v, want empty after deleting the only open tab", keysOf(w.openTabs))
	}
}

// Deleting a request BEFORE the selected one shifts selectedID down by one (the
// selected row's index moves with the slice). Select "c" (idx 2), then delete
// "a" (idx 0): the selection must follow "c" to its new index 1.
func TestDeleteRequest_SelectedShiftsDown(t *testing.T) {
	w := newScopeWindow(t, fourReqColl())
	w.sidebar.Select(2) // select "c" (idx 2)
	if w.selectedID != 2 {
		t.Fatalf("setup: selectedID = %d, want 2", w.selectedID)
	}

	w.deleteRequest(0) // delete "a"; "c" shifts from idx 2 → 1

	if w.selectedID != 1 {
		t.Errorf("after deleting an earlier row, selectedID = %d, want 1", w.selectedID)
	}
	if name := w.coll.Requests[w.selectedID].DisplayName(); name != "c" {
		t.Errorf("selected request = %q, want %q (selection should still point at 'c')", name, "c")
	}
}

// Deleting when nothing is open: the request is removed and the slice shrinks; no
// panic from the empty openTabs map, and selection stays untouched (-1).
func TestDeleteRequest_NothingOpen(t *testing.T) {
	w := newScopeWindow(t, fourReqColl())
	if len(w.openTabs) != 0 || w.selectedID != -1 {
		t.Fatalf("setup expected no open tabs and no selection")
	}

	w.deleteRequest(1) // remove "b"

	if len(w.coll.Requests) != 3 {
		t.Errorf("len(Requests) = %d, want 3", len(w.coll.Requests))
	}
	if w.coll.Requests[1].DisplayName() != "c" {
		t.Errorf("Requests[1] = %q, want %q", w.coll.Requests[1].DisplayName(), "c")
	}
	if w.selectedID != -1 {
		t.Errorf("selectedID = %d, want -1 (unchanged)", w.selectedID)
	}
	if !w.dirty {
		t.Errorf("deleteRequest did not mark the window dirty")
	}
}

// Deleting the last (highest-index) request removes it cleanly: the slice shrinks
// and no later elements need shifting.
func TestDeleteRequest_LastRequest(t *testing.T) {
	w := newScopeWindow(t, fourReqColl())
	last := len(w.coll.Requests) - 1

	w.deleteRequest(last) // remove "d"

	if len(w.coll.Requests) != 3 {
		t.Fatalf("len(Requests) = %d, want 3", len(w.coll.Requests))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got := w.coll.Requests[i].DisplayName(); got != want {
			t.Errorf("Requests[%d] = %q, want %q", i, got, want)
		}
	}
}

// keysOf returns the keys of an openTabs map for diagnostic messages.
func keysOf(m map[int]*requestTab) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
