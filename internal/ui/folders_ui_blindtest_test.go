package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// ---------------------------------------------------------------------------
// Independent BLIND tests for the grouped-folder SIDEBAR UI + no-regression.
// Written from the SPEC only; does not read the other authors' test files.
// All helper names are prefixed fuib (folders-ui-blind) to avoid collisions.
// ---------------------------------------------------------------------------

// fuibColl returns a fresh collection with n top-level requests named R0..R(n-1).
func fuibColl(n int) model.Collection {
	c := model.NewCollection("C")
	for i := 0; i < n; i++ {
		c.Requests = append(c.Requests, model.Request{
			Name:   "R" + string(rune('0'+i)),
			Method: model.MethodGet,
			URL:    "http://localhost/r",
			Auth:   model.Auth{Kind: model.AuthInherit},
			Body:   model.Body{Type: model.BodyNone},
		})
	}
	return c
}

// fuibWin builds a real Window over coll via the package test helper.
func fuibWin(coll model.Collection) *Window {
	return newTestWindow(coll)
}

// fuibReqIdxForRow returns the flat ReqIdx of the first request row whose
// owning folder is folderID. -1 if none.
func fuibReqRowIdx(w *Window, folderID string) int {
	for _, r := range w.sidebarRows() {
		if !r.IsFolder && r.FolderID == folderID {
			return r.ReqIdx
		}
	}
	return -1
}

// fuibFolderHeaderCount counts folder-header rows in the visible row set.
func fuibFolderHeaderCount(w *Window) int {
	n := 0
	for _, r := range w.sidebarRows() {
		if r.IsFolder {
			n++
		}
	}
	return n
}

// fuibRowsForFolder returns the request ReqIdx slice grouped under folderID, in
// visible-row order.
func fuibRowsForFolder(w *Window, folderID string) []int {
	var out []int
	for _, r := range w.sidebarRows() {
		if !r.IsFolder && r.FolderID == folderID {
			out = append(out, r.ReqIdx)
		}
	}
	return out
}

// ---- SPEC 1: List length tracks sidebarRows -------------------------------

func TestFUIB_ListLengthTracksSidebarRows(t *testing.T) {
	w := fuibWin(fuibColl(4))

	// Two folders, move two requests into the first.
	f1 := w.addFolder("F1")
	_ = w.addFolder("F2")
	w.moveRequestToFolder(0, f1)
	w.moveRequestToFolder(1, f1)
	w.refreshSidebar()

	if got, want := w.sidebar.Length(), len(w.sidebarRows()); got != want {
		t.Fatalf("SPEC1: sidebar.Length()=%d, want len(sidebarRows())=%d", got, want)
	}

	before := w.sidebar.Length()
	rowsUnder := len(fuibRowsForFolder(w, f1)) // should be 2
	if rowsUnder != 2 {
		t.Fatalf("SPEC1 setup: expected 2 requests under F1, got %d", rowsUnder)
	}

	// Collapse F1: visible length must shrink by exactly that folder's req count.
	w.toggleFolder(f1)
	after := w.sidebar.Length()
	if after != before-rowsUnder {
		t.Fatalf("SPEC1: after collapse length=%d, want %d (before %d - %d)", after, before-rowsUnder, before, rowsUnder)
	}
	if w.sidebar.Length() != len(w.sidebarRows()) {
		t.Fatalf("SPEC1: post-collapse length=%d != len(rows)=%d", w.sidebar.Length(), len(w.sidebarRows()))
	}
	t.Logf("SPEC1 PASS: length tracks rows; collapse shrank %d -> %d (-%d)", before, after, rowsUnder)
}

// ---- SPEC 2: Grouping / order on screen -----------------------------------

func TestFUIB_GroupingAndOrder(t *testing.T) {
	w := fuibWin(fuibColl(4)) // R0..R3

	f1 := w.addFolder("F1")
	f2 := w.addFolder("F2")
	// R1 -> F1, R3 -> F2; R0,R2 stay top-level.
	w.moveRequestToFolder(1, f1)
	w.moveRequestToFolder(3, f2)
	w.refreshSidebar()

	rows := w.sidebarRows()
	// Expected order: [F1 header][R1][F2 header][R3][R0][R2]
	if len(rows) != 6 {
		t.Fatalf("SPEC2: expected 6 rows, got %d (%+v)", len(rows), rows)
	}
	check := func(i int, isFolder bool, folderID string, reqIdx int) {
		r := rows[i]
		if r.IsFolder != isFolder || r.FolderID != folderID || (!isFolder && r.ReqIdx != reqIdx) {
			t.Fatalf("SPEC2: rows[%d]=%+v, want IsFolder=%v FolderID=%q ReqIdx=%d", i, r, isFolder, folderID, reqIdx)
		}
	}
	check(0, true, f1, -1)
	check(1, false, f1, 1)
	check(2, true, f2, -1)
	check(3, false, f2, 3)
	check(4, false, "", 0)
	check(5, false, "", 2)

	// Collapsed folder contributes only its header.
	w.toggleFolder(f1)
	rows = w.sidebarRows()
	// Now: [F1 header][F2 header][R3][R0][R2]
	if rows[0].FolderID != f1 || !rows[0].IsFolder {
		t.Fatalf("SPEC2 collapsed: rows[0]=%+v, want F1 header", rows[0])
	}
	for _, r := range rows {
		if !r.IsFolder && r.FolderID == f1 {
			t.Fatalf("SPEC2 collapsed: F1 still contributes request row %+v", r)
		}
	}
	if len(rows) != 5 {
		t.Fatalf("SPEC2 collapsed: expected 5 rows, got %d", len(rows))
	}
	t.Logf("SPEC2 PASS: folders-then-toplevel order correct; collapsed folder = header only")
}

// ---- SPEC 3: Collapse toggle ----------------------------------------------

func TestFUIB_CollapseToggleFlipsAndCounts(t *testing.T) {
	w := fuibWin(fuibColl(3))

	f1 := w.addFolder("F1")
	w.moveRequestToFolder(0, f1)
	w.moveRequestToFolder(1, f1)
	w.refreshSidebar()

	f, ok := w.folderByID(f1)
	if !ok {
		t.Fatalf("SPEC3: folder not found")
	}
	if f.Collapsed {
		t.Fatalf("SPEC3: new folder should start expanded")
	}

	// Count badge / folderRequestCount is sensible regardless of collapse state.
	if got := w.folderRequestCount(f1); got != 2 {
		t.Fatalf("SPEC3: folderRequestCount=%d, want 2", got)
	}

	w.toggleFolder(f1)
	if !f.Collapsed {
		t.Fatalf("SPEC3: toggle did not set Collapsed")
	}
	// Count is independent of collapse (still 2 requests belong to the folder).
	if got := w.folderRequestCount(f1); got != 2 {
		t.Fatalf("SPEC3: folderRequestCount after collapse=%d, want 2", got)
	}

	w.toggleFolder(f1)
	if f.Collapsed {
		t.Fatalf("SPEC3: second toggle did not clear Collapsed")
	}
	t.Logf("SPEC3 PASS: toggle flips Collapsed both ways; count badge stays %d", 2)
}

// ---- SPEC 4: Request open still works in a folder (no #13 regression) ------

func TestFUIB_RequestInFolderOpensCorrectByName(t *testing.T) {
	w := fuibWin(fuibColl(3)) // R0,R1,R2

	f1 := w.addFolder("F1")
	// Put R2 (flat idx 2) under the folder.
	w.moveRequestToFolder(2, f1)
	w.refreshSidebar()

	reqIdx := fuibReqRowIdx(w, f1)
	if reqIdx != 2 {
		t.Fatalf("SPEC4: expected R2 (flat idx 2) under folder, got reqIdx=%d", reqIdx)
	}

	// Drive the documented open path used by the verbRow onTap handler.
	w.selectByReqIdx(reqIdx)

	rt, ok := w.openTabs[reqIdx]
	if !ok {
		t.Fatalf("SPEC4 REGRESSION: request in folder did not open a tab (openTabs=%v)", w.openTabs)
	}
	if rt.idx != reqIdx {
		t.Fatalf("SPEC4: opened tab idx=%d, want %d", rt.idx, reqIdx)
	}
	// Correct request by NAME.
	if w.coll.Requests[reqIdx].Name != "R2" {
		t.Fatalf("SPEC4: opened wrong request, name=%q want R2", w.coll.Requests[reqIdx].Name)
	}
	// selection is the FLAT index, not the visible-row position.
	if w.selectedID != reqIdx {
		t.Fatalf("SPEC4: selectedID=%d, want flat idx %d", w.selectedID, reqIdx)
	}

	// Accent reflects selection by FLAT index. Render the request row via setRequest
	// (same path UpdateItem uses) and confirm the selected row paints cyan while a
	// non-selected one does not.
	selItem := newSidebarItem()
	selItem.setRequest(reqIdx, w.coll.Requests[reqIdx], reqIdx == w.selectedID)
	if c, _ := selItem.req.accent.FillColor.(color.NRGBA); c != verbRowAccent {
		t.Fatalf("SPEC4: selected row accent=%v, want cyan %v", selItem.req.accent.FillColor, verbRowAccent)
	}
	other := 0 // R0 is not selected
	otherItem := newSidebarItem()
	otherItem.setRequest(other, w.coll.Requests[other], other == w.selectedID)
	if otherItem.req.accent.FillColor != color.Transparent {
		t.Fatalf("SPEC4: non-selected row accent=%v, want transparent", otherItem.req.accent.FillColor)
	}
	t.Logf("SPEC4 PASS: folder request R2 opened (flat idx %d), selectedID=flat, accent by flat index", reqIdx)
}

// fuibVerbRowTap exercises the genuine verbRow.Tapped routing (fyne.Tappable):
// it builds a sidebarItem wired exactly as buildSidebar wires it and taps.
func TestFUIB_VerbRowTapRoutingOpensFolderRequest(t *testing.T) {
	w := fuibWin(fuibColl(2)) // R0,R1

	f1 := w.addFolder("F1")
	w.moveRequestToFolder(1, f1)
	w.refreshSidebar()

	reqIdx := fuibReqRowIdx(w, f1)
	if reqIdx != 1 {
		t.Fatalf("SPEC4-tap: expected R1 under folder, got %d", reqIdx)
	}

	// Build a request row morphed to the folder request, wired like buildSidebar.
	it := newSidebarItem()
	it.req.onTap = func(idx int) { w.selectByReqIdx(idx) }
	it.setRequest(reqIdx, w.coll.Requests[reqIdx], false)

	// A primary tap must route through onTap -> selectByReqIdx -> open.
	test.Tap(it.req)

	if _, ok := w.openTabs[reqIdx]; !ok {
		t.Fatalf("SPEC4-tap REGRESSION (#13): verbRow.Tapped did not open the folder request; openTabs=%v", w.openTabs)
	}
	if w.selectedID != reqIdx {
		t.Fatalf("SPEC4-tap: selectedID=%d, want %d", w.selectedID, reqIdx)
	}
	t.Logf("SPEC4-tap PASS: verbRow primary tap opens folder request via onTap routing")
}

// ---- SPEC 5: Move to folder ------------------------------------------------

func TestFUIB_MoveRequestToFolderAndBack(t *testing.T) {
	w := fuibWin(fuibColl(2)) // R0,R1

	f1 := w.addFolder("F1")
	w.refreshSidebar()

	// Initially both top-level.
	if got := fuibRowsForFolder(w, ""); len(got) != 2 {
		t.Fatalf("SPEC5: expected 2 top-level rows initially, got %v", got)
	}

	// Move R0 into F1.
	w.moveRequestToFolder(0, f1)
	w.refreshSidebar()

	under := fuibRowsForFolder(w, f1)
	if len(under) != 1 || under[0] != 0 {
		t.Fatalf("SPEC5: after move R0 should be under F1, got %v", under)
	}
	if w.coll.Requests[0].FolderID != f1 {
		t.Fatalf("SPEC5: request FolderID=%q, want %q", w.coll.Requests[0].FolderID, f1)
	}
	// Its row's grouping changed: R0 no longer at top level.
	for _, idx := range fuibRowsForFolder(w, "") {
		if idx == 0 {
			t.Fatalf("SPEC5: R0 still appears at top level after move")
		}
	}

	// Move back to "" (top level).
	w.moveRequestToFolder(0, "")
	w.refreshSidebar()
	if w.coll.Requests[0].FolderID != "" {
		t.Fatalf("SPEC5: after move-back FolderID=%q, want empty", w.coll.Requests[0].FolderID)
	}
	top := fuibRowsForFolder(w, "")
	found := false
	for _, idx := range top {
		if idx == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("SPEC5: R0 not back at top level, top=%v", top)
	}
	if len(fuibRowsForFolder(w, f1)) != 0 {
		t.Fatalf("SPEC5: F1 should be empty after move-back")
	}
	t.Logf("SPEC5 PASS: move into folder and back to top level reflected in rows")
}

// ---- SPEC 6: Folder create / rename / delete via ops ----------------------

func TestFUIB_FolderCreateRenameDelete(t *testing.T) {
	w := fuibWin(fuibColl(2)) // R0,R1

	// create: addFolder adds a header row.
	before := fuibFolderHeaderCount(w)
	f1 := w.addFolder("Alpha")
	w.refreshSidebar()
	if got := fuibFolderHeaderCount(w); got != before+1 {
		t.Fatalf("SPEC6: header count after add=%d, want %d", got, before+1)
	}

	// rename: changes the displayed name (folder model + header render).
	w.renameFolder(f1, "Beta")
	w.refreshSidebar()
	f, ok := w.folderByID(f1)
	if !ok {
		t.Fatalf("SPEC6: folder missing after rename")
	}
	if f.Name != "Beta" {
		t.Fatalf("SPEC6: rename failed, name=%q want Beta", f.Name)
	}
	// Confirm the header render shows the new name.
	hdr := newFolderRow()
	hdr.set(*f, w.folderRequestCount(f1))
	if hdr.name.Text != "Beta" {
		t.Fatalf("SPEC6: folder header label=%q, want Beta", hdr.name.Text)
	}

	// Put both requests in the folder, then delete: requests reappear at top level.
	w.moveRequestToFolder(0, f1)
	w.moveRequestToFolder(1, f1)
	w.refreshSidebar()
	if len(fuibRowsForFolder(w, f1)) != 2 {
		t.Fatalf("SPEC6: expected 2 requests under folder before delete")
	}

	w.deleteFolder(f1)
	w.refreshSidebar()
	if _, ok := w.folderByID(f1); ok {
		t.Fatalf("SPEC6: folder still present after delete")
	}
	if got := fuibFolderHeaderCount(w); got != before {
		t.Fatalf("SPEC6: header count after delete=%d, want %d", got, before)
	}
	top := fuibRowsForFolder(w, "")
	if len(top) != 2 {
		t.Fatalf("SPEC6: after folder delete, expected 2 top-level requests, got %v", top)
	}
	for i := 0; i < 2; i++ {
		if w.coll.Requests[i].FolderID != "" {
			t.Fatalf("SPEC6: request %d FolderID=%q, want empty after folder delete", i, w.coll.Requests[i].FolderID)
		}
	}

	// Reparented requests are still openable.
	w.selectByReqIdx(0)
	if _, ok := w.openTabs[0]; !ok {
		t.Fatalf("SPEC6: reparented request not openable")
	}
	t.Logf("SPEC6 PASS: create/rename/delete via ops; requests reparent to top level and stay openable")
}

// ---- SPEC 7: No regressions (delete / duplicate with folders present) ------

func TestFUIB_DeleteRequestWithFoldersReindexes(t *testing.T) {
	w := fuibWin(fuibColl(4)) // R0,R1,R2,R3

	f1 := w.addFolder("F1")
	w.moveRequestToFolder(1, f1) // R1 in folder
	w.moveRequestToFolder(2, f1) // R2 in folder
	w.refreshSidebar()

	// Open R2 (flat idx 2) and R3 (flat idx 3).
	w.selectByReqIdx(2)
	w.openRequestTab(3)
	if _, ok := w.openTabs[2]; !ok {
		t.Fatalf("SPEC7-del setup: R2 tab not open")
	}
	if _, ok := w.openTabs[3]; !ok {
		t.Fatalf("SPEC7-del setup: R3 tab not open")
	}

	// Delete R1 (flat idx 1). R2->idx1, R3->idx2; open tabs must re-index.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("SPEC7-del REGRESSION: deleteRequest panicked: %v", r)
			}
		}()
		w.deleteRequest(1)
	}()
	w.refreshSidebar()

	if len(w.coll.Requests) != 3 {
		t.Fatalf("SPEC7-del: expected 3 requests after delete, got %d", len(w.coll.Requests))
	}
	// R2 is now flat idx 1, still in folder.
	if w.coll.Requests[1].Name != "R2" || w.coll.Requests[1].FolderID != f1 {
		t.Fatalf("SPEC7-del: idx1 should be R2 in F1, got name=%q folder=%q", w.coll.Requests[1].Name, w.coll.Requests[1].FolderID)
	}
	// The tab that was open for R2 must now be keyed at idx 1, and its rt.idx synced.
	rt, ok := w.openTabs[1]
	if !ok {
		t.Fatalf("SPEC7-del REGRESSION: R2 tab lost after re-index; openTabs=%v", w.openTabs)
	}
	if rt.idx != 1 {
		t.Fatalf("SPEC7-del: R2 tab rt.idx=%d, want 1", rt.idx)
	}
	// R3 tab moved from key 3 -> 2.
	if rt3, ok := w.openTabs[2]; !ok || rt3.idx != 2 {
		t.Fatalf("SPEC7-del REGRESSION: R3 tab not re-keyed to 2 (ok=%v)", ok)
	}
	// No stale key at old index 3.
	if _, ok := w.openTabs[3]; ok {
		t.Fatalf("SPEC7-del: stale openTabs key 3 remains")
	}
	t.Logf("SPEC7-del PASS: deleteRequest re-indexed open tabs with folders present, no panic")
}

func TestFUIB_DuplicateRequestLandsInSameFolder(t *testing.T) {
	w := fuibWin(fuibColl(3)) // R0,R1,R2

	f1 := w.addFolder("F1")
	w.moveRequestToFolder(1, f1) // R1 in folder
	w.refreshSidebar()

	var dupPanic interface{}
	func() {
		defer func() { dupPanic = recover() }()
		w.duplicateRequest(1) // duplicate the folder request
	}()
	if dupPanic != nil {
		t.Fatalf("SPEC7-dup REGRESSION: duplicateRequest panicked: %v", dupPanic)
	}
	w.refreshSidebar()

	if len(w.coll.Requests) != 4 {
		t.Fatalf("SPEC7-dup: expected 4 requests, got %d", len(w.coll.Requests))
	}
	// Copy inserted at idx 2, inherits the same folder.
	dup := w.coll.Requests[2]
	if dup.FolderID != f1 {
		t.Fatalf("SPEC7-dup REGRESSION: duplicate FolderID=%q, want %q (same folder)", dup.FolderID, f1)
	}
	if dup.Name != "R1 copy" {
		t.Fatalf("SPEC7-dup: duplicate name=%q, want \"R1 copy\"", dup.Name)
	}
	// Both original and copy show under the folder, in order.
	under := fuibRowsForFolder(w, f1)
	if len(under) != 2 || under[0] != 1 || under[1] != 2 {
		t.Fatalf("SPEC7-dup: folder rows=%v, want [1 2]", under)
	}
	// Selection lands on the copy (flat idx 2).
	if w.selectedID != 2 {
		t.Fatalf("SPEC7-dup: selectedID=%d, want 2 (the copy)", w.selectedID)
	}
	t.Logf("SPEC7-dup PASS: duplicate lands in same folder, named %q, selected", dup.Name)
}

// fuibTapNoPanic: existing tap-routing behavior intact — left-click on a folder
// header toggles collapse (does not panic, does not open a tab), and a top-level
// request row still opens.
func TestFUIB_TapRoutingIntactNoPanic(t *testing.T) {
	w := fuibWin(fuibColl(2)) // R0,R1

	f1 := w.addFolder("F1")
	w.moveRequestToFolder(0, f1)
	w.refreshSidebar()

	// Folder header tap toggles collapse, opens no tab.
	hdr := newSidebarItem()
	hdr.folder.onToggle = w.toggleFolder
	f, _ := w.folderByID(f1)
	hdr.setFolder(*f, w.folderRequestCount(f1))

	wasCollapsed := f.Collapsed
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("SPEC7-tap REGRESSION: folder header tap panicked: %v", r)
			}
		}()
		test.Tap(hdr.folder)
	}()
	if f.Collapsed == wasCollapsed {
		t.Fatalf("SPEC7-tap: folder header tap did not toggle collapse")
	}
	if len(w.openTabs) != 0 {
		t.Fatalf("SPEC7-tap: folder header tap opened a tab (openTabs=%v)", w.openTabs)
	}

	// Top-level request row still opens via left-click routing.
	reqIdx := fuibReqRowIdx(w, "")
	if reqIdx < 0 {
		t.Fatalf("SPEC7-tap setup: no top-level request")
	}
	row := newSidebarItem()
	row.req.onTap = func(idx int) { w.selectByReqIdx(idx) }
	row.setRequest(reqIdx, w.coll.Requests[reqIdx], false)
	test.Tap(row.req)
	if _, ok := w.openTabs[reqIdx]; !ok {
		t.Fatalf("SPEC7-tap REGRESSION: top-level request did not open on tap")
	}
	t.Logf("SPEC7-tap PASS: header tap toggles (no tab), request tap opens; no panic")
}
