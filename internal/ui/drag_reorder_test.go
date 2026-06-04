package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/ultramcu/yon/internal/model"
)

// verbRow must remain fyne.Tappable (issue #13 / left-click open) AND additionally
// satisfy fyne.Draggable now that it carries drag-to-reorder. Both interfaces must
// coexist: the click-open path and the drag path are dispatched independently by
// the driver, so losing either breaks the sidebar.
func TestVerbRow_IsDraggableAndTappable(t *testing.T) {
	r := newVerbRow()
	if _, ok := interface{}(r).(fyne.Draggable); !ok {
		t.Fatal("verbRow must implement fyne.Draggable for drag-to-reorder")
	}
	if _, ok := interface{}(r).(fyne.Tappable); !ok {
		t.Fatal("verbRow must STILL implement fyne.Tappable (#13): a plain click opens the row")
	}
}

// Driving Dragged then DragEnd must call onDragEnd with the row's request id and
// the LAST absolute Y seen, once the gesture has actually moved.
func TestVerbRow_DraggedThenDragEndFiresOnDragEnd(t *testing.T) {
	r := newVerbRow()
	r.set(7, model.Request{Method: model.MethodGet, Name: "x"}, false)

	var gotID int
	var gotY float32
	fired := false
	r.onDragEnd = func(srcReqIdx int, dropAbsY float32) {
		gotID = srcReqIdx
		gotY = dropAbsY
		fired = true
	}

	// Move well beyond the deadzone: start at Y=10, end at Y=200.
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(0, 10)}})
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(0, 120)}})
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(0, 200)}})
	r.DragEnd()

	if !fired {
		t.Fatal("DragEnd after a real drag must call onDragEnd")
	}
	if gotID != 7 {
		t.Fatalf("onDragEnd got src id %d, want 7", gotID)
	}
	if gotY != 200 {
		t.Fatalf("onDragEnd got dropAbsY %v, want 200 (the last Y seen)", gotY)
	}

	// Drag state must be cleared so a following gesture starts fresh.
	if r.dragging {
		t.Fatal("DragEnd must clear the dragging flag")
	}
}

// A DragEnd with no movement (a plain tap never even fires Dragged) and a DragEnd
// with only negligible movement (click jitter) must NOT reorder: onDragEnd is not
// called below the deadzone.
func TestVerbRow_NegligibleDragDoesNotFire(t *testing.T) {
	r := newVerbRow()
	r.set(1, model.Request{Method: model.MethodGet, Name: "x"}, false)

	fired := false
	r.onDragEnd = func(int, float32) { fired = true }

	// No Dragged at all (a clean tap): DragEnd must do nothing.
	r.DragEnd()
	if fired {
		t.Fatal("DragEnd with no Dragged (a plain tap) must not fire onDragEnd")
	}

	// A 2px wobble is below dragDeadzone — treat it as click jitter, not a reorder.
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(0, 50)}})
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(0, 52)}})
	r.DragEnd()
	if fired {
		t.Fatal("a sub-deadzone wobble must not fire onDragEnd (a tiny accidental move is not a reorder)")
	}
}

// laidOutWindow builds a window from coll, lays it out at a real size, and forces
// the sidebar to render its rows so the drag hit-test reads non-zero geometry.
func laidOutWindow(t *testing.T, coll model.Collection) *Window {
	t.Helper()
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)
	w.win.Resize(fyne.NewSize(1100, 720))
	w.sidebar.Refresh()
	w.sidebar.Resize(w.sidebar.Size())
	return w
}

// rowCenterY returns the absolute centre Y of the visible row at index visibleRow,
// derived from the SAME public geometry handleRequestDrop uses (list top + scroll
// offset + padded row height), so dropping at this Y resolves back to visibleRow.
func rowCenterY(w *Window, visibleRow int) float32 {
	listTop := fyne.CurrentApp().Driver().AbsolutePositionForObject(w.sidebar).Y
	rowHeight := w.sidebarRowHeight()
	paddedHeight := rowHeight + theme.Padding()
	contentY := float32(visibleRow)*paddedHeight + rowHeight/2
	return listTop + contentY - w.sidebar.GetScrollOffset()
}

// dropAt synthesizes a real drag of the request at srcReqIdx ending over the
// centre of visible row destVisibleRow, exercising the full wiring (verbRow.
// Dragged/DragEnd -> handleRequestDrop -> dropTarget -> moveRequest).
func dropAt(t *testing.T, w *Window, srcReqIdx, destVisibleRow int) {
	t.Helper()
	row := findVerbRowForReqIdx(t, w, srcReqIdx)
	y := rowCenterY(w, destVisibleRow)
	row.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(0, y-100)}})
	row.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(0, y)}})
	row.DragEnd()
	w.sidebar.Refresh()
}

// findVerbRowForReqIdx returns the rendered *verbRow bound to flat request index
// reqIdx, failing the test if none is laid out.
func findVerbRowForReqIdx(t *testing.T, w *Window, reqIdx int) *verbRow {
	t.Helper()
	var match *verbRow
	walkObjects(w.app.fyneApp, w.win.Canvas().Content(), func(o fyne.CanvasObject) {
		if vr, ok := o.(*verbRow); ok && vr.Visible() && vr.id == reqIdx {
			match = vr
		}
	})
	if match == nil {
		t.Fatalf("no rendered verbRow is bound to request index %d", reqIdx)
	}
	return match
}

// TestHandleRequestDrop_ReorderWithinFolder drags a request down past a sibling in
// the same folder and asserts the flat order changed accordingly.
func TestHandleRequestDrop_ReorderWithinFolder(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Folders = []model.Folder{{ID: "f1", Name: "Folder"}}
	coll.Requests = []model.Request{
		{Name: "a", Method: model.MethodGet, FolderID: "f1"},
		{Name: "b", Method: model.MethodGet, FolderID: "f1"},
		{Name: "c", Method: model.MethodGet, FolderID: "f1"},
	}
	w := laidOutWindow(t, coll)

	// rows: [0]=folder header, [1]=a, [2]=b, [3]=c. Drag "a" (req 0) onto the LOWER
	// half region — drop at row 2 ("b") centre: upper half lands before b, so to put
	// a after b we drop a bit lower. Simpler: drop a onto row 3 ("c") centre (upper
	// half) -> lands before c, i.e. order becomes b, a, c.
	dropAt(t, w, 0, 3)

	got := []string{w.coll.Requests[0].Name, w.coll.Requests[1].Name, w.coll.Requests[2].Name}
	want := []string{"b", "a", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("after reorder within folder, flat order = %v, want %v", got, want)
		}
	}
	if w.coll.Requests[1].FolderID != "f1" {
		t.Fatalf("moved request should stay in f1, got %q", w.coll.Requests[1].FolderID)
	}
}

// TestHandleRequestDrop_TopLevelIntoFolder drags a top-level request onto a folder
// header, which reparents it into that folder.
func TestHandleRequestDrop_TopLevelIntoFolder(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Folders = []model.Folder{{ID: "f1", Name: "Folder"}}
	coll.Requests = []model.Request{
		{Name: "inFolder", Method: model.MethodGet, FolderID: "f1"},
		{Name: "loose", Method: model.MethodGet, FolderID: ""},
	}
	w := laidOutWindow(t, coll)

	// rows: [0]=folder header, [1]=inFolder, [2]=loose. Drop "loose" (req 1) onto the
	// folder header (row 0) -> into f1, at the top of the group.
	dropAt(t, w, 1, 0)

	var loose *model.Request
	for i := range w.coll.Requests {
		if w.coll.Requests[i].Name == "loose" {
			loose = &w.coll.Requests[i]
		}
	}
	if loose == nil {
		t.Fatal("loose request vanished after drop")
	}
	if loose.FolderID != "f1" {
		t.Fatalf("dropping onto the folder header should reparent into f1, got FolderID %q", loose.FolderID)
	}
}

// TestHandleRequestDrop_FolderRequestToTopLevel drags a request out of a folder to
// the top level by dropping it onto a top-level request row.
func TestHandleRequestDrop_FolderRequestToTopLevel(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Folders = []model.Folder{{ID: "f1", Name: "Folder"}}
	coll.Requests = []model.Request{
		{Name: "inFolder", Method: model.MethodGet, FolderID: "f1"},
		{Name: "top", Method: model.MethodGet, FolderID: ""},
	}
	w := laidOutWindow(t, coll)

	// rows: [0]=folder header, [1]=inFolder, [2]=top. Drop "inFolder" (req 0) onto
	// the top-level request row (row 2) -> top-level (FolderID "").
	dropAt(t, w, 0, 2)

	var moved *model.Request
	for i := range w.coll.Requests {
		if w.coll.Requests[i].Name == "inFolder" {
			moved = &w.coll.Requests[i]
		}
	}
	if moved == nil {
		t.Fatal("inFolder request vanished after drop")
	}
	if moved.FolderID != "" {
		t.Fatalf("dropping onto a top-level row should move to top level, got FolderID %q", moved.FolderID)
	}
}

// TestHandleRequestDrop_DisabledWhileFiltered confirms a filter blocks drag
// reorder (it would anchor against a partial view): the flat order is untouched.
func TestHandleRequestDrop_DisabledWhileFiltered(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{
		{Name: "alpha", Method: model.MethodGet},
		{Name: "beta", Method: model.MethodGet},
	}
	w := laidOutWindow(t, coll)
	w.filterQuery = "alpha"

	before := w.coll.Requests[0].Name
	w.handleRequestDrop(0, rowCenterY(w, 0))
	if w.coll.Requests[0].Name != before || w.dirty {
		t.Fatalf("drag reorder must be a no-op while a filter is active (order/dirty changed)")
	}
}
