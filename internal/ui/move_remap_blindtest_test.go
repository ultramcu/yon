package ui

// BLIND tests for the data-layer drag-reorder core: moveRequest + dropTarget.
// Written from the SPEC, independent of the other move/drag test files.

import (
	"sort"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// mrbtWindow builds a Window with len(names) distinctly-named top-level GET
// requests, in the given order, and refreshes the sidebar so w.rows is current.
func mrbtWindow(t *testing.T, names ...string) *Window {
	t.Helper()
	coll := model.NewCollection("BlindMove")
	reqs := make([]model.Request, len(names))
	for i, n := range names {
		reqs[i] = model.Request{Name: n, Method: model.MethodGet, URL: "http://x/" + n}
	}
	coll.Requests = reqs
	w := newTestWindow(coll)
	w.refreshSidebar()
	return w
}

// mrbtReqNames returns the flat Collection.Requests names in flat order.
func mrbtReqNames(w *Window) []string {
	out := make([]string, len(w.coll.Requests))
	for i := range w.coll.Requests {
		out[i] = w.coll.Requests[i].Name
	}
	return out
}

// mrbtSortedNames returns the request-name multiset (sorted) of the flat slice.
func mrbtSortedNames(w *Window) []string {
	out := mrbtReqNames(w)
	sort.Strings(out)
	return out
}

// mrbtDisplayReqNames returns the names of the REQUEST rows in sidebar display
// order (folder headers skipped), each looked up by its row.ReqIdx.
func mrbtDisplayReqNames(w *Window) []string {
	var out []string
	for _, r := range w.sidebarRows() {
		if r.IsFolder {
			continue
		}
		out = append(out, w.coll.Requests[r.ReqIdx].Name)
	}
	return out
}

// mrbtFlatIdxOf returns the flat index of the first request named name, or -1.
func mrbtFlatIdxOf(w *Window, name string) int {
	for i := range w.coll.Requests {
		if w.coll.Requests[i].Name == name {
			return i
		}
	}
	return -1
}

// mrbtEqual reports a == b for string slices.
func mrbtEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// SPEC 1: Within-folder (here: top-level) reorder changes display order; flat
// length and the request multiset are unchanged.
// ---------------------------------------------------------------------------

func TestMRBT_WithinReorder_ChangesDisplayOrderKeepsMultiset(t *testing.T) {
	w := mrbtWindow(t, "Alpha", "Bravo", "Charlie", "Delta")

	beforeLen := len(w.coll.Requests)
	beforeMulti := mrbtSortedNames(w)
	beforeDisplay := mrbtDisplayReqNames(w) // Alpha Bravo Charlie Delta

	// Move Charlie (idx 2) to immediately before Alpha (idx 0).
	w.moveRequest(2, "", 0)

	if got := len(w.coll.Requests); got != beforeLen {
		t.Fatalf("flat length changed: before %d after %d", beforeLen, got)
	}
	if got := mrbtSortedNames(w); !mrbtEqual(got, beforeMulti) {
		t.Fatalf("request multiset changed: before %v after %v", beforeMulti, got)
	}
	afterDisplay := mrbtDisplayReqNames(w)
	if mrbtEqual(afterDisplay, beforeDisplay) {
		t.Fatalf("display order unchanged, expected reorder: %v", afterDisplay)
	}
	want := []string{"Charlie", "Alpha", "Bravo", "Delta"}
	if !mrbtEqual(afterDisplay, want) {
		t.Fatalf("display order = %v, want %v", afterDisplay, want)
	}
}

// ---------------------------------------------------------------------------
// SPEC 2: Cross-folder move sets FolderID and renders under that folder; moving
// back out (destFolderID "") returns to top level.
// ---------------------------------------------------------------------------

func TestMRBT_CrossFolder_IntoFolderThenBackOut(t *testing.T) {
	w := mrbtWindow(t, "One", "Two", "Three")
	fid := w.addFolder("Box")
	w.refreshSidebar()

	twoIdx := mrbtFlatIdxOf(w, "Two")
	// Move "Two" into folder Box, at end of that group.
	w.moveRequest(twoIdx, fid, noMoveTarget)

	twoIdx = mrbtFlatIdxOf(w, "Two")
	if got := w.coll.Requests[twoIdx].FolderID; got != fid {
		t.Fatalf("after move-in FolderID = %q, want %q", got, fid)
	}

	// Confirm "Two" renders under the Box header (appears after the folder row,
	// before any later top-level row), and carries FolderID fid in its row.
	rows := w.sidebarRows()
	var sawHeader, twoUnderFolder bool
	for _, r := range rows {
		if r.IsFolder && r.FolderID == fid {
			sawHeader = true
			continue
		}
		if !r.IsFolder && w.coll.Requests[r.ReqIdx].Name == "Two" {
			if sawHeader && r.FolderID == fid {
				twoUnderFolder = true
			}
		}
	}
	if !twoUnderFolder {
		t.Fatalf("Two did not render under folder %q; rows=%+v", fid, rows)
	}

	// Now move it back out to top level.
	twoIdx = mrbtFlatIdxOf(w, "Two")
	w.moveRequest(twoIdx, "", noMoveTarget)
	twoIdx = mrbtFlatIdxOf(w, "Two")
	if got := w.coll.Requests[twoIdx].FolderID; got != "" {
		t.Fatalf("after move-out FolderID = %q, want top-level \"\"", got)
	}
	// And its row is top-level (FolderID "") again.
	for _, r := range w.sidebarRows() {
		if !r.IsFolder && w.coll.Requests[r.ReqIdx].Name == "Two" {
			if r.FolderID != "" {
				t.Fatalf("Two row FolderID = %q after move-out, want \"\"", r.FolderID)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// SPEC 3 (CRITICAL): index remap. Open several tabs across folders + top-level,
// move one, and assert openTabs keeps the SAME pointers under remapped keys,
// every rt.idx equals its key, selectedID follows the SAME request, length
// unchanged, no request lost/duplicated.
// ---------------------------------------------------------------------------

// mrbtTabByName returns the *requestTab whose flat index currently names name,
// by scanning openTabs and matching the request name at rt.idx.
func mrbtTabByName(w *Window, name string) *requestTab {
	for _, rt := range w.openTabs {
		if rt.idx >= 0 && rt.idx < len(w.coll.Requests) && w.coll.Requests[rt.idx].Name == name {
			return rt
		}
	}
	return nil
}

// mrbtCheckTabInvariants asserts every openTabs entry is keyed by its own
// rt.idx (key == rt.idx).
func mrbtCheckTabInvariants(t *testing.T, w *Window) {
	t.Helper()
	for key, rt := range w.openTabs {
		if rt.idx != key {
			t.Fatalf("openTabs key %d != rt.idx %d (request %q)", key, rt.idx,
				w.coll.Requests[rt.idx].Name)
		}
	}
}

func TestMRBT_Remap_MoveSelectedRequest(t *testing.T) {
	w := mrbtWindow(t, "Ada", "Boole", "Curie", "Dirac", "Euler")
	fid := w.addFolder("Sci")
	// Put Curie + Dirac into the folder (no reorder; flat indices stable).
	w.moveRequestToFolder(mrbtFlatIdxOf(w, "Curie"), fid)
	w.moveRequestToFolder(mrbtFlatIdxOf(w, "Dirac"), fid)
	w.refreshSidebar()

	// Open tabs for several across folder + top-level.
	for _, n := range []string{"Ada", "Curie", "Dirac", "Euler"} {
		w.openRequestTab(mrbtFlatIdxOf(w, n))
	}
	// Record the pointers by name BEFORE the move.
	ptrAda := mrbtTabByName(w, "Ada")
	ptrCurie := mrbtTabByName(w, "Curie")
	ptrDirac := mrbtTabByName(w, "Dirac")
	ptrEuler := mrbtTabByName(w, "Euler")
	if ptrAda == nil || ptrCurie == nil || ptrDirac == nil || ptrEuler == nil {
		t.Fatalf("setup: not all tabs opened")
	}
	wantTabCount := len(w.openTabs)

	// Select the request we are about to MOVE.
	sel := mrbtFlatIdxOf(w, "Curie")
	w.selectedID = sel
	selName := "Curie"

	beforeLen := len(w.coll.Requests)
	beforeMulti := mrbtSortedNames(w)

	// Move Curie to top-level end. This permutes flat indices broadly.
	w.moveRequest(mrbtFlatIdxOf(w, "Curie"), "", noMoveTarget)

	// Length + multiset preserved (nothing lost/duplicated).
	if got := len(w.coll.Requests); got != beforeLen {
		t.Fatalf("flat length changed: before %d after %d", beforeLen, got)
	}
	if got := mrbtSortedNames(w); !mrbtEqual(got, beforeMulti) {
		t.Fatalf("multiset changed: before %v after %v", beforeMulti, got)
	}
	// No tab opened or closed.
	if got := len(w.openTabs); got != wantTabCount {
		t.Fatalf("openTabs count changed: before %d after %d", wantTabCount, got)
	}
	// Same pointers, under remapped keys.
	if got := mrbtTabByName(w, "Ada"); got != ptrAda {
		t.Fatalf("Ada tab pointer changed: %p != %p", got, ptrAda)
	}
	if got := mrbtTabByName(w, "Curie"); got != ptrCurie {
		t.Fatalf("Curie tab pointer changed: %p != %p", got, ptrCurie)
	}
	if got := mrbtTabByName(w, "Dirac"); got != ptrDirac {
		t.Fatalf("Dirac tab pointer changed: %p != %p", got, ptrDirac)
	}
	if got := mrbtTabByName(w, "Euler"); got != ptrEuler {
		t.Fatalf("Euler tab pointer changed: %p != %p", got, ptrEuler)
	}
	// Every rt.idx equals its map key.
	mrbtCheckTabInvariants(t, w)
	// selectedID still points at the SAME request (the moved one).
	if w.selectedID < 0 || w.selectedID >= len(w.coll.Requests) {
		t.Fatalf("selectedID out of range after move: %d", w.selectedID)
	}
	if got := w.coll.Requests[w.selectedID].Name; got != selName {
		t.Fatalf("selectedID now names %q, want moved request %q", got, selName)
	}
}

func TestMRBT_Remap_MoveDifferentRequestWhileOtherSelected(t *testing.T) {
	w := mrbtWindow(t, "Ada", "Boole", "Curie", "Dirac", "Euler")
	fid := w.addFolder("Sci")
	w.moveRequestToFolder(mrbtFlatIdxOf(w, "Boole"), fid)
	w.moveRequestToFolder(mrbtFlatIdxOf(w, "Curie"), fid)
	w.refreshSidebar()

	for _, n := range []string{"Ada", "Boole", "Curie", "Euler"} {
		w.openRequestTab(mrbtFlatIdxOf(w, n))
	}
	ptrAda := mrbtTabByName(w, "Ada")
	ptrBoole := mrbtTabByName(w, "Boole")
	ptrCurie := mrbtTabByName(w, "Curie")
	ptrEuler := mrbtTabByName(w, "Euler")
	wantTabCount := len(w.openTabs)

	// Select EULER, but MOVE a DIFFERENT request (Ada into folder Sci).
	w.selectedID = mrbtFlatIdxOf(w, "Euler")

	beforeLen := len(w.coll.Requests)
	beforeMulti := mrbtSortedNames(w)

	w.moveRequest(mrbtFlatIdxOf(w, "Ada"), fid, noMoveTarget)

	if got := len(w.coll.Requests); got != beforeLen {
		t.Fatalf("flat length changed: before %d after %d", beforeLen, got)
	}
	if got := mrbtSortedNames(w); !mrbtEqual(got, beforeMulti) {
		t.Fatalf("multiset changed: before %v after %v", beforeMulti, got)
	}
	if got := len(w.openTabs); got != wantTabCount {
		t.Fatalf("openTabs count changed: before %d after %d", wantTabCount, got)
	}
	if mrbtTabByName(w, "Ada") != ptrAda ||
		mrbtTabByName(w, "Boole") != ptrBoole ||
		mrbtTabByName(w, "Curie") != ptrCurie ||
		mrbtTabByName(w, "Euler") != ptrEuler {
		t.Fatalf("a tab pointer changed across a move of a different request")
	}
	mrbtCheckTabInvariants(t, w)
	// Selection still on Euler (the one we did NOT move).
	if got := w.coll.Requests[w.selectedID].Name; got != "Euler" {
		t.Fatalf("selectedID now names %q, want still-selected %q", got, "Euler")
	}
}

// ---------------------------------------------------------------------------
// SPEC 4: a real move marks the window dirty.
// ---------------------------------------------------------------------------

func TestMRBT_Dirty_RealMoveMarksDirty(t *testing.T) {
	w := mrbtWindow(t, "P", "Q", "R")
	w.dirty = false
	w.moveRequest(2, "", 0) // R before P — a real reorder
	if !w.dirty {
		t.Fatalf("dirty not set after a real move")
	}
}

// ---------------------------------------------------------------------------
// SPEC 5: edges / no corruption. Each must not panic and must leave state
// consistent (length + multiset preserved, every tab key == rt.idx).
// ---------------------------------------------------------------------------

func TestMRBT_Edges_NoCorruption(t *testing.T) {
	type tc struct {
		name string
		run  func(t *testing.T)
	}
	cases := []tc{
		{"SrcOutOfRange", func(t *testing.T) {
			w := mrbtWindow(t, "A", "B", "C")
			before := mrbtReqNames(w)
			w.moveRequest(99, "", 0)
			w.moveRequest(-1, "", 0)
			if !mrbtEqual(mrbtReqNames(w), before) {
				t.Fatalf("out-of-range src changed slice: %v", mrbtReqNames(w))
			}
		}},
		{"UnknownDestFolder", func(t *testing.T) {
			w := mrbtWindow(t, "A", "B", "C")
			before := mrbtReqNames(w)
			w.moveRequest(0, "no-such-folder", noMoveTarget)
			if !mrbtEqual(mrbtReqNames(w), before) {
				t.Fatalf("unknown dest folder changed slice: %v", mrbtReqNames(w))
			}
			if w.coll.Requests[0].FolderID != "" {
				t.Fatalf("unknown dest folder reparented request to %q", w.coll.Requests[0].FolderID)
			}
		}},
		{"SrcEqualsBefore", func(t *testing.T) {
			w := mrbtWindow(t, "A", "B", "C")
			before := mrbtReqNames(w)
			w.moveRequest(1, "", 1) // before yourself == stay put
			if !mrbtEqual(mrbtReqNames(w), before) {
				t.Fatalf("src==before changed slice: %v", mrbtReqNames(w))
			}
		}},
		{"MoveWhereAlreadyIs", func(t *testing.T) {
			w := mrbtWindow(t, "A", "B", "C")
			before := mrbtReqNames(w)
			// Move B (idx1) before C (idx2): B is already immediately before C.
			w.moveRequest(1, "", 2)
			if !mrbtEqual(mrbtReqNames(w), before) {
				t.Fatalf("redundant move changed display order: %v", mrbtReqNames(w))
			}
		}},
		{"OnlyRequest", func(t *testing.T) {
			w := mrbtWindow(t, "Solo")
			w.moveRequest(0, "", noMoveTarget)
			w.moveRequest(0, "", 0)
			if got := mrbtReqNames(w); !mrbtEqual(got, []string{"Solo"}) {
				t.Fatalf("only-request move corrupted slice: %v", got)
			}
		}},
		{"IntoEmptyFolder", func(t *testing.T) {
			w := mrbtWindow(t, "A", "B")
			fid := w.addFolder("Empty")
			w.refreshSidebar()
			w.moveRequest(0, fid, noMoveTarget)
			ai := mrbtFlatIdxOf(w, "A")
			if w.coll.Requests[ai].FolderID != fid {
				t.Fatalf("move into empty folder did not reparent: FolderID=%q", w.coll.Requests[ai].FolderID)
			}
			if got := mrbtSortedNames(w); !mrbtEqual(got, []string{"A", "B"}) {
				t.Fatalf("move into empty folder changed multiset: %v", got)
			}
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			c.run(t)
		})
	}
}

// ---------------------------------------------------------------------------
// SPEC 6: dropTarget geometry. For each gesture we both assert the returned
// (destFolderID, beforeReqIdx) pair AND feed it to moveRequest to confirm the
// dragged request lands where expected in display order.
// ---------------------------------------------------------------------------

// mrbtRowIndexOfReq returns the visible-row index of the request named name in
// w.rows, or -1.
func mrbtRowIndexOfReq(w *Window, name string) int {
	for i, r := range w.rows {
		if !r.IsFolder && w.coll.Requests[r.ReqIdx].Name == name {
			return i
		}
	}
	return -1
}

// mrbtRowIndexOfFolder returns the visible-row index of the header for folderID.
func mrbtRowIndexOfFolder(w *Window, folderID string) int {
	for i, r := range w.rows {
		if r.IsFolder && r.FolderID == folderID {
			return i
		}
	}
	return -1
}

func TestMRBT_DropTarget_RequestRowUpperHalf(t *testing.T) {
	w := mrbtWindow(t, "A", "B", "C")
	w.refreshSidebar()
	// Upper half over B → top-level, anchor = B's flat index.
	row := mrbtRowIndexOfReq(w, "B")
	bIdx := mrbtFlatIdxOf(w, "B")
	dest, before := w.dropTarget(mrbtFlatIdxOf(w, "C"), row, false)
	if dest != "" || before != bIdx {
		t.Fatalf("upper-half over B => (%q,%d), want (\"\",%d)", dest, before, bIdx)
	}
	// Feed it: moving C before B → display A C B.
	w.moveRequest(mrbtFlatIdxOf(w, "C"), dest, before)
	if got := mrbtDisplayReqNames(w); !mrbtEqual(got, []string{"A", "C", "B"}) {
		t.Fatalf("after drop landing = %v, want [A C B]", got)
	}
}

func TestMRBT_DropTarget_RequestRowLowerHalf(t *testing.T) {
	w := mrbtWindow(t, "A", "B", "C")
	w.refreshSidebar()
	// Lower half over A → anchor = NEXT request in group (B).
	row := mrbtRowIndexOfReq(w, "A")
	bIdx := mrbtFlatIdxOf(w, "B")
	dest, before := w.dropTarget(mrbtFlatIdxOf(w, "C"), row, true)
	if dest != "" || before != bIdx {
		t.Fatalf("lower-half over A => (%q,%d), want anchor=next(B)=%d", dest, before, bIdx)
	}
	// Moving C lower-half-of-A → C lands after A, before B → A C B.
	w.moveRequest(mrbtFlatIdxOf(w, "C"), dest, before)
	if got := mrbtDisplayReqNames(w); !mrbtEqual(got, []string{"A", "C", "B"}) {
		t.Fatalf("after drop landing = %v, want [A C B]", got)
	}
}

func TestMRBT_DropTarget_LowerHalfOfLastInGroupIsEnd(t *testing.T) {
	w := mrbtWindow(t, "A", "B", "C")
	w.refreshSidebar()
	// Lower half over C (last top-level) → end sentinel.
	row := mrbtRowIndexOfReq(w, "C")
	dest, before := w.dropTarget(mrbtFlatIdxOf(w, "A"), row, true)
	if dest != "" || before != noMoveTarget {
		t.Fatalf("lower-half of last C => (%q,%d), want (\"\",noMoveTarget)", dest, before)
	}
	// Moving A to the end → B C A.
	w.moveRequest(mrbtFlatIdxOf(w, "A"), dest, before)
	if got := mrbtDisplayReqNames(w); !mrbtEqual(got, []string{"B", "C", "A"}) {
		t.Fatalf("after end-drop landing = %v, want [B C A]", got)
	}
}

func TestMRBT_DropTarget_FolderHeaderTopOfGroup(t *testing.T) {
	w := mrbtWindow(t, "A", "B", "C")
	fid := w.addFolder("Box")
	// Put B and C into the folder so the folder has a first request.
	w.moveRequestToFolder(mrbtFlatIdxOf(w, "B"), fid)
	w.moveRequestToFolder(mrbtFlatIdxOf(w, "C"), fid)
	w.refreshSidebar()

	headerRow := mrbtRowIndexOfFolder(w, fid)
	if headerRow < 0 {
		t.Fatalf("folder header row not found")
	}
	// Drop onto the folder header → that folder, before its first request (B).
	bIdx := mrbtFlatIdxOf(w, "B")
	dest, before := w.dropTarget(mrbtFlatIdxOf(w, "A"), headerRow, false)
	if dest != fid || before != bIdx {
		t.Fatalf("on folder header => (%q,%d), want (%q,%d=firstReq B)", dest, before, fid, bIdx)
	}
	// Feed it: A moves into folder at TOP → folder group display [A B C].
	w.moveRequest(mrbtFlatIdxOf(w, "A"), dest, before)
	if got := w.coll.Requests[mrbtFlatIdxOf(w, "A")].FolderID; got != fid {
		t.Fatalf("A not reparented into folder, FolderID=%q", got)
	}
	// Display order of the folder's requests should start with A.
	got := mrbtDisplayReqNames(w)
	if !mrbtEqual(got, []string{"A", "B", "C"}) {
		t.Fatalf("after folder-header drop display = %v, want [A B C]", got)
	}
}

func TestMRBT_DropTarget_EmptyFolderHeader(t *testing.T) {
	w := mrbtWindow(t, "A", "B")
	fid := w.addFolder("Empty")
	w.refreshSidebar()
	headerRow := mrbtRowIndexOfFolder(w, fid)
	dest, before := w.dropTarget(mrbtFlatIdxOf(w, "A"), headerRow, false)
	if dest != fid || before != noMoveTarget {
		t.Fatalf("empty folder header => (%q,%d), want (%q,noMoveTarget)", dest, before, fid)
	}
	w.moveRequest(mrbtFlatIdxOf(w, "A"), dest, before)
	if got := w.coll.Requests[mrbtFlatIdxOf(w, "A")].FolderID; got != fid {
		t.Fatalf("A not reparented into empty folder, FolderID=%q", got)
	}
}

func TestMRBT_DropTarget_BelowListAndOutOfRange(t *testing.T) {
	w := mrbtWindow(t, "A", "B", "C")
	w.refreshSidebar()
	// visibleRow past the end → top-level, end.
	dest, before := w.dropTarget(mrbtFlatIdxOf(w, "A"), len(w.rows), false)
	if dest != "" || before != noMoveTarget {
		t.Fatalf("below list => (%q,%d), want (\"\",noMoveTarget)", dest, before)
	}
	// Negative visibleRow → same.
	dest, before = w.dropTarget(mrbtFlatIdxOf(w, "A"), -5, true)
	if dest != "" || before != noMoveTarget {
		t.Fatalf("negative row => (%q,%d), want (\"\",noMoveTarget)", dest, before)
	}
	// Feeding the below-list result moves A to the very end → B C A.
	w.moveRequest(mrbtFlatIdxOf(w, "A"), "", noMoveTarget)
	if got := mrbtDisplayReqNames(w); !mrbtEqual(got, []string{"B", "C", "A"}) {
		t.Fatalf("below-list drop landing = %v, want [B C A]", got)
	}
}
