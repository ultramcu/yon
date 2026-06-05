package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// These tests cover the UI AFFORDANCE that opens the Variables dock — the piece
// the panel makers missed. The dock logic (toggleVarsPanel/syncVarsToggle) was
// complete, but nothing in the visible UI invoked it: the footer had no
// "Variables" toggle and there was no View menu. Without these wires the panel
// is unreachable in normal use, so both must FAIL before the wiring and PASS
// after.

// canvasTreeContains reports whether obj appears anywhere in the canvas-object
// tree rooted at root — walking container children and tappable wrappers (whose
// child isn't a regular container child).
func canvasTreeContains(root, target fyne.CanvasObject) bool {
	if root == nil {
		return false
	}
	if root == target {
		return true
	}
	switch o := root.(type) {
	case *fyne.Container:
		for _, c := range o.Objects {
			if canvasTreeContains(c, target) {
				return true
			}
		}
	case *container.Split:
		return canvasTreeContains(o.Leading, target) || canvasTreeContains(o.Trailing, target)
	case *tappable:
		return canvasTreeContains(o.child, target)
	}
	return false
}

// findTappableFor walks the tree and returns the *tappable whose child is target
// (nil if none) — i.e. the clickable wrapper around a canvas.Text label.
func findTappableFor(root fyne.CanvasObject, target fyne.CanvasObject) *tappable {
	switch o := root.(type) {
	case *tappable:
		if o.child == target {
			return o
		}
		return findTappableFor(o.child, target)
	case *fyne.Container:
		for _, c := range o.Objects {
			if t := findTappableFor(c, target); t != nil {
				return t
			}
		}
	case *container.Split:
		if t := findTappableFor(o.Leading, target); t != nil {
			return t
		}
		return findTappableFor(o.Trailing, target)
	}
	return nil
}

// TestVarsToggleAffordanceWired asserts the footer "Variables" toggle exists,
// is actually embedded in the status bar tree, and that tapping its wrapper
// flips the dock's visibility — the on-screen handle the makers never wired.
func TestVarsToggleAffordanceWired(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	w := app.OpenCollectionWindow(model.NewCollection("Vars"), "/tmp/vars.yon")

	if w.varsToggle == nil {
		t.Fatal("w.varsToggle is nil: buildStatusBar never created the footer Variables toggle")
	}
	if w.varsToggle.Text != "Variables" {
		t.Fatalf("footer toggle has wrong label %q, want %q", w.varsToggle.Text, "Variables")
	}

	// The toggle must really live in the status bar tree, not be an orphan field.
	if !canvasTreeContains(w.contentBottom, w.varsToggle) {
		t.Fatal("status bar does not contain w.varsToggle: the toggle isn't shown in the footer")
	}

	// And its tappable wrapper must flip the dock when clicked.
	tap := findTappableFor(w.contentBottom, w.varsToggle)
	if tap == nil {
		t.Fatal("no tappable wraps w.varsToggle: the footer label is not clickable")
	}

	before := w.varsVisible
	tap.Tapped(&fyne.PointEvent{})
	if w.varsVisible == before {
		t.Fatal("tapping the footer Variables toggle did not flip varsVisible")
	}
	tap.Tapped(&fyne.PointEvent{})
	if w.varsVisible != before {
		t.Fatal("second tap did not flip varsVisible back")
	}
}

// TestViewMenuHasVariables asserts the main menu has a "View" menu containing a
// "Variables" item whose action opens the dock — the second affordance the
// makers never added.
func TestViewMenuHasVariables(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	w := app.OpenCollectionWindow(model.NewCollection("Vars"), "/tmp/vars.yon")

	mm := w.buildMainMenu()
	var view *fyne.Menu
	for _, m := range mm.Items {
		if m.Label == "View" {
			view = m
			break
		}
	}
	if view == nil {
		t.Fatal("no View menu: View ▸ Variables affordance is missing")
	}

	var item *fyne.MenuItem
	for _, it := range view.Items {
		if it.Label == "Variables" {
			item = it
			break
		}
	}
	if item == nil {
		t.Fatal("View menu has no Variables item")
	}
	if item.Action == nil {
		t.Fatal("View ▸ Variables has a nil Action: clicking it would do nothing")
	}

	before := w.varsVisible
	item.Action()
	if w.varsVisible == before {
		t.Fatal("invoking View ▸ Variables did not flip varsVisible")
	}
}

// TestVarsToggleSeededOpenFromPref asserts a window built with the dock-open
// pref set both opens the dock AND styles the footer toggle as open (accented +
// bold), proving syncVarsToggle runs after varsToggle is assigned.
func TestVarsToggleSeededOpenFromPref(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)
	app.prefs().SetBool(prefKeyVarsPanelOpen, true)

	w := app.OpenCollectionWindow(model.NewCollection("Vars"), "/tmp/vars.yon")

	if !w.varsVisible {
		t.Fatal("pref-seeded window did not open the Variables dock")
	}
	if w.varsToggle == nil {
		t.Fatal("w.varsToggle is nil on a pref-seeded-open window")
	}
	if !w.varsToggle.TextStyle.Bold {
		t.Error("footer toggle is not bold while the dock is open: syncVarsToggle did not reflect the seeded pref")
	}
}
