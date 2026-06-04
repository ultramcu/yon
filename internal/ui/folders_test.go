package ui

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// addFolder appends a folder with a unique id and a trimmed name, and marks the
// window dirty.
func TestAddFolder_AppendsAndReturnsUniqueID(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))

	id := w.addFolder("  Auth  ")
	if id == "" {
		t.Fatal("addFolder returned an empty id")
	}
	if len(w.coll.Folders) != 1 {
		t.Fatalf("len(Folders) = %d, want 1", len(w.coll.Folders))
	}
	if got := w.coll.Folders[0].Name; got != "Auth" {
		t.Errorf("folder Name = %q, want %q (trimmed)", got, "Auth")
	}
	if w.coll.Folders[0].ID != id {
		t.Errorf("folder ID = %q, want returned id %q", w.coll.Folders[0].ID, id)
	}
	if !w.dirty {
		t.Error("addFolder did not mark the window dirty")
	}
}

// IDs from many rapid addFolder calls are all distinct.
func TestAddFolder_IDsAreUnique(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))
	seen := make(map[string]bool)
	const n = 500
	for i := 0; i < n; i++ {
		id := w.addFolder("f")
		if seen[id] {
			t.Fatalf("duplicate folder id %q on iteration %d", id, i)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d unique ids, want %d", len(seen), n)
	}
}

// renameFolder trims and sets the name, and marks dirty.
func TestRenameFolder_TrimsAndSets(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))
	id := w.addFolder("old")
	w.dirty = false

	w.renameFolder(id, "  new name  ")
	if got := w.coll.Folders[0].Name; got != "new name" {
		t.Errorf("Name = %q, want %q", got, "new name")
	}
	if !w.dirty {
		t.Error("renameFolder did not mark dirty")
	}

	// Unknown id is a no-op.
	w.renameFolder("nope", "x")
	if got := w.coll.Folders[0].Name; got != "new name" {
		t.Errorf("unknown-id rename mutated a folder: Name = %q", got)
	}
}

// deleteFolder removes the folder AND reparents its requests to top-level,
// keeping every request (count unchanged).
func TestDeleteFolder_ReparentsRequestsAndKeepsThem(t *testing.T) {
	coll := model.NewCollection("T")
	w := newScopeWindow(t, coll)
	fid := w.addFolder("grp")
	w.coll.Requests = []model.Request{
		{Method: model.MethodGet, Name: "a", FolderID: fid},
		{Method: model.MethodGet, Name: "b"}, // top-level
		{Method: model.MethodGet, Name: "c", FolderID: fid},
	}
	w.dirty = false

	w.deleteFolder(fid)

	if len(w.coll.Folders) != 0 {
		t.Errorf("len(Folders) = %d, want 0", len(w.coll.Folders))
	}
	if len(w.coll.Requests) != 3 {
		t.Fatalf("len(Requests) = %d, want 3 (no request may be lost)", len(w.coll.Requests))
	}
	for i, r := range w.coll.Requests {
		if r.FolderID != "" {
			t.Errorf("Requests[%d].FolderID = %q, want \"\" (reparented to top-level)", i, r.FolderID)
		}
	}
	if !w.dirty {
		t.Error("deleteFolder did not mark dirty")
	}
}

// moveRequestToFolder sets FolderID without reordering Requests; flat indices
// and contents are unchanged.
func TestMoveRequestToFolder_SetsFolderIDNoReorder(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))
	fid := w.addFolder("grp")
	w.coll.Requests = []model.Request{
		{Method: model.MethodGet, Name: "a"},
		{Method: model.MethodGet, Name: "b"},
		{Method: model.MethodGet, Name: "c"},
	}
	w.dirty = false

	w.moveRequestToFolder(1, fid)

	// Order/contents unchanged; only the target's FolderID changed.
	want := []string{"a", "b", "c"}
	for i, n := range want {
		if w.coll.Requests[i].Name != n {
			t.Errorf("Requests[%d].Name = %q, want %q (no reorder)", i, w.coll.Requests[i].Name, n)
		}
	}
	if w.coll.Requests[1].FolderID != fid {
		t.Errorf("Requests[1].FolderID = %q, want %q", w.coll.Requests[1].FolderID, fid)
	}
	if w.coll.Requests[0].FolderID != "" || w.coll.Requests[2].FolderID != "" {
		t.Error("moveRequestToFolder touched a non-target request's FolderID")
	}
	if !w.dirty {
		t.Error("moveRequestToFolder did not mark dirty")
	}

	// Move back to top-level.
	w.moveRequestToFolder(1, "")
	if w.coll.Requests[1].FolderID != "" {
		t.Errorf("move to \"\" did not clear FolderID, got %q", w.coll.Requests[1].FolderID)
	}
}

// moveRequestToFolder is a no-op for a bad index and for an unknown folder id.
func TestMoveRequestToFolder_GuardsBadInput(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))
	fid := w.addFolder("grp")
	w.coll.Requests = []model.Request{{Method: model.MethodGet, Name: "a", FolderID: fid}}
	w.dirty = false

	w.moveRequestToFolder(5, fid)  // out of range
	w.moveRequestToFolder(-1, fid) // out of range
	w.moveRequestToFolder(0, "ghost")
	if w.coll.Requests[0].FolderID != fid {
		t.Errorf("bad input mutated FolderID to %q", w.coll.Requests[0].FolderID)
	}
	if w.dirty {
		t.Error("a no-op move marked the window dirty")
	}
}

// toggleFolderCollapsed flips Collapsed and marks dirty.
func TestToggleFolderCollapsed_FlipsAndDirties(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))
	id := w.addFolder("grp")
	w.dirty = false

	w.toggleFolderCollapsed(id)
	if !w.coll.Folders[0].Collapsed {
		t.Error("toggle did not set Collapsed true")
	}
	if !w.dirty {
		t.Error("toggle did not mark dirty")
	}

	w.toggleFolderCollapsed(id)
	if w.coll.Folders[0].Collapsed {
		t.Error("second toggle did not set Collapsed false")
	}
}

// duplicateRequest copies FolderID, so a duplicate of a grouped request lands in
// the same folder (a struct copy already carries FolderID; this pins it).
func TestDuplicateRequest_CopiesFolderID(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))
	fid := w.addFolder("grp")
	w.coll.Requests = []model.Request{
		{Method: model.MethodGet, Name: "a", FolderID: fid},
	}

	w.duplicateRequest(0)

	if len(w.coll.Requests) != 2 {
		t.Fatalf("len(Requests) = %d, want 2", len(w.coll.Requests))
	}
	if w.coll.Requests[1].FolderID != fid {
		t.Errorf("duplicate FolderID = %q, want %q (same folder)", w.coll.Requests[1].FolderID, fid)
	}
}

// sidebarRows groups requests under their folders (collapsed folders show only a
// header) and lists top-level requests last, each request row carrying its flat
// Requests index.
func TestSidebarRows_GroupedOrder(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))
	// Folders in display order: f1 (open), f2 (collapsed).
	f1 := w.addFolder("one")
	f2 := w.addFolder("two")
	w.coll.Folders[1].Collapsed = true
	w.coll.Requests = []model.Request{
		{Method: model.MethodGet, Name: "top1"},              // idx 0, top-level
		{Method: model.MethodGet, Name: "f1a", FolderID: f1}, // idx 1
		{Method: model.MethodGet, Name: "f2a", FolderID: f2}, // idx 2 (collapsed → hidden)
		{Method: model.MethodGet, Name: "f1b", FolderID: f1}, // idx 3
		{Method: model.MethodGet, Name: "top2"},              // idx 4, top-level
	}

	rows := w.sidebarRows()

	type want struct {
		isFolder bool
		folder   string
		reqIdx   int
	}
	expected := []want{
		{isFolder: true, folder: f1}, // f1 header
		{reqIdx: 1, folder: f1},      // f1a
		{reqIdx: 3, folder: f1},      // f1b
		{isFolder: true, folder: f2}, // f2 header (collapsed → no children)
		{reqIdx: 0},                  // top1
		{reqIdx: 4},                  // top2
	}

	if len(rows) != len(expected) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(expected), rows)
	}
	for i, e := range expected {
		r := rows[i]
		if r.IsFolder != e.isFolder {
			t.Errorf("row %d IsFolder = %v, want %v", i, r.IsFolder, e.isFolder)
		}
		if r.FolderID != e.folder {
			t.Errorf("row %d FolderID = %q, want %q", i, r.FolderID, e.folder)
		}
		if !e.isFolder && r.ReqIdx != e.reqIdx {
			t.Errorf("row %d ReqIdx = %d, want %d", i, r.ReqIdx, e.reqIdx)
		}
		if e.isFolder && r.ReqIdx != -1 {
			t.Errorf("folder row %d ReqIdx = %d, want -1", i, r.ReqIdx)
		}
	}
}
