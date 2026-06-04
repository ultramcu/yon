package ui

import (
	"testing"

	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// fourReqDupColl builds four named requests where "a" (idx 0) carries Params, so
// the deep-copy independence case has slice data to clone and mutate.
func fourReqDupColl() model.Collection {
	return model.Collection{
		Version: 1,
		Name:    "Dup",
		Requests: []model.Request{
			{Method: model.MethodGet, Name: "a", Params: []model.Param{{Key: "k", Value: "v", Enabled: true}}},
			{Method: model.MethodGet, Name: "b"},
			{Method: model.MethodGet, Name: "c"},
			{Method: model.MethodGet, Name: "d"},
		},
	}
}

// duplicateName gives a named request a " copy" suffix and leaves an empty name
// empty so the row falls back to its derived DisplayName.
func TestDuplicateName(t *testing.T) {
	if got := duplicateName("a"); got != "a copy" {
		t.Errorf("duplicateName(%q) = %q, want %q", "a", got, "a copy")
	}
	if got := duplicateName(""); got != "" {
		t.Errorf("duplicateName(%q) = %q, want empty", "", got)
	}
}

// Duplicating inserts the copy at idx+1, grows the slice by one, names the copy
// the " copy" variant, and leaves the original untouched.
func TestDuplicateRequest_InsertsCopyAfter(t *testing.T) {
	w := newScopeWindow(t, fourReqDupColl())

	w.duplicateRequest(0) // duplicate "a"

	if got := len(w.coll.Requests); got != 5 {
		t.Fatalf("len(Requests) = %d, want 5", got)
	}
	if name := w.coll.Requests[0].Name; name != "a" {
		t.Errorf("original Requests[0].Name = %q, want %q", name, "a")
	}
	if name := w.coll.Requests[1].Name; name != "a copy" {
		t.Errorf("copy Requests[1].Name = %q, want %q", name, "a copy")
	}
	// Everything after the insertion point shifted up by one.
	if name := w.coll.Requests[2].Name; name != "b" {
		t.Errorf("Requests[2].Name = %q, want %q (shifted up)", name, "b")
	}
}

// KEY CASE: the copy's slices are cloned, not shared. Mutating the duplicate's
// Params (read back from the stored collection, not a local var) must not touch
// the original's Params — proving the backing arrays are independent.
func TestDuplicateRequest_DeepCopyIndependent(t *testing.T) {
	w := newScopeWindow(t, fourReqDupColl())

	w.duplicateRequest(0)

	// Mutate the stored copy's Params: change an element and append a new one.
	w.coll.Requests[1].Params[0].Value = "MUTATED"
	w.coll.Requests[1].Params = append(w.coll.Requests[1].Params, model.Param{Key: "x", Value: "y"})

	orig := w.coll.Requests[0]
	if len(orig.Params) != 1 {
		t.Fatalf("original Params len = %d, want 1 (append leaked through shared array)", len(orig.Params))
	}
	if orig.Params[0].Value != "v" {
		t.Errorf("original Params[0].Value = %q, want %q (mutation leaked through shared array)", orig.Params[0].Value, "v")
	}
}

// Duplicating before an open tab shifts that tab's openTabs key UP by one, keeps
// the SAME *requestTab pointer, and syncs rt.idx to the new key.
func TestDuplicateRequest_ReindexesOpenTab(t *testing.T) {
	w := newScopeWindow(t, fourReqDupColl())
	w.openRequestTab(2) // open "c"
	rtC := w.openTabs[2]
	if rtC == nil {
		t.Fatal("setup: openRequestTab(2) did not register a tab")
	}

	w.duplicateRequest(0) // insert "a copy" at idx 1; later elements shift up

	if _, stale := w.openTabs[2]; stale {
		// Key 2 should no longer hold rtC — but a newly opened copy's tab could
		// legitimately land here, so only flag it if it's still the same pointer.
		if w.openTabs[2] == rtC {
			t.Errorf("openTabs still maps key 2 to the original tab after duplicating idx 0")
		}
	}
	got, ok := w.openTabs[3]
	if !ok {
		t.Fatalf("openTabs missing re-indexed key 3; have keys %v", keysOf(w.openTabs))
	}
	if got != rtC {
		t.Errorf("openTabs[3] = %p, want the original *requestTab %p", got, rtC)
	}
	if got.idx != 3 {
		t.Errorf("re-indexed tab's rt.idx = %d, want 3", got.idx)
	}
	// "c" really lives at slice index 3 now.
	if name := w.coll.Requests[3].DisplayName(); name != "c" {
		t.Errorf("Requests[3] = %q, want %q", name, "c")
	}
}

// The selectedID-shift step keeps an existing selection (one that sits AFTER the
// insertion point) pointing at the same Request DURING the re-index, before the
// final Select moves focus to the copy. We observe the shift by duplicating the
// SELECTED row itself: duplicating idx 2 ("c") with idx 2 selected leaves the
// original "c" still selectable at its shifted index — and the final Select lands
// on the copy at idx+1. This pins both: the copy is selected, and the original
// "c" moved up (selectedID was incremented mid-flight, the copy sits just below).
func TestDuplicateRequest_SelectedShiftsUpThenSelectsCopy(t *testing.T) {
	w := newScopeWindow(t, fourReqDupColl())
	w.sidebar.Select(3) // select + open "d" (after the insertion point)
	if w.selectedID != 3 {
		t.Fatalf("setup: selectedID = %d, want 3", w.selectedID)
	}

	// Duplicate "a" (idx 0): "d" shifts from 3→4, and the open tab/selection for
	// it must follow. The final Select then focuses the new copy at idx 1.
	w.duplicateRequest(0)

	if w.selectedID != 1 {
		t.Errorf("after duplicating idx 0, selectedID = %d, want 1 (the copy)", w.selectedID)
	}
	// "d" really moved up to index 4, and its tab key followed the shift — proving
	// the selectedID++/openTabs re-index ran before the final Select.
	if name := w.coll.Requests[4].DisplayName(); name != "d" {
		t.Errorf("Requests[4] = %q, want %q (shifted up)", name, "d")
	}
	if _, ok := w.openTabs[4]; !ok {
		t.Errorf("open tab for \"d\" did not shift to key 4; have keys %v", keysOf(w.openTabs))
	}
}

// Isolates the selectedID++ branch: with the sidebar's OnSelected detached, the
// trailing Select(idx+1) can no longer rewrite selectedID, so the increment from
// duplicateRequest's own shift step (3→4 for a selection after the insertion
// point) is directly observable.
func TestDuplicateRequest_SelectedIDIncrements(t *testing.T) {
	w := newScopeWindow(t, fourReqDupColl())
	w.selectedID = 3                                  // "d", after the insertion point
	w.sidebar.OnSelected = func(widget.ListItemID) {} // neutralise the final Select

	w.duplicateRequest(0) // insert "a copy" before "d"

	if w.selectedID != 4 {
		t.Errorf("selectedID = %d, want 4 (shifted up by the insert)", w.selectedID)
	}
}

// Duplicating selects the new copy (selectedID == idx+1) and opens its tab.
func TestDuplicateRequest_SelectsAndOpensCopy(t *testing.T) {
	w := newScopeWindow(t, fourReqDupColl())

	w.duplicateRequest(0)

	if w.selectedID != 1 {
		t.Errorf("selectedID = %d, want 1 (the new copy)", w.selectedID)
	}
	if _, ok := w.openTabs[1]; !ok {
		t.Errorf("duplicating did not open the copy's tab at key 1; have keys %v", keysOf(w.openTabs))
	}
}

// Out-of-range idx is a no-op: the collection is untouched.
func TestDuplicateRequest_OutOfRangeNoop(t *testing.T) {
	w := newScopeWindow(t, fourReqDupColl())
	before := len(w.coll.Requests)

	w.duplicateRequest(-1)
	w.duplicateRequest(before) // == len, one past the end

	if got := len(w.coll.Requests); got != before {
		t.Errorf("len(Requests) = %d, want %d (no-op expected)", got, before)
	}
}
