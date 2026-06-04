package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/ultramcu/yon/internal/model"
)

// layoutWindow resizes + refreshes the window so the sidebar List lays out its
// rows (rendered verbRow/folderRow widgets become discoverable by walkObjects).
func layoutWindow(t *testing.T, w *Window) {
	t.Helper()
	w.win.Resize(fyne.NewSize(1100, 720))
	w.sidebar.Refresh()
	w.sidebar.Resize(w.sidebar.Size())
}

// findFolderRowFor returns the rendered *folderRow showing the given name, or nil.
func findFolderRowFor(w *Window, name string) *folderRow {
	var match *folderRow
	walkObjects(w.app.fyneApp, w.win.Canvas().Content(), func(o fyne.CanvasObject) {
		if fr, ok := o.(*folderRow); ok && fr.Visible() && fr.name != nil && fr.name.Text == name {
			match = fr
		}
	})
	return match
}

// findVisibleVerbRowFor returns the rendered, VISIBLE *verbRow showing name, or
// nil. Unlike findVerbRowFor it skips hidden (morphed-away) rows so a request
// under a collapsed folder is reported as absent.
func findVisibleVerbRowFor(w *Window, name string) *verbRow {
	var match *verbRow
	walkObjects(w.app.fyneApp, w.win.Canvas().Content(), func(o fyne.CanvasObject) {
		if vr, ok := o.(*verbRow); ok && vr.Visible() && vr.name != nil && vr.name.Text == name {
			match = vr
		}
	})
	return match
}

// The List length tracks len(sidebarRows()): folder headers + visible requests.
func TestSidebar_ListLengthMatchesRows(t *testing.T) {
	coll := model.NewCollection("C")
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)

	fid := w.addFolder("grp")
	w.coll.Requests = []model.Request{
		{Method: model.MethodGet, Name: "a", FolderID: fid},
		{Method: model.MethodGet, Name: "b"}, // top-level
	}
	w.refreshSidebar()

	// 1 folder header + 1 grouped req + 1 top-level req = 3 rows.
	if got, want := w.sidebar.Length(), len(w.sidebarRows()); got != want {
		t.Fatalf("List length = %d, want len(sidebarRows()) = %d", got, want)
	}
	if w.sidebar.Length() != 3 {
		t.Fatalf("List length = %d, want 3", w.sidebar.Length())
	}
}

// A folder header renders its name + a chevron reflecting Collapsed, and a
// collapsed folder hides its request rows (length shrinks).
func TestSidebar_CollapsedFolderHidesRequests(t *testing.T) {
	coll := model.NewCollection("C")
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)

	fid := w.addFolder("Auth")
	w.coll.Requests = []model.Request{
		{Method: model.MethodGet, Name: "login", FolderID: fid},
	}
	w.refreshSidebar()
	layoutWindow(t, w)

	// Expanded: header + request both render; chevron points down.
	if findFolderRowFor(w, "Auth") == nil {
		t.Fatal("folder header 'Auth' not rendered when expanded")
	}
	if fr := findFolderRowFor(w, "Auth"); fr.chevron.Resource.Name() != theme.MenuDropDownIcon().Name() {
		t.Errorf("expanded chevron = %s, want MenuDropDown", fr.chevron.Resource.Name())
	}
	if findVisibleVerbRowFor(w, "login") == nil {
		t.Fatal("request 'login' should be visible while folder expanded")
	}
	expanded := w.sidebar.Length()

	// Collapse and re-render: the request row disappears; the header stays with a
	// right-pointing chevron.
	w.toggleFolder(fid)
	layoutWindow(t, w)

	if got := w.sidebar.Length(); got != expanded-1 {
		t.Fatalf("collapsed length = %d, want %d (one request hidden)", got, expanded-1)
	}
	if findVisibleVerbRowFor(w, "login") != nil {
		t.Error("request 'login' should be hidden under a collapsed folder")
	}
	if fr := findFolderRowFor(w, "Auth"); fr == nil {
		t.Fatal("folder header should remain when collapsed")
	} else if fr.chevron.Resource.Name() != theme.MenuExpandIcon().Name() {
		t.Errorf("collapsed chevron = %s, want MenuExpand", fr.chevron.Resource.Name())
	}
}

// Clicking a folder header (folderRow.Tapped) toggles collapse and updates the
// row set.
func TestSidebar_FolderHeaderTapTogglesCollapse(t *testing.T) {
	coll := model.NewCollection("C")
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)

	fid := w.addFolder("grp")
	w.coll.Requests = []model.Request{{Method: model.MethodGet, Name: "r", FolderID: fid}}
	w.refreshSidebar()
	layoutWindow(t, w)

	fr := findFolderRowFor(w, "grp")
	if fr == nil {
		t.Fatal("folder header not rendered")
	}
	fr.Tapped(&fyne.PointEvent{})

	if !w.coll.Folders[0].Collapsed {
		t.Fatal("tapping the header should collapse the folder")
	}
	layoutWindow(t, w)
	if findVisibleVerbRowFor(w, "r") != nil {
		t.Error("request should be hidden after the header tap collapsed the folder")
	}

	// Tap again to expand.
	if fr := findFolderRowFor(w, "grp"); fr != nil {
		fr.Tapped(&fyne.PointEvent{})
	}
	if w.coll.Folders[0].Collapsed {
		t.Fatal("a second header tap should expand the folder")
	}
}

// A request row inside a folder still opens on left-click (tap routing through
// selectByReqIdx) and shows the cyan accent when selected.
func TestSidebar_GroupedRequestOpensAndAccents(t *testing.T) {
	coll := model.NewCollection("C")
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)

	fid := w.addFolder("grp")
	w.coll.Requests = []model.Request{
		{Method: model.MethodGet, Name: "top"},
		{Method: model.MethodPost, Name: "inside", FolderID: fid},
	}
	w.refreshSidebar()
	layoutWindow(t, w)

	vr := findVisibleVerbRowFor(w, "inside")
	if vr == nil {
		t.Fatal("grouped request row 'inside' not rendered")
	}
	// Left-click routes verbRow.Tapped -> onTap(reqIdx) -> selectByReqIdx -> open.
	vr.Tapped(&fyne.PointEvent{})

	if len(w.openTabs) != 1 {
		t.Fatalf("left-clicking the grouped request should open one tab, got %d", len(w.openTabs))
	}
	if _, ok := w.openTabs[1]; !ok {
		t.Fatalf("opened tab should be flat request index 1, openTabs=%v", keysOf(w.openTabs))
	}
	if w.selectedID != 1 {
		t.Fatalf("selectedID should be the flat index 1, got %d", w.selectedID)
	}
	// The accent paints when the row's ReqIdx == selectedID (re-render to apply).
	layoutWindow(t, w)
	if vr := findVisibleVerbRowFor(w, "inside"); vr == nil || vr.accent.FillColor != verbRowAccent {
		t.Error("selected grouped request should show the cyan accent")
	}
}

// "Move to folder…" moves a request: its FolderID changes and it renders under
// the new folder.
func TestSidebar_MoveRequestToFolder(t *testing.T) {
	coll := model.NewCollection("C")
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)

	fid := w.addFolder("dest")
	w.coll.Requests = []model.Request{{Method: model.MethodGet, Name: "movable"}} // top-level
	w.refreshSidebar()

	// Build the submenu the right-click would show and invoke the "dest" item.
	menu := w.moveToFolderMenu(0)
	var destItem *fyne.MenuItem
	for _, it := range menu.Items {
		if it.Label == "dest" {
			destItem = it
		}
	}
	if destItem == nil {
		t.Fatal("move submenu missing the 'dest' folder item")
	}
	destItem.Action()

	if w.coll.Requests[0].FolderID != fid {
		t.Fatalf("FolderID = %q after move, want %q", w.coll.Requests[0].FolderID, fid)
	}
	// It now renders under the folder: the grouped row exists in the new order.
	rows := w.sidebarRows()
	foundUnder := false
	for _, r := range rows {
		if !r.IsFolder && r.ReqIdx == 0 && r.FolderID == fid {
			foundUnder = true
		}
	}
	if !foundUnder {
		t.Fatalf("moved request not grouped under folder %q: %+v", fid, rows)
	}
}

// Creating a folder adds a header row; renaming updates it; deleting removes the
// header and reparents its requests to top level.
func TestSidebar_FolderLifecycle(t *testing.T) {
	coll := model.NewCollection("C")
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)

	// Create.
	fid := w.addFolder("Original")
	w.coll.Requests = []model.Request{{Method: model.MethodGet, Name: "child", FolderID: fid}}
	w.refreshSidebar()
	layoutWindow(t, w)
	if findFolderRowFor(w, "Original") == nil {
		t.Fatal("created folder header not rendered")
	}

	// Rename (directly via the model + refresh, as the dialog confirm does).
	w.renameFolder(fid, "Renamed")
	w.refreshSidebar()
	layoutWindow(t, w)
	if findFolderRowFor(w, "Renamed") == nil {
		t.Fatal("renamed folder header not rendered")
	}
	if findFolderRowFor(w, "Original") != nil {
		t.Error("old folder name should no longer render")
	}

	// Delete: header gone, request survives at top level.
	w.deleteFolder(fid)
	w.refreshSidebar()
	layoutWindow(t, w)
	if findFolderRowFor(w, "Renamed") != nil {
		t.Error("deleted folder header should be gone")
	}
	if w.coll.Requests[0].FolderID != "" {
		t.Errorf("deleted folder's request should reparent to top level, FolderID = %q", w.coll.Requests[0].FolderID)
	}
	if findVisibleVerbRowFor(w, "child") == nil {
		t.Error("the reparented request should still render at top level")
	}
}

// folderRow must implement fyne.Tappable so its primary tap (toggle) isn't
// swallowed by the glfw tap router — the same property #13 required of verbRow.
func TestFolderRow_IsTappable(t *testing.T) {
	if _, ok := interface{}(newFolderRow()).(fyne.Tappable); !ok {
		t.Fatal("folderRow must implement fyne.Tappable so the header toggle fires")
	}
}
