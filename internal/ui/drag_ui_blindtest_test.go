package ui

// Blind, independent tests for the Fyne DRAG UI wiring on sidebar request rows
// (verbRow as fyne.Draggable) plus the no-regression guarantees. Written from the
// SPEC only; does not read drag_reorder_test.go / move_test.go. Helper names are
// suffixed _bt2 to avoid collisions with the other author's helpers.

import (
	"testing"

	"fyne.io/fyne/v2"

	"github.com/ultramcu/yon/internal/model"
)

// layoutWindow_bt2 resizes the window to a real size and forces a layout pass so
// sidebar rows have non-zero geometry (mirrors sidebar_tap_routing_test.go).
func layoutWindow_bt2(t *testing.T, w *Window) {
	t.Helper()
	w.win.Resize(fyne.NewSize(1100, 720))
	w.sidebar.Refresh()
	w.sidebar.Resize(w.sidebar.Size())
}

// dragEvt_bt2 builds a DragEvent whose pointer sits at absolute (x, y).
func dragEvt_bt2(x, y, dx, dy float32) *fyne.DragEvent {
	return &fyne.DragEvent{
		PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(x, y)},
		Dragged:    fyne.NewDelta(dx, dy),
	}
}

// reqNames_bt2 returns the request DisplayNames in flat order, for readable asserts.
func reqNames_bt2(w *Window) []string {
	out := make([]string, len(w.coll.Requests))
	for i := range w.coll.Requests {
		out[i] = w.coll.Requests[i].DisplayName()
	}
	return out
}

// reqIdxByName_bt2 returns the flat index of the request whose DisplayName is name.
func reqIdxByName_bt2(t *testing.T, w *Window, name string) int {
	t.Helper()
	for i := range w.coll.Requests {
		if w.coll.Requests[i].DisplayName() == name {
			return i
		}
	}
	t.Fatalf("no request named %q; have %v", name, reqNames_bt2(w))
	return -1
}

// ---------------------------------------------------------------------------
// SPEC 1: interfaces coexist; tap-to-open still works.
// ---------------------------------------------------------------------------

func TestBT2_VerbRowSatisfiesBothInterfaces(t *testing.T) {
	var r interface{} = newVerbRow()
	if _, ok := r.(fyne.Draggable); !ok {
		t.Error("*verbRow must satisfy fyne.Draggable")
	}
	if _, ok := r.(fyne.Tappable); !ok {
		t.Error("*verbRow must satisfy fyne.Tappable")
	}
}

func TestBT2_TapStillOpensRequest(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{
		{Name: "alpha", Method: model.MethodGet, URL: "http://x/a"},
		{Name: "bravo", Method: model.MethodPost, URL: "http://x/b"},
	}
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)
	layoutWindow_bt2(t, w)

	row := findVerbRowFor(t, w, "bravo")
	// Drive the tap path directly (Draggable addition must not break Tapped).
	row.Tapped(&fyne.PointEvent{})

	if len(w.openTabs) != 1 {
		t.Fatalf("tap should open exactly one tab, got %d (openTabs=%v)", len(w.openTabs), keysOf(w.openTabs))
	}
	if _, ok := w.openTabs[1]; !ok {
		t.Fatalf("tap should open request index 1 (bravo); openTabs=%v", keysOf(w.openTabs))
	}
	if w.selectedID != 1 {
		t.Fatalf("selectedID = %d, want 1 after tapping bravo", w.selectedID)
	}
}

// ---------------------------------------------------------------------------
// SPEC 2: drag tracking — Dragged then DragEnd fires onDragEnd(id, lastY);
// pure tap (no Dragged) does not fire; sub-deadzone drag does not fire.
// ---------------------------------------------------------------------------

func TestBT2_DragEndFiresWithLastY(t *testing.T) {
	r := newVerbRow()
	r.id = 7
	var gotID int = -999
	var gotY float32 = -999
	fired := false
	r.onDragEnd = func(srcReqIdx int, dropAbsY float32) {
		fired = true
		gotID = srcReqIdx
		gotY = dropAbsY
	}

	r.Dragged(dragEvt_bt2(10, 100, 0, 0))
	r.Dragged(dragEvt_bt2(10, 150, 0, 50))
	r.Dragged(dragEvt_bt2(10, 200, 0, 50)) // last Y = 200, total travel 100 >> deadzone
	r.DragEnd()

	if !fired {
		t.Fatal("onDragEnd should fire after a real drag")
	}
	if gotID != 7 {
		t.Errorf("onDragEnd id = %d, want 7 (the row id)", gotID)
	}
	if gotY != 200 {
		t.Errorf("onDragEnd dropAbsY = %v, want 200 (the LAST Y seen)", gotY)
	}
}

func TestBT2_PureTapDoesNotFireDragEnd(t *testing.T) {
	r := newVerbRow()
	r.id = 3
	fired := false
	r.onDragEnd = func(int, float32) { fired = true }

	// No Dragged at all — a plain click results in only DragEnd (or nothing).
	r.DragEnd()

	if fired {
		t.Fatal("onDragEnd must NOT fire when there was no prior Dragged (pure tap)")
	}
}

func TestBT2_SubDeadzoneDragDoesNotFire(t *testing.T) {
	r := newVerbRow()
	r.id = 5
	fired := false
	r.onDragEnd = func(int, float32) { fired = true }

	// Total vertical travel below dragDeadzone (5px): click-jitter must not reorder.
	r.Dragged(dragEvt_bt2(10, 100, 0, 0))
	r.Dragged(dragEvt_bt2(10, 102, 0, 2))
	r.DragEnd()

	if fired {
		t.Fatalf("onDragEnd must NOT fire for sub-deadzone travel (<%v px)", dragDeadzone)
	}
}

func TestBT2_DragStateResetsBetweenGestures(t *testing.T) {
	r := newVerbRow()
	r.id = 1
	fires := 0
	r.onDragEnd = func(int, float32) { fires++ }

	// First gesture: real drag, should fire once.
	r.Dragged(dragEvt_bt2(10, 100, 0, 0))
	r.Dragged(dragEvt_bt2(10, 200, 0, 100))
	r.DragEnd()
	if fires != 1 {
		t.Fatalf("first gesture should fire once, got %d", fires)
	}

	// Second gesture: pure tap (no Dragged). Stale start/last must NOT leak through.
	r.DragEnd()
	if fires != 1 {
		t.Fatalf("second (tap) gesture must not fire; total fires = %d, want 1", fires)
	}
}

// ---------------------------------------------------------------------------
// SPEC 3: handleRequestDrop end-to-end with real geometry.
// ---------------------------------------------------------------------------

// rowCenterAbsY_bt2 returns the absolute centre Y of the verbRow or folderRow at
// the given content/text name. It walks the laid-out tree for the row.
func centerYOfVerb_bt2(t *testing.T, w *Window, name string) float32 {
	t.Helper()
	row := findVerbRowFor(t, w, name)
	if row.Size().IsZero() {
		t.Fatalf("verbRow %q has zero size; sidebar did not lay out", name)
	}
	return centerOf(w.app.fyneApp, row).Y
}

func TestBT2_DropReordersWithinTopLevel(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{
		{Name: "one", Method: model.MethodGet, URL: "http://x/1"},
		{Name: "two", Method: model.MethodGet, URL: "http://x/2"},
		{Name: "three", Method: model.MethodGet, URL: "http://x/3"},
	}
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)
	layoutWindow_bt2(t, w)

	// Open a tab so we can confirm openTabs stays consistent through the move.
	w.openRequestTab(0) // "one"
	if _, ok := w.openTabs[0]; !ok {
		t.Fatal("precondition: tab for 'one' should be open at idx 0")
	}

	// Drag "one" (idx 0) onto the centre of "three".
	srcIdx := reqIdxByName_bt2(t, w, "one")
	targetY := centerYOfVerb_bt2(t, w, "three")
	w.handleRequestDrop(srcIdx, targetY)

	// "one" should no longer be first; order should have changed.
	got := reqNames_bt2(w)
	if got[0] == "one" {
		t.Fatalf("expected 'one' to move away from the front; order = %v", got)
	}
	// The set of names must be preserved (move, not copy/loss).
	if len(got) != 3 {
		t.Fatalf("request count changed: %v", got)
	}
	// openTabs must still point at the SAME request "one" (its idx may have moved).
	oneIdx := reqIdxByName_bt2(t, w, "one")
	if _, ok := w.openTabs[oneIdx]; !ok {
		t.Fatalf("open tab for 'one' lost/misindexed; one now at %d, openTabs=%v", oneIdx, keysOf(w.openTabs))
	}
	if w.openTabs[oneIdx].idx != oneIdx {
		t.Fatalf("tab.idx (%d) out of sync with flat idx (%d)", w.openTabs[oneIdx].idx, oneIdx)
	}
}

func TestBT2_DropTopLevelOntoFolderHeaderJoinsFolder(t *testing.T) {
	coll := model.NewCollection("C")
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)

	fid := w.addFolder("grp")
	w.coll.Requests = []model.Request{
		{Name: "inside", Method: model.MethodGet, URL: "http://x/i", FolderID: fid},
		{Name: "loose", Method: model.MethodGet, URL: "http://x/l"}, // top-level
	}
	w.refreshSidebar()
	layoutWindow_bt2(t, w)

	srcIdx := reqIdxByName_bt2(t, w, "loose")
	if w.coll.Requests[srcIdx].FolderID != "" {
		t.Fatal("precondition: 'loose' should start top-level")
	}

	// Drop "loose" onto the folder HEADER row. The folder header is a folderRow.
	hdrY := folderHeaderCenterY_bt2(t, w, "grp")
	w.handleRequestDrop(srcIdx, hdrY)

	li := reqIdxByName_bt2(t, w, "loose")
	if w.coll.Requests[li].FolderID != fid {
		t.Fatalf("'loose' should have joined folder %q after dropping on its header; FolderID=%q",
			fid, w.coll.Requests[li].FolderID)
	}
}

func TestBT2_DropFolderRequestOntoTopLevelLeavesFolder(t *testing.T) {
	coll := model.NewCollection("C")
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)

	fid := w.addFolder("grp")
	w.coll.Requests = []model.Request{
		{Name: "child", Method: model.MethodGet, URL: "http://x/c", FolderID: fid},
		{Name: "top", Method: model.MethodGet, URL: "http://x/t"}, // top-level
	}
	w.refreshSidebar()
	layoutWindow_bt2(t, w)

	srcIdx := reqIdxByName_bt2(t, w, "child")
	if w.coll.Requests[srcIdx].FolderID != fid {
		t.Fatal("precondition: 'child' should start in the folder")
	}

	// Drop "child" onto the top-level request "top".
	targetY := centerYOfVerb_bt2(t, w, "top")
	w.handleRequestDrop(srcIdx, targetY)

	ci := reqIdxByName_bt2(t, w, "child")
	if w.coll.Requests[ci].FolderID != "" {
		t.Fatalf("'child' should have left the folder (FolderID \"\") after dropping on a top-level row; got %q",
			w.coll.Requests[ci].FolderID)
	}
}

// folderHeaderCenterY_bt2 returns the absolute centre Y of the folder header row
// whose name label reads name, found by walking the laid-out tree.
func folderHeaderCenterY_bt2(t *testing.T, w *Window, name string) float32 {
	t.Helper()
	var match *folderRow
	walkObjects(w.app.fyneApp, w.win.Canvas().Content(), func(o fyne.CanvasObject) {
		if fr, ok := o.(*folderRow); ok && fr.name != nil && fr.name.Text == name {
			match = fr
		}
	})
	if match == nil {
		t.Fatalf("no rendered folderRow named %q found", name)
	}
	if match.Size().IsZero() {
		t.Fatalf("folderRow %q has zero size; sidebar did not lay out", name)
	}
	return centerOf(w.app.fyneApp, match).Y
}

// ---------------------------------------------------------------------------
// SPEC 4: filter disables drag.
// ---------------------------------------------------------------------------

func TestBT2_FilterDisablesDrop(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{
		{Name: "alpha", Method: model.MethodGet, URL: "http://x/a"},
		{Name: "beta", Method: model.MethodGet, URL: "http://x/b"},
		{Name: "gamma", Method: model.MethodGet, URL: "http://x/g"},
	}
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)
	layoutWindow_bt2(t, w)

	before := reqNames_bt2(w)

	// Activate the filter; drag-drop must become a no-op.
	w.filterQuery = "alpha"
	w.dirty = false

	srcIdx := reqIdxByName_bt2(t, w, "alpha")
	targetY := centerYOfVerb_bt2(t, w, "alpha")
	w.handleRequestDrop(srcIdx, targetY+500) // any Y; should be ignored entirely

	if got := reqNames_bt2(w); !equalStrs_bt2(got, before) {
		t.Fatalf("filter active: order must not change; before=%v after=%v", before, got)
	}
	if w.dirty {
		t.Fatal("filter active: handleRequestDrop must not mark the window dirty")
	}

	// Clear the filter → drag re-enabled: a real reorder should now take effect.
	w.filterQuery = ""
	w.refreshSidebar()
	w.sidebar.Resize(w.sidebar.Size())

	src2 := reqIdxByName_bt2(t, w, "alpha")
	y2 := centerYOfVerb_bt2(t, w, "gamma")
	w.handleRequestDrop(src2, y2)
	if got := reqNames_bt2(w); equalStrs_bt2(got, before) {
		t.Fatalf("after clearing filter, drag should reorder; order unchanged: %v", got)
	}
}

// ---------------------------------------------------------------------------
// SPEC 5: no-op self-drop.
// ---------------------------------------------------------------------------

func TestBT2_SelfDropIsNoOp(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{
		{Name: "first", Method: model.MethodGet, URL: "http://x/1"},
		{Name: "second", Method: model.MethodGet, URL: "http://x/2"},
	}
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)
	layoutWindow_bt2(t, w)

	before := reqNames_bt2(w)
	w.dirty = false

	// Drop "first" onto the UPPER half of its own row (before itself, same folder).
	srcIdx := reqIdxByName_bt2(t, w, "first")
	row := findVerbRowFor(t, w, "first")
	pos := absPos(w.app.fyneApp, row)
	upperY := pos.Y + 2 // near the top → upper half → anchor before itself

	w.handleRequestDrop(srcIdx, upperY)

	if got := reqNames_bt2(w); !equalStrs_bt2(got, before) {
		t.Fatalf("self-drop must not reorder; before=%v after=%v", before, got)
	}
	if w.dirty {
		t.Fatal("self-drop must not mark the window dirty")
	}
}

// ---------------------------------------------------------------------------
// SPEC 6: no regression — right-click menu ops still work with drag present.
// ---------------------------------------------------------------------------

func TestBT2_RightClickMenuStillWorks(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{
		{Name: "keep", Method: model.MethodGet, URL: "http://x/k"},
		{Name: "dup", Method: model.MethodGet, URL: "http://x/d"},
		{Name: "gone", Method: model.MethodGet, URL: "http://x/g"},
	}
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)
	layoutWindow_bt2(t, w)

	// Duplicate "dup" (idx 1): count grows, a second "dup" appears, re-index intact.
	startCount := len(w.coll.Requests)
	w.duplicateRequest(reqIdxByName_bt2(t, w, "dup"))
	if len(w.coll.Requests) != startCount+1 {
		t.Fatalf("duplicate should add one request; count %d -> %d", startCount, len(w.coll.Requests))
	}

	// Delete "gone": count shrinks back, name removed, indices remain valid.
	w.deleteRequest(reqIdxByName_bt2(t, w, "gone"))
	if len(w.coll.Requests) != startCount {
		t.Fatalf("delete should remove one request; count = %d, want %d", len(w.coll.Requests), startCount)
	}
	for i := range w.coll.Requests {
		if w.coll.Requests[i].DisplayName() == "gone" {
			t.Fatalf("'gone' should be deleted; still present at %d", i)
		}
	}

	// Move-to-folder menu path: create a folder and move "keep" into it.
	fid := w.addFolder("box")
	w.moveRequestToFolder(reqIdxByName_bt2(t, w, "keep"), fid)
	ki := reqIdxByName_bt2(t, w, "keep")
	if w.coll.Requests[ki].FolderID != fid {
		t.Fatalf("move-to-folder should set FolderID=%q; got %q", fid, w.coll.Requests[ki].FolderID)
	}
}

func TestBT2_DeleteReindexesOpenTabsWithDragPresent(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{
		{Name: "r0", Method: model.MethodGet, URL: "http://x/0"},
		{Name: "r1", Method: model.MethodGet, URL: "http://x/1"},
		{Name: "r2", Method: model.MethodGet, URL: "http://x/2"},
	}
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)
	layoutWindow_bt2(t, w)

	w.openRequestTab(2) // open r2 at idx 2
	if _, ok := w.openTabs[2]; !ok {
		t.Fatal("precondition: r2 tab open at idx 2")
	}

	// Delete r0 → r2 shifts to idx 1; its tab must follow.
	w.deleteRequest(0)
	r2 := reqIdxByName_bt2(t, w, "r2")
	if r2 != 1 {
		t.Fatalf("r2 should be at idx 1 after deleting r0; got %d", r2)
	}
	if _, ok := w.openTabs[r2]; !ok {
		t.Fatalf("r2's tab must re-index to %d after delete; openTabs=%v", r2, keysOf(w.openTabs))
	}
}

// ---- small local helpers (unique names) ----

func equalStrs_bt2(a, b []string) bool {
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
