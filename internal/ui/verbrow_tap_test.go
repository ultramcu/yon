package ui

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// verbRow gained TappedSecondary (Delete) but not Tapped, which made it
// SecondaryTappable while NOT fyne.Tappable. The glfw tap router resolves a
// click to the deepest object implementing ANY tappable interface, so a left
// click resolved to the row (no primary handler) and shadowed the enclosing
// widget.List's own selection — rows stopped opening on click. This is the
// load-bearing regression guard: verbRow MUST be fyne.Tappable.
func TestVerbRow_IsTappable(t *testing.T) {
	if _, ok := interface{}(newVerbRow()).(fyne.Tappable); !ok {
		t.Fatal("verbRow must implement fyne.Tappable: it is SecondaryTappable, " +
			"which otherwise shadows the List's own primary-tap selection so rows never open")
	}
}

// A primary (left) tap must invoke onTap with the row's current request index.
func TestVerbRow_TappedFiresOnTap(t *testing.T) {
	row := newVerbRow()

	var got int
	fired := false
	row.onTap = func(id int) {
		got = id
		fired = true
	}

	row.set(3, model.Request{Method: model.MethodGet, Name: "x"}, false)
	row.Tapped(&fyne.PointEvent{})

	if !fired {
		t.Fatal("Tapped should invoke onTap")
	}
	if got != 3 {
		t.Fatalf("onTap got id %d, want 3", got)
	}
}

// Delete must not regress: a right-click (TappedSecondary) still pops up a menu
// whose "Delete" item drives onDelete with the row's current request index. The
// row needs a live canvas for the popup, so it's parented in a test window; we
// then fire the popped-up menu's Delete action.
func TestVerbRow_TappedSecondaryFiresOnDelete(t *testing.T) {
	row := newVerbRow()

	var got int
	fired := false
	row.onDelete = func(id int) {
		got = id
		fired = true
	}
	row.set(2, model.Request{Method: model.MethodPost, Name: "y"}, false)

	w := test.NewWindow(row)
	t.Cleanup(w.Close)

	row.TappedSecondary(&fyne.PointEvent{})

	item := deleteMenuItem(t, w.Canvas())
	item.Action()

	if !fired {
		t.Fatal("the row's Delete menu item should invoke onDelete")
	}
	if got != 2 {
		t.Fatalf("onDelete got id %d, want 2", got)
	}
}

// deleteMenuItem finds the "Delete" *fyne.MenuItem in the popup menu currently
// overlaying the canvas, failing the test if no such menu/item is present. The
// popup is wrapped in an OverlayContainer whose Content is the *PopUpMenu, whose
// Items are unexported menuItem widgets that wrap a *fyne.MenuItem in an exported
// "Item" field — so we unwrap the container and read each item reflectively.
func deleteMenuItem(t *testing.T, c fyne.Canvas) *fyne.MenuItem {
	t.Helper()
	for _, o := range c.Overlays().List() {
		menu := popUpMenuIn(o)
		if menu == nil {
			continue
		}
		for _, obj := range menu.Items {
			f := reflect.ValueOf(obj).Elem().FieldByName("Item")
			if !f.IsValid() {
				continue
			}
			if mi, ok := f.Interface().(*fyne.MenuItem); ok && mi.Label == "Delete" {
				return mi
			}
		}
	}
	t.Fatal("TappedSecondary should pop up a menu with a Delete item")
	return nil
}

// popUpMenuIn returns the *widget.PopUpMenu carried by an overlay object, either
// directly or wrapped in an OverlayContainer's Content field.
func popUpMenuIn(o fyne.CanvasObject) *widget.PopUpMenu {
	if m, ok := o.(*widget.PopUpMenu); ok {
		return m
	}
	f := reflect.ValueOf(o).Elem().FieldByName("Content")
	if !f.IsValid() {
		return nil
	}
	if m, ok := f.Interface().(*widget.PopUpMenu); ok {
		return m
	}
	return nil
}
