package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// TestSidebarTapRouting_PrimaryTapOpensRequest is a routing-level regression
// guard for issue #13: a left-click on a sidebar request row must open that
// request as a tab.
//
// Why this test exists (and why TestVerbRow_IsTappable is not enough):
//
// Fyne v2.7.4's glfw driver routes a desktop click in two steps
// (internal/driver/util.go FindObjectAtPositionMatching +
// internal/driver/glfw/window.go processMouseClicked / mouseClickedHandleTapDoubleTap):
//
//  1. It walks the laid-out canvas object tree TOP-DOWN and keeps the DEEPEST
//     object under the cursor matching ANY of the "broad tappable" interfaces:
//     fyne.Tappable | fyne.SecondaryTappable | fyne.DoubleTappable |
//     fyne.Focusable | desktop.Mouseable.
//  2. For a primary (left) release it dispatches Tapped ONLY if that deepest
//     object actually implements fyne.Tappable.
//
// Before #13, verbRow implemented SecondaryTappable (for right-click Delete) but
// NOT fyne.Tappable. Over a sidebar row the deepest broad match is therefore the
// *verbRow, which shadows the enclosing widget.List's own listItem selection —
// but verbRow had no Tapped, so the primary tap was swallowed and no tab opened.
// After #13, verbRow.Tapped -> onTap(id) -> sidebar.Select(id) -> openRequestTab.
//
// The fyne test package's Tap/TapAt CANNOT reproduce this: its position-based
// findTappable matches ONLY fyne.Tappable, so it resolves to the listItem and
// opens the tab even with the bug present. To guard the real behaviour we must
// mirror the DRIVER's broad predicate ourselves, which this test does in
// findBroadTappableAt below.
func TestSidebarTapRouting_PrimaryTapOpensRequest(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{
		{Name: "alpha", Method: model.MethodGet, URL: "http://x/alpha"},
		{Name: "bravo", Method: model.MethodPost, URL: "http://x/bravo"},
	}
	w := newTestWindow(coll)
	t.Cleanup(w.win.Close)

	// Lay the window out at a real size so the sidebar rows get non-zero
	// positions/sizes (a zero-size tree would make the geometry walk meaningless).
	w.win.Resize(fyne.NewSize(1100, 720))
	w.sidebar.Refresh()
	w.sidebar.Resize(w.sidebar.Size()) // force a layout pass on the list itself

	// Find the *verbRow showing "bravo" (the second request) once the list has
	// rendered its rows, and aim the click at its centre. We target by content so
	// the test asserts the RIGHT request opens, not just "a" request.
	target := findVerbRowFor(t, w, "bravo")
	center := centerOf(w.app.fyneApp, target)

	// Sanity: the test really did lay out a non-zero row at a real position.
	if target.Size().IsZero() {
		t.Fatal("verbRow has zero size; the sidebar did not lay out — geometry walk would be meaningless")
	}

	// --- Mirror the driver's hit-test (step 1): deepest broad-tappable match. ---
	found := findBroadTappableAt(w.app.fyneApp, w.win.Canvas().Content(), center)
	if found == nil {
		t.Fatal("no broad-tappable object found under the row centre; layout/walk is wrong")
	}

	// Document WHY the broad predicate matters: the deepest broad-tappable match
	// over a sidebar row is the *verbRow itself (it shadows the List's listItem).
	// This holds REGARDLESS of the fix — verbRow is SecondaryTappable either way —
	// so it is a stable assertion that the geometry/walk really resolved to a row.
	row, ok := found.(*verbRow)
	if !ok {
		t.Fatalf("deepest broad-tappable object over the row is %T, want *verbRow; "+
			"if this changed, the #13 shadowing analysis no longer holds", found)
	}
	if row.id != 1 {
		t.Fatalf("hit-test resolved to verbRow id %d, want 1 (the \"bravo\" row)", row.id)
	}

	// --- Mirror the driver's dispatch (step 2): primary tap iff fyne.Tappable. ---
	// This is the crux of #13: with the bug present verbRow is NOT fyne.Tappable,
	// so the driver (and this mirror) skips Tapped entirely and NO tab opens — the
	// exact regression symptom, asserted just below as len(openTabs) == 0 failing.
	if tap, ok := found.(fyne.Tappable); ok {
		tap.Tapped(&fyne.PointEvent{
			Position:         center.Subtract(absPos(w.app.fyneApp, found)),
			AbsolutePosition: center,
		})
	}

	// --- PRIMARY assertion: the full routing ran and the clicked request opened.
	// This is what fails (0 tabs) if verbRow.Tapped is removed (reverting #13). ---
	if len(w.openTabs) != 1 {
		t.Fatalf("primary tap should open exactly one tab, got %d "+
			"(regression #13: verbRow not fyne.Tappable -> tap swallowed -> no selection)", len(w.openTabs))
	}
	if _, ok := w.openTabs[1]; !ok {
		t.Fatalf("the opened tab should be request index 1 (\"bravo\"); openTabs=%v", keysOf(w.openTabs))
	}
	if w.selectedID != 1 {
		t.Fatalf("selectedID should be 1 after the tap (accent/selection path ran), got %d", w.selectedID)
	}
	if got := w.coll.Requests[1].Name; got != "bravo" {
		t.Fatalf("sanity: opened request name = %q, want %q", got, "bravo")
	}

	// Belt-and-braces documentation of the fix: the matched row must satisfy
	// fyne.Tappable (the property #13 added). Kept after the routing assertions so
	// the primary failure is the observable "0 tabs opened", with this as the
	// explanatory interface-level guard.
	if _, ok := found.(fyne.Tappable); !ok {
		t.Fatal("the matched *verbRow must satisfy fyne.Tappable so the driver " +
			"dispatches a primary Tapped to it (issue #13)")
	}
}

// findVerbRowFor renders the sidebar and returns the *verbRow currently showing
// the request whose DisplayName is name. It fails the test if no such row is
// laid out (e.g. the list never rendered its items).
func findVerbRowFor(t *testing.T, w *Window, name string) *verbRow {
	t.Helper()
	var match *verbRow
	walkObjects(w.app.fyneApp, w.win.Canvas().Content(), func(o fyne.CanvasObject) {
		if vr, ok := o.(*verbRow); ok && vr.name != nil && vr.name.Text == name {
			match = vr
		}
	})
	if match == nil {
		t.Fatalf("no rendered verbRow shows %q; the sidebar list did not lay out its rows", name)
	}
	return match
}

// findBroadTappableAt mirrors fyne's internal driver
// FindObjectAtPositionMatching: it walks the laid-out object tree TOP-DOWN and
// returns the DEEPEST (last visited in the top-down walk) object whose absolute
// bounds contain pt AND which matches the driver's broad-tappable predicate
// (fyne.Tappable | fyne.SecondaryTappable | fyne.DoubleTappable | fyne.Focusable
// | desktop.Mouseable). This is the SAME predicate processMouseClicked uses, and
// is broader than fyne/test's Tap (which matches only fyne.Tappable) — that
// breadth is exactly what reproduces issue #13.
func findBroadTappableAt(app fyne.App, root fyne.CanvasObject, pt fyne.Position) fyne.CanvasObject {
	var found fyne.CanvasObject
	walkObjects(app, root, func(o fyne.CanvasObject) {
		if !o.Visible() {
			return
		}
		if !isBroadTappable(o) {
			return
		}
		pos := absPos(app, o)
		size := o.Size()
		if pt.X < pos.X || pt.Y < pos.Y {
			return
		}
		if pt.X >= pos.X+size.Width || pt.Y >= pos.Y+size.Height {
			return
		}
		// Keep the deepest/last match (top-down walk order matches the driver).
		found = o
	})
	return found
}

// isBroadTappable reports whether o implements ANY of the interfaces the glfw
// driver treats as a click target (see window.go processMouseClicked). Draggable
// is deliberately omitted: the driver only includes it once a drag has started,
// which a single click does not.
func isBroadTappable(o fyne.CanvasObject) bool {
	switch o.(type) {
	case fyne.Tappable, fyne.SecondaryTappable, fyne.DoubleTappable, fyne.Focusable, desktop.Mouseable:
		return true
	}
	return false
}

// walkObjects walks the laid-out canvas object tree top-down, invoking visit for
// each object in the same order fyne's WalkVisibleObjectTree does. It recurses
// into *fyne.Container children and into widgets via their rendered objects
// (test.WidgetRenderer, which is the public equivalent of cache.Renderer used by
// the driver's walk).
func walkObjects(app fyne.App, o fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	if o == nil {
		return
	}
	visit(o)
	switch co := o.(type) {
	case *fyne.Container:
		for _, child := range co.Objects {
			walkObjects(app, child, visit)
		}
	case *container.Scroll:
		walkObjects(app, co.Content, visit)
	case fyne.Widget:
		r := test.WidgetRenderer(co)
		if r == nil {
			return
		}
		for _, child := range r.Objects() {
			walkObjects(app, child, visit)
		}
	}
}

// absPos returns the absolute position of o in the canvas, via the driver's own
// AbsolutePositionForObject so it matches the geometry the real router uses.
func absPos(app fyne.App, o fyne.CanvasObject) fyne.Position {
	return app.Driver().AbsolutePositionForObject(o)
}

// centerOf returns the absolute centre point of o.
func centerOf(app fyne.App, o fyne.CanvasObject) fyne.Position {
	p := absPos(app, o)
	s := o.Size()
	return fyne.NewPos(p.X+s.Width/2, p.Y+s.Height/2)
}
