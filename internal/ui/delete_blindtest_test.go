package ui

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// fourReq builds a collection with four distinguishable named requests A,B,C,D.
func fourReq() model.Collection {
	coll := model.NewCollection("T")
	coll.Requests = []model.Request{
		{Name: "A", Method: model.MethodGet},
		{Name: "B", Method: model.MethodPost},
		{Name: "C", Method: model.MethodPut},
		{Name: "D", Method: model.MethodDelete},
	}
	return coll
}

func names(reqs []model.Request) []string {
	out := make([]string, len(reqs))
	for i, r := range reqs {
		out[i] = r.Name
	}
	return out
}

// SPEC 1: slice removal preserves the other requests in relative order.
func TestBlindDelete_SliceRemovalKeepsOrder(t *testing.T) {
	w := newScopeWindow(t, fourReq())

	w.deleteRequest(1) // remove B

	got := names(w.coll.Requests)
	want := []string{"A", "C", "D"}
	if len(got) != len(want) {
		t.Fatalf("len Requests = %d %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Requests order = %v, want %v", got, want)
		}
	}
	for _, r := range w.coll.Requests {
		if r.Name == "B" {
			t.Fatalf("deleted request B still present: %v", got)
		}
	}
}

// SPEC 2: open-tab re-index. Open tabs for A,B,C,D; delete A (idx 0). A tab that
// was at key k>0 must be reachable at k-1, be the SAME pointer, and rt.idx == new key.
func TestBlindDelete_OpenTabReindex(t *testing.T) {
	w := newScopeWindow(t, fourReq())

	for i := 0; i < 4; i++ {
		w.openRequestTab(i)
	}
	// Capture pointers before deletion, keyed by original index.
	before := map[int]*requestTab{}
	for k, rt := range w.openTabs {
		before[k] = rt
	}
	if len(before) != 4 {
		t.Fatalf("setup: expected 4 open tabs, got %d", len(before))
	}

	w.deleteRequest(0) // delete A

	// C was at index 2 -> should now be reachable at key 1.
	for _, origK := range []int{1, 2, 3} {
		newK := origK - 1
		rt, ok := w.openTabs[newK]
		if !ok {
			t.Fatalf("tab originally at %d not reachable at key %d; openTabs keys=%v",
				origK, newK, blindKeysOf(w.openTabs))
		}
		if rt != before[origK] {
			t.Fatalf("tab at new key %d is a different *requestTab pointer than the original at %d (recreated)", newK, origK)
		}
		if rt.idx != newK {
			t.Fatalf("rt.idx back-reference = %d at new key %d, want %d", rt.idx, newK, newK)
		}
	}
	if len(w.openTabs) != 3 {
		t.Fatalf("after deleting one open tab, openTabs len = %d, want 3", len(w.openTabs))
	}
}

func blindKeysOf(m map[int]*requestTab) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// SPEC 3: deleting a request that had an open tab removes its entry and drops the count.
func TestBlindDelete_DeletingOpenRequestRemovesEntry(t *testing.T) {
	w := newScopeWindow(t, fourReq())

	w.openRequestTab(0)
	w.openRequestTab(2)
	if len(w.openTabs) != 2 {
		t.Fatalf("setup: openTabs len = %d, want 2", len(w.openTabs))
	}
	deletedTab := w.openTabs[2]

	w.deleteRequest(2) // C had an open tab

	if len(w.openTabs) != 1 {
		t.Fatalf("openTabs len = %d after deleting open request, want 1; keys=%v",
			len(w.openTabs), blindKeysOf(w.openTabs))
	}
	for k, rt := range w.openTabs {
		if rt == deletedTab {
			t.Fatalf("deleted request's tab pointer still present at key %d", k)
		}
	}
}

// SPEC 4a: deleting the currently-selected request sets selectedID to -1.
func TestBlindDelete_DeleteSelectedClearsSelection(t *testing.T) {
	w := newScopeWindow(t, fourReq())
	w.selectedID = 2 // C selected

	w.deleteRequest(2)

	if w.selectedID != -1 {
		t.Fatalf("selectedID = %d after deleting selected, want -1", w.selectedID)
	}
}

// SPEC 4b: deleting a request BEFORE the selected one decrements selectedID.
func TestBlindDelete_DeleteBeforeSelectedDecrements(t *testing.T) {
	w := newScopeWindow(t, fourReq())
	w.selectedID = 3 // D selected

	w.deleteRequest(1) // delete B (before D)

	if w.selectedID != 2 {
		t.Fatalf("selectedID = %d after deleting earlier request, want 2", w.selectedID)
	}
	// Sanity: the selected element should still be D.
	if w.coll.Requests[w.selectedID].Name != "D" {
		t.Fatalf("selectedID %d now points at %q, want D", w.selectedID, w.coll.Requests[w.selectedID].Name)
	}
}

// SPEC 4c: deleting a request AFTER the selected one leaves selectedID unchanged.
func TestBlindDelete_DeleteAfterSelectedUnchanged(t *testing.T) {
	w := newScopeWindow(t, fourReq())
	w.selectedID = 1 // B selected

	w.deleteRequest(3) // delete D (after B)

	if w.selectedID != 1 {
		t.Fatalf("selectedID = %d after deleting later request, want 1 (unchanged)", w.selectedID)
	}
}

// SPEC 5a: deleting when nothing is open does not panic and removes the row.
func TestBlindDelete_NothingOpenNoPanic(t *testing.T) {
	w := newScopeWindow(t, fourReq())
	if len(w.openTabs) != 0 {
		t.Fatalf("setup: expected no open tabs, got %d", len(w.openTabs))
	}

	w.deleteRequest(1)

	if got := names(w.coll.Requests); len(got) != 3 {
		t.Fatalf("Requests = %v, want 3 remaining", got)
	}
}

// SPEC 5b: deleting the only request leaves Requests empty without panic.
func TestBlindDelete_LastOnlyRequest(t *testing.T) {
	coll := model.NewCollection("T")
	coll.Requests = []model.Request{{Name: "Only", Method: model.MethodGet}}
	w := newScopeWindow(t, coll)
	w.openRequestTab(0)

	w.deleteRequest(0)

	if len(w.coll.Requests) != 0 {
		t.Fatalf("Requests len = %d, want 0", len(w.coll.Requests))
	}
	if len(w.openTabs) != 0 {
		t.Fatalf("openTabs len = %d, want 0", len(w.openTabs))
	}
}

// SPEC 5c: out-of-range indices are safe no-ops (-1 and len).
func TestBlindDelete_OutOfRangeNoOp(t *testing.T) {
	for _, idx := range []int{-1, 4} {
		w := newScopeWindow(t, fourReq())
		w.openRequestTab(0)
		w.dirty = false

		w.deleteRequest(idx)

		if got := names(w.coll.Requests); len(got) != 4 {
			t.Fatalf("idx=%d: Requests changed to %v, want 4 unchanged", idx, got)
		}
		if w.dirty {
			t.Fatalf("idx=%d: dirty set true by out-of-range delete (should be no-op)", idx)
		}
		if len(w.openTabs) != 1 {
			t.Fatalf("idx=%d: openTabs len = %d, want 1 unchanged", idx, len(w.openTabs))
		}
	}
}

// SPEC 6: dirty becomes true after a valid delete.
func TestBlindDelete_SetsDirty(t *testing.T) {
	w := newScopeWindow(t, fourReq())
	w.dirty = false

	w.deleteRequest(0)

	if !w.dirty {
		t.Fatalf("dirty = false after delete, want true")
	}
}
