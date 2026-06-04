package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// --- BLIND test suite for (*Window).duplicateRequest, authored independently
// from the Dev's tests. Helper names are prefixed bt_ to avoid collisions.

// bt_newWindow builds a Window backed by coll (mirrors newScopeWindow but with a
// unique name so there's no collision with the Dev's helpers).
func bt_newWindow(t *testing.T, coll model.Collection) *Window {
	t.Helper()
	a := test.NewApp()
	w := newWindow(New(a), coll, "")
	t.Cleanup(w.win.Close)
	return w
}

// bt_coll builds a collection with several named requests; the request at
// index 1 ("beta") carries a Param and a Header so deep-copy can be exercised.
func bt_coll() model.Collection {
	c := model.NewCollection("BT")
	c.Requests = []model.Request{
		{Name: "alpha", Method: model.MethodGet, URL: "http://h/a"},
		{
			Name:    "beta",
			Method:  model.MethodPost,
			URL:     "http://h/b",
			Params:  []model.Param{{Key: "p", Value: "1", Enabled: true}},
			Headers: []model.Param{{Key: "H", Value: "h", Enabled: true}},
		},
		{Name: "gamma", Method: model.MethodGet, URL: "http://h/c"},
		{Name: "delta", Method: model.MethodGet, URL: "http://h/d"},
	}
	return c
}

// TestBT_Insertion verifies SPEC 1: length grows by one, copy lands at idx+1,
// original is unchanged at idx, others keep order.
func TestBT_Insertion(t *testing.T) {
	w := bt_newWindow(t, bt_coll())
	const idx = 1
	before := append([]model.Request(nil), w.coll.Requests...)

	w.duplicateRequest(idx)

	if got, want := len(w.coll.Requests), len(before)+1; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	// Original still at idx, fields intact (Name/Method/URL).
	orig := w.coll.Requests[idx]
	if orig.Name != "beta" || orig.Method != model.MethodPost || orig.URL != "http://h/b" {
		t.Fatalf("original at %d corrupted: %+v", idx, orig)
	}
	// Copy at idx+1 mirrors the source's Method/URL.
	cp := w.coll.Requests[idx+1]
	if cp.Method != model.MethodPost || cp.URL != "http://h/b" {
		t.Fatalf("copy at %d not a content copy: %+v", idx+1, cp)
	}
	// Elements before idx and after the inserted copy keep order.
	if w.coll.Requests[0].Name != "alpha" {
		t.Fatalf("index 0 = %q, want alpha", w.coll.Requests[0].Name)
	}
	if w.coll.Requests[3].Name != "gamma" || w.coll.Requests[4].Name != "delta" {
		t.Fatalf("tail order broke: %q,%q want gamma,delta",
			w.coll.Requests[3].Name, w.coll.Requests[4].Name)
	}
}

// TestBT_CopyName verifies SPEC 2: non-empty name -> name+" copy"; empty stays
// empty.
func TestBT_CopyName(t *testing.T) {
	w := bt_newWindow(t, bt_coll())
	w.duplicateRequest(1) // source "beta"
	if got, want := w.coll.Requests[2].Name, "beta copy"; got != want {
		t.Fatalf("copy name = %q, want %q", got, want)
	}

	// Empty-name source.
	c := model.NewCollection("E")
	c.Requests = []model.Request{{Name: "", Method: model.MethodGet, URL: "http://h/x"}}
	w2 := bt_newWindow(t, c)
	w2.duplicateRequest(0)
	if got := w2.coll.Requests[1].Name; got != "" {
		t.Fatalf("empty-source copy name = %q, want empty", got)
	}
}

// TestBT_DeepCopy verifies SPEC 3 (the key correctness test): mutating the
// STORED copy's Params/Headers must not change the STORED original.
func TestBT_DeepCopy(t *testing.T) {
	w := bt_newWindow(t, bt_coll())
	const idx = 1
	w.duplicateRequest(idx)

	// Sanity: both have one param/header with the seeded values.
	if len(w.coll.Requests[idx].Params) != 1 || len(w.coll.Requests[idx+1].Params) != 1 {
		t.Fatalf("param counts: orig=%d copy=%d",
			len(w.coll.Requests[idx].Params), len(w.coll.Requests[idx+1].Params))
	}

	// Mutate the copy's existing param element in place.
	w.coll.Requests[idx+1].Params[0].Value = "X"
	if got := w.coll.Requests[idx].Params[0].Value; got != "1" {
		t.Fatalf("SHALLOW PARAM ALIASING: original Params[0].Value = %q, want %q (copy mutation leaked)", got, "1")
	}

	w.coll.Requests[idx+1].Headers[0].Value = "Y"
	if got := w.coll.Requests[idx].Headers[0].Value; got != "h" {
		t.Fatalf("SHALLOW HEADER ALIASING: original Headers[0].Value = %q, want %q (copy mutation leaked)", got, "h")
	}

	// Append to the copy's slices: a shared backing array could overwrite the
	// original's element or change its length view.
	w.coll.Requests[idx+1].Params = append(w.coll.Requests[idx+1].Params, model.Param{Key: "extra", Value: "z", Enabled: true})
	w.coll.Requests[idx+1].Headers = append(w.coll.Requests[idx+1].Headers, model.Param{Key: "X2", Value: "z", Enabled: true})
	if got := len(w.coll.Requests[idx].Params); got != 1 {
		t.Fatalf("original Params len = %d after copy append, want 1 (shared backing array)", got)
	}
	if got := len(w.coll.Requests[idx].Headers); got != 1 {
		t.Fatalf("original Headers len = %d after copy append, want 1 (shared backing array)", got)
	}
	// And the original's values are still pristine.
	if w.coll.Requests[idx].Params[0].Value != "1" || w.coll.Requests[idx].Headers[0].Value != "h" {
		t.Fatalf("original values corrupted after copy append: %+v / %+v",
			w.coll.Requests[idx].Params, w.coll.Requests[idx].Headers)
	}
}

// TestBT_OpenTabReindex verifies SPEC 4: tabs at keys > idx move to key+1, keep
// the same *requestTab pointer, and rt.idx tracks the new key; keys <= idx
// unchanged.
func TestBT_OpenTabReindex(t *testing.T) {
	w := bt_newWindow(t, bt_coll())
	const idx = 1
	// Open tabs at 0 (< idx), 1 (== idx), 2 and 3 (> idx).
	w.openRequestTab(0)
	w.openRequestTab(1)
	w.openRequestTab(2)
	w.openRequestTab(3)

	// Capture pointers before duplicating.
	p0 := w.openTabs[0]
	p1 := w.openTabs[1]
	p2 := w.openTabs[2]
	p3 := w.openTabs[3]
	if p0 == nil || p1 == nil || p2 == nil || p3 == nil {
		t.Fatalf("expected 4 open tabs, got map %v", bt_keys(w.openTabs))
	}

	w.duplicateRequest(idx)

	// keys <= idx unchanged, same pointers, rt.idx matches key.
	if w.openTabs[0] != p0 {
		t.Fatalf("tab key 0 pointer changed/lost")
	}
	if w.openTabs[0].idx != 0 {
		t.Fatalf("tab key 0 rt.idx = %d, want 0", w.openTabs[0].idx)
	}
	if w.openTabs[1] != p1 {
		t.Fatalf("tab key 1 pointer changed/lost")
	}
	if w.openTabs[1].idx != 1 {
		t.Fatalf("tab key 1 rt.idx = %d, want 1", w.openTabs[1].idx)
	}

	// key 2 -> 3, key 3 -> 4, same pointers, rt.idx synced.
	if w.openTabs[3] != p2 {
		t.Fatalf("tab formerly at key 2 not reachable at key 3 (pointer mismatch)")
	}
	if w.openTabs[3].idx != 3 {
		t.Fatalf("re-keyed tab (was 2, now 3) rt.idx = %d, want 3", w.openTabs[3].idx)
	}
	if w.openTabs[4] != p3 {
		t.Fatalf("tab formerly at key 3 not reachable at key 4 (pointer mismatch)")
	}
	if w.openTabs[4].idx != 4 {
		t.Fatalf("re-keyed tab (was 3, now 4) rt.idx = %d, want 4", w.openTabs[4].idx)
	}

	// No tab should now be parked at the old key 2 except the newly opened copy
	// (selection opens idx+1 == 2). We assert that key 2 exists and is the copy's
	// tab, distinct from the relocated p2.
	if w.openTabs[2] == nil {
		t.Fatalf("expected a tab at new copy key 2")
	}
	if w.openTabs[2] == p2 {
		t.Fatalf("key 2 still holds the relocated original tab; re-index failed")
	}
}

// TestBT_Selection verifies SPEC 5: the copy at idx+1 is selected and its tab is
// open.
func TestBT_Selection(t *testing.T) {
	w := bt_newWindow(t, bt_coll())
	const idx = 1
	w.duplicateRequest(idx)

	if w.selectedID != idx+1 {
		t.Fatalf("selectedID = %d, want %d", w.selectedID, idx+1)
	}
	if w.openTabs[idx+1] == nil {
		t.Fatalf("expected open tab at copy index %d, keys=%v", idx+1, bt_keys(w.openTabs))
	}
}

// TestBT_SelectionFollowsExisting verifies the SPEC-4 selection-bump corollary:
// a selection that was > idx moves up by one with its request.
func TestBT_SelectionFollowsExisting(t *testing.T) {
	w := bt_newWindow(t, bt_coll())
	w.openRequestTab(3)
	w.sidebar.Select(3) // select "delta" at index 3 (> idx)
	if w.selectedID != 3 {
		t.Fatalf("precondition: selectedID = %d, want 3", w.selectedID)
	}
	w.duplicateRequest(1)
	// After duplicating at 1, delta is at index 4; but duplicateRequest also
	// selects the new copy (idx+1 == 2). The spec for selection is "new copy
	// selected", so the final selection is 2. We assert the copy is selected.
	if w.selectedID != 2 {
		t.Fatalf("after duplicate, selectedID = %d, want 2 (new copy)", w.selectedID)
	}
	// And delta really did move to index 4.
	if w.coll.Requests[4].Name != "delta" {
		t.Fatalf("delta at index 4 expected, got %q", w.coll.Requests[4].Name)
	}
}

// TestBT_OutOfRange verifies SPEC 6: -1 and len(Requests) are safe no-ops.
func TestBT_OutOfRange(t *testing.T) {
	for _, bad := range []string{"neg", "len"} {
		w := bt_newWindow(t, bt_coll())
		n := len(w.coll.Requests)
		var idx int
		if bad == "neg" {
			idx = -1
		} else {
			idx = n
		}
		w.duplicateRequest(idx) // must not panic
		if got := len(w.coll.Requests); got != n {
			t.Fatalf("[%s] len changed to %d, want %d", bad, got, n)
		}
		if w.dirty {
			t.Fatalf("[%s] dirty set by no-op duplicate", bad)
		}
	}
}

// TestBT_DirtyAfterDuplicate verifies SPEC 7: a successful duplicate sets dirty.
func TestBT_DirtyAfterDuplicate(t *testing.T) {
	w := bt_newWindow(t, bt_coll())
	if w.dirty {
		t.Fatalf("precondition: window already dirty")
	}
	w.duplicateRequest(1)
	if !w.dirty {
		t.Fatalf("dirty = false after successful duplicate, want true")
	}
}

func bt_keys(m map[int]*requestTab) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
