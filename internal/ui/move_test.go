package ui

import (
	"sort"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// movedColl builds a collection with two folders (F1, F2) and a mix of grouped
// and top-level requests, in a known flat order, for the move tests:
//
//	flat 0: a  (F1)
//	flat 1: b  (F1)
//	flat 2: c  (F2)
//	flat 3: d  ("" top-level)
//	flat 4: e  ("" top-level)
func movedColl() model.Collection {
	return model.Collection{
		Version: 1,
		Name:    "Move",
		Folders: []model.Folder{
			{ID: "F1", Name: "One"},
			{ID: "F2", Name: "Two"},
		},
		Requests: []model.Request{
			{Method: model.MethodGet, Name: "a", FolderID: "F1"},
			{Method: model.MethodGet, Name: "b", FolderID: "F1"},
			{Method: model.MethodGet, Name: "c", FolderID: "F2"},
			{Method: model.MethodGet, Name: "d"},
			{Method: model.MethodGet, Name: "e"},
		},
	}
}

// flatNames returns the flat request names in order, for asserting slice
// ordering. (The package already has names([]model.Request); this is the
// Window-scoped convenience used across the move tests.)
func flatNames(w *Window) []string {
	out := make([]string, len(w.coll.Requests))
	for i := range w.coll.Requests {
		out[i] = w.coll.Requests[i].Name
	}
	return out
}

// nameMultiset returns the sorted request names, to assert nothing was lost or
// duplicated across a move (it's a permutation of the same multiset).
func nameMultiset(w *Window) []string {
	out := flatNames(w)
	sort.Strings(out)
	return out
}

// folderOf returns the FolderID of the request currently named n (or "?!" if no
// such request exists), so tests can assert reparenting by name not index.
func folderOf(w *Window, n string) string {
	for i := range w.coll.Requests {
		if w.coll.Requests[i].Name == n {
			return w.coll.Requests[i].FolderID
		}
	}
	return "?!"
}

// idxOf returns the current flat index of the request named n, or -1.
func idxOf(w *Window, n string) int {
	for i := range w.coll.Requests {
		if w.coll.Requests[i].Name == n {
			return i
		}
	}
	return -1
}

// ---- moveSlice (the pure permutation engine) --------------------------------

func TestMoveSlice_BasicReorderAndPerm(t *testing.T) {
	cases := []struct {
		name     string
		in       []string
		src, dst int
		want     []string
		wantPerm []int // nil means expect a no-op (nil perm)
	}{
		{"move first before last-anchor (idx1)", []string{"a", "b", "c", "d"}, 0, 2, []string{"b", "a", "c", "d"}, []int{1, 0, 2, 3}},
		{"move last to front", []string{"a", "b", "c", "d"}, 3, 0, []string{"d", "a", "b", "c"}, []int{1, 2, 3, 0}},
		{"move to end (dst==len)", []string{"a", "b", "c", "d"}, 1, 4, []string{"a", "c", "d", "b"}, []int{0, 3, 0, 1}},
		{"no-op: insert at own spot", []string{"a", "b", "c"}, 1, 1, []string{"a", "b", "c"}, nil},
		{"no-op: insert before next == own", []string{"a", "b", "c"}, 1, 2, []string{"a", "b", "c"}, nil},
		{"src out of range", []string{"a"}, 5, 0, []string{"a"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := append([]string(nil), tc.in...)
			perm := moveSlice(s, tc.src, tc.dst)
			// wantPerm[i] for the "move to end" case is computed below; recompute the
			// expected slice ordering by checking the slice itself.
			if tc.wantPerm == nil && perm != nil {
				t.Fatalf("expected nil perm (no-op), got %v with slice %v", perm, s)
			}
			if tc.wantPerm != nil && perm == nil {
				t.Fatalf("expected a perm, got nil (no-op); slice %v", s)
			}
			gotJoin := join(s)
			wantJoin := join(tc.want)
			if gotJoin != wantJoin {
				t.Fatalf("slice after move = %v, want %v", s, tc.want)
			}
			// Verify perm maps old positions to the new positions consistently with
			// the resulting slice: for every old index, in[old] must equal s[perm[old]].
			if perm != nil {
				for old := range tc.in {
					np := perm[old]
					if np < 0 || np >= len(s) {
						t.Fatalf("perm[%d]=%d out of range", old, np)
					}
					if s[np] != tc.in[old] {
						t.Errorf("perm[%d]=%d but s[%d]=%q, want %q", old, np, np, s[np], tc.in[old])
					}
				}
				// perm must be a bijection (every new position hit exactly once).
				seen := make([]bool, len(s))
				for _, np := range perm {
					if seen[np] {
						t.Fatalf("perm is not a bijection: %v", perm)
					}
					seen[np] = true
				}
			}
		})
	}
}

func join(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out
}

// ---- moveRequest: model reordering / reparenting ----------------------------

// Within the same folder, moving b (flat 1) before a (flat 0) reorders the flat
// slice so b precedes a — which is the display order for folder F1.
func TestMoveRequest_WithinFolderReorders(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	// move "b" (idx 1) to before "a" (idx 0)
	w.moveRequest(1, "F1", 0)

	if got := join(flatNames(w)); got != "b,a,c,d,e" {
		t.Fatalf("flat order = %v, want b,a,c,d,e", flatNames(w))
	}
	if folderOf(w, "b") != "F1" {
		t.Errorf("b folder = %q, want F1", folderOf(w, "b"))
	}
}

// Moving c into F1 sets its FolderID and lands it before a (top of F1).
func TestMoveRequest_AcrossFoldersSetsFolderID(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	w.moveRequest(idxOf(w, "c"), "F1", idxOf(w, "a")) // c → F1, before a

	if folderOf(w, "c") != "F1" {
		t.Fatalf("c folder = %q, want F1", folderOf(w, "c"))
	}
	// c should now sit immediately before a in the flat slice.
	if idxOf(w, "c") >= idxOf(w, "a") {
		t.Errorf("c (idx %d) should be before a (idx %d)", idxOf(w, "c"), idxOf(w, "a"))
	}
	assertSameMultiset(t, w, []string{"a", "b", "c", "d", "e"})
}

// Moving a grouped request to top-level sets FolderID "".
func TestMoveRequest_ToTopLevel(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	w.moveRequest(idxOf(w, "a"), "", noMoveTarget) // a → top-level, end

	if folderOf(w, "a") != "" {
		t.Fatalf("a folder = %q, want top-level \"\"", folderOf(w, "a"))
	}
	// End sentinel → a is last in the flat slice.
	if flatNames(w)[len(w.coll.Requests)-1] != "a" {
		t.Errorf("flat order = %v, want a last", flatNames(w))
	}
}

// Moving before a specific anchor places the request immediately before it.
func TestMoveRequest_BeforeAnchor(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	// move "e" (top-level) to before "d" (top-level)
	w.moveRequest(idxOf(w, "e"), "", idxOf(w, "d"))
	if idxOf(w, "e") != idxOf(w, "d")-1 {
		t.Fatalf("e (idx %d) should be immediately before d (idx %d)", idxOf(w, "e"), idxOf(w, "d"))
	}
}

func assertSameMultiset(t *testing.T, w *Window, want []string) {
	t.Helper()
	got := nameMultiset(w)
	sort.Strings(want)
	if join(got) != join(want) {
		t.Fatalf("request multiset = %v, want %v (a move must not lose/duplicate)", got, want)
	}
	if len(w.coll.Requests) != len(want) {
		t.Fatalf("slice length = %d, want %d (move must not change length)", len(w.coll.Requests), len(want))
	}
}

// ---- moveRequest: the index-remap (the hardest part) ------------------------

// With several open tabs spanning folders + top-level, moving one request must
// remap openTabs onto the SAME pointers under their new flat keys, sync every
// rt.idx to its new key, follow selectedID to the moved request's new index, keep
// the slice length, and lose/duplicate nothing.
func TestMoveRequest_RemapsOpenTabsAndSelection(t *testing.T) {
	w := newScopeWindow(t, movedColl())

	// Open tabs for a (0), c (2), e (4) — grab their pointers by request name so we
	// can find them after the move regardless of rekeying.
	w.openRequestTab(idxOf(w, "a"))
	w.openRequestTab(idxOf(w, "c"))
	w.openRequestTab(idxOf(w, "e"))
	rtA := w.openTabs[idxOf(w, "a")]
	rtC := w.openTabs[idxOf(w, "c")]
	rtE := w.openTabs[idxOf(w, "e")]
	if rtA == nil || rtC == nil || rtE == nil {
		t.Fatal("setup: failed to open the three tabs")
	}

	// Select "c" so we can assert selection follows it.
	w.selectedID = idxOf(w, "c")

	// Move "a" (flat 0) to the end of top-level: a big permutation that shifts
	// b,c,d,e all down by one and jumps a to the back.
	w.moveRequest(idxOf(w, "a"), "", noMoveTarget)

	// Length + multiset preserved.
	assertSameMultiset(t, w, []string{"a", "b", "c", "d", "e"})

	// No tab was opened/closed: still exactly 3 entries, same pointers.
	if len(w.openTabs) != 3 {
		t.Fatalf("openTabs has %d entries, want 3 (no tab opened/closed)", len(w.openTabs))
	}
	// Each surviving tab must live under the NEW flat index of its request, be the
	// SAME pointer, and have rt.idx == its key.
	for name, rt := range map[string]*requestTab{"a": rtA, "c": rtC, "e": rtE} {
		newKey := idxOf(w, name)
		got, ok := w.openTabs[newKey]
		if !ok {
			t.Fatalf("openTabs missing key %d for %q; keys=%v", newKey, name, keysOf(w.openTabs))
		}
		if got != rt {
			t.Errorf("openTabs[%d] for %q = %p, want original %p", newKey, name, got, rt)
		}
		if got.idx != newKey {
			t.Errorf("%q tab rt.idx = %d, want %d (key)", name, got.idx, newKey)
		}
	}

	// Selection followed "c" to its new flat index.
	if w.selectedID != idxOf(w, "c") {
		t.Errorf("selectedID = %d, want %d (should follow 'c')", w.selectedID, idxOf(w, "c"))
	}

	if !w.dirty {
		t.Errorf("moveRequest did not mark the window dirty")
	}
}

// Selection follows the MOVED request itself (not just bystanders).
func TestMoveRequest_SelectionFollowsMoved(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	w.openRequestTab(idxOf(w, "d"))
	w.selectedID = idxOf(w, "d")

	w.moveRequest(idxOf(w, "d"), "F1", idxOf(w, "a")) // d → F1, before a (jumps to front area)

	if folderOf(w, "d") != "F1" {
		t.Fatalf("d folder = %q, want F1", folderOf(w, "d"))
	}
	if w.selectedID != idxOf(w, "d") {
		t.Errorf("selectedID = %d, want %d (the moved 'd')", w.selectedID, idxOf(w, "d"))
	}
	if rt := w.openTabs[w.selectedID]; rt == nil || rt.idx != w.selectedID {
		t.Errorf("moved request's tab not correctly rekeyed to %d", w.selectedID)
	}
}

// ---- moveRequest: edge cases ------------------------------------------------

func TestMoveRequest_SrcOutOfRange(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	before := join(flatNames(w))
	w.moveRequest(99, "F1", 0)
	w.moveRequest(-1, "", noMoveTarget)
	if join(flatNames(w)) != before {
		t.Errorf("out-of-range src changed the slice: %v", flatNames(w))
	}
	if w.dirty {
		t.Errorf("out-of-range move marked dirty")
	}
}

func TestMoveRequest_UnknownDestFolder(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	before := join(flatNames(w))
	w.moveRequest(0, "NOPE", noMoveTarget)
	if join(flatNames(w)) != before {
		t.Errorf("unknown dest folder changed the slice: %v", flatNames(w))
	}
	if folderOf(w, "a") != "F1" {
		t.Errorf("unknown dest reparented anyway: a folder = %q", folderOf(w, "a"))
	}
	if w.dirty {
		t.Errorf("unknown dest folder marked dirty")
	}
}

// src == beforeReqIdx is treated as "append at end" inside the same folder, and
// since the element is already accounted for, the order is unchanged (no-op) when
// the folder is also unchanged.
func TestMoveRequest_SrcEqualsBefore(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	a := idxOf(w, "a")
	w.moveRequest(a, "F1", a) // same folder, before itself
	// No reorder, no reparent → not dirty, slice intact.
	if join(flatNames(w)) != "a,b,c,d,e" {
		t.Errorf("src==before changed slice: %v", flatNames(w))
	}
	if w.dirty {
		t.Errorf("src==before within same folder should be a no-op, but marked dirty")
	}
}

// Moving a request to where it already sits (same folder, before its current
// successor) is order-stable: no corruption, multiset intact.
func TestMoveRequest_NoOpPosition(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	// a is at flat 0; "before b" (flat 1) keeps it in place.
	w.moveRequest(idxOf(w, "a"), "F1", idxOf(w, "b"))
	if join(flatNames(w)) != "a,b,c,d,e" {
		t.Errorf("no-op position changed slice: %v", flatNames(w))
	}
	assertSameMultiset(t, w, []string{"a", "b", "c", "d", "e"})
}

// Cross-folder drop that doesn't change the flat slot still reparents + marks
// dirty (a same-position folder change is a real edit).
func TestMoveRequest_CrossFolderSamePositionReparents(t *testing.T) {
	w := newScopeWindow(t, movedColl())
	// "b" is at flat 1; moving it to F2 before c (flat 2). insertAt collapses to 1,
	// so the slice order is unchanged, but the folder must change.
	w.moveRequest(idxOf(w, "b"), "F2", idxOf(w, "c"))
	if folderOf(w, "b") != "F2" {
		t.Fatalf("b folder = %q, want F2 (reparented even at same flat slot)", folderOf(w, "b"))
	}
	if !w.dirty {
		t.Errorf("cross-folder same-position move should mark dirty")
	}
}

// Moving the only request in a single-request collection is a clean no-op-ish
// operation: still one request, no panic.
func TestMoveRequest_OnlyRequest(t *testing.T) {
	coll := model.Collection{
		Version:  1,
		Requests: []model.Request{{Method: model.MethodGet, Name: "solo"}},
	}
	w := newScopeWindow(t, coll)
	w.openRequestTab(0)
	w.selectedID = 0
	w.moveRequest(0, "", noMoveTarget)
	if len(w.coll.Requests) != 1 || w.coll.Requests[0].Name != "solo" {
		t.Fatalf("only-request move corrupted slice: %v", flatNames(w))
	}
	if w.openTabs[0] == nil || w.openTabs[0].idx != 0 || w.selectedID != 0 {
		t.Errorf("only-request move broke tab/selection state")
	}
}

// Moving a request into an empty folder sets the folder; the empty folder
// receives it and the multiset is intact.
func TestMoveRequest_IntoEmptyFolder(t *testing.T) {
	coll := movedColl()
	coll.Folders = append(coll.Folders, model.Folder{ID: "F3", Name: "Empty"})
	w := newScopeWindow(t, coll)
	w.moveRequest(idxOf(w, "d"), "F3", noMoveTarget)
	if folderOf(w, "d") != "F3" {
		t.Fatalf("d folder = %q, want F3", folderOf(w, "d"))
	}
	assertSameMultiset(t, w, []string{"a", "b", "c", "d", "e"})
}

// ---- dropTarget (pure geometry) ---------------------------------------------

// dtWindow builds a window whose w.rows is the full grouped view (no filter,
// folders expanded), matching movedColl's layout:
//
//	row 0: [folder F1]
//	row 1:   a (F1, flat 0)
//	row 2:   b (F1, flat 1)
//	row 3: [folder F2]
//	row 4:   c (F2, flat 2)
//	row 5:   d (top, flat 3)
//	row 6:   e (top, flat 4)
func dtWindow(t *testing.T) *Window {
	w := newScopeWindow(t, movedColl())
	w.rows = w.sidebarRows()
	return w
}

func TestDropTarget_OverRequestUpperHalf(t *testing.T) {
	w := dtWindow(t)
	// Over row 2 (b, flat 1), upper half → same folder F1, before b (flat 1).
	folder, before := w.dropTarget(0, 2, false)
	if folder != "F1" || before != 1 {
		t.Fatalf("upper-half over b = (%q,%d), want (F1,1)", folder, before)
	}
}

func TestDropTarget_OverRequestLowerHalf(t *testing.T) {
	w := dtWindow(t)
	// Over row 1 (a, flat 0), lower half → F1, before the NEXT request b (flat 1).
	folder, before := w.dropTarget(4, 1, true)
	if folder != "F1" || before != 1 {
		t.Fatalf("lower-half over a = (%q,%d), want (F1,1)", folder, before)
	}
}

func TestDropTarget_LowerHalfLastInGroup(t *testing.T) {
	w := dtWindow(t)
	// Over row 2 (b, last in F1), lower half → F1, end of group (sentinel).
	folder, before := w.dropTarget(0, 2, true)
	if folder != "F1" || before != noMoveTarget {
		t.Fatalf("lower-half over last-in-F1 = (%q,%d), want (F1,%d)", folder, before, noMoveTarget)
	}
}

func TestDropTarget_OverFolderHeader(t *testing.T) {
	w := dtWindow(t)
	// Over row 3 (folder F2 header) → into F2, before its first request c (flat 2).
	folder, before := w.dropTarget(0, 3, false)
	if folder != "F2" || before != 2 {
		t.Fatalf("over F2 header = (%q,%d), want (F2,2)", folder, before)
	}
}

func TestDropTarget_OverEmptyOrCollapsedFolderHeader(t *testing.T) {
	coll := movedColl()
	coll.Folders = append(coll.Folders, model.Folder{ID: "F3", Name: "Empty"})
	w := newScopeWindow(t, coll)
	w.rows = w.sidebarRows()
	// Find F3's header row.
	hdr := -1
	for i, r := range w.rows {
		if r.IsFolder && r.FolderID == "F3" {
			hdr = i
		}
	}
	if hdr < 0 {
		t.Fatal("setup: F3 header row not found")
	}
	folder, before := w.dropTarget(0, hdr, false)
	if folder != "F3" || before != noMoveTarget {
		t.Fatalf("over empty F3 header = (%q,%d), want (F3,%d)", folder, before, noMoveTarget)
	}
}

func TestDropTarget_BelowLastRow(t *testing.T) {
	w := dtWindow(t)
	// visibleRow past the end → top-level, end.
	folder, before := w.dropTarget(0, len(w.rows), false)
	if folder != "" || before != noMoveTarget {
		t.Fatalf("below last row = (%q,%d), want (\"\",%d)", folder, before, noMoveTarget)
	}
	// Negative row → same fallback.
	folder, before = w.dropTarget(0, -1, true)
	if folder != "" || before != noMoveTarget {
		t.Fatalf("negative row = (%q,%d), want (\"\",%d)", folder, before, noMoveTarget)
	}
}

// dropTarget feeds moveRequest end-to-end: dragging c onto F1's header should
// land c at the top of F1.
func TestDropTarget_FeedsMoveRequest(t *testing.T) {
	w := dtWindow(t)
	src := idxOf(w, "c")
	folder, before := w.dropTarget(src, 0, false) // row 0 = F1 header
	w.moveRequest(src, folder, before)
	if folderOf(w, "c") != "F1" {
		t.Fatalf("c folder = %q, want F1", folderOf(w, "c"))
	}
	// c should be first among F1's flat requests (before a).
	if idxOf(w, "c") >= idxOf(w, "a") {
		t.Errorf("c (idx %d) not above a (idx %d)", idxOf(w, "c"), idxOf(w, "a"))
	}
}
