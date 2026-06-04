package ui

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// TestDuplicateRequest_DeepCopyGuarded proves the copy's Params/Headers are
// independent of the source — with teeth. The plain deep-copy tests pass even on
// a shallow copy because duplicateRequest selects the copy, which opens its
// editor and rebuilds its slices (masking any aliasing). Here OnSelected is
// detached so the copy is NOT opened, so a shallow copy would leave the slices
// shared and this test would catch it. Guards data integrity of the original
// request against an accidental shallow copy.
func TestDuplicateRequest_DeepCopyGuarded(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{{
		Name:    "a",
		Method:  model.MethodGet,
		Params:  []model.Param{{Key: "k", Value: "orig", Enabled: true}},
		Headers: []model.Param{{Key: "H", Value: "horig", Enabled: true}},
	}}
	w := newTestWindow(coll)
	w.sidebar.OnSelected = nil // don't open the copy's tab (that would re-clone its slices)

	w.duplicateRequest(0)

	// Mutate the stored copy's slices in place; the original must not change.
	w.coll.Requests[1].Params[0].Value = "MUT"
	w.coll.Requests[1].Headers[0].Value = "MUTH"

	if got := w.coll.Requests[0].Params[0].Value; got != "orig" {
		t.Fatalf("Params aliased: original Value = %q, want %q (copy is not a deep copy)", got, "orig")
	}
	if got := w.coll.Requests[0].Headers[0].Value; got != "horig" {
		t.Fatalf("Headers aliased: original Value = %q, want %q", got, "horig")
	}
}

// TestVerbRow_TappedSecondaryFiresOnDuplicate guards the menu wiring: the
// right-click menu must carry a "Duplicate" item that invokes onDuplicate with
// the row's id (mirrors TestVerbRow_TappedSecondaryFiresOnDelete). Without this,
// removing the menu item or its buildSidebar wiring would go uncaught.
func TestVerbRow_TappedSecondaryFiresOnDuplicate(t *testing.T) {
	row := newVerbRow()

	var got int
	fired := false
	row.onDuplicate = func(id int) {
		got = id
		fired = true
	}
	row.set(3, model.Request{Method: model.MethodGet, Name: "z"}, false)

	w := test.NewWindow(row)
	t.Cleanup(w.Close)

	row.TappedSecondary(&fyne.PointEvent{})

	item := menuItemByLabel(t, w.Canvas(), "Duplicate")
	item.Action()

	if !fired {
		t.Fatal("the row's Duplicate menu item should invoke onDuplicate")
	}
	if got != 3 {
		t.Fatalf("onDuplicate got id %d, want 3", got)
	}
}

// menuItemByLabel finds a popped-up menu item by label, reusing popUpMenuIn
// (defined in verbrow_tap_test.go).
func menuItemByLabel(t *testing.T, c fyne.Canvas, label string) *fyne.MenuItem {
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
			if mi, ok := f.Interface().(*fyne.MenuItem); ok && mi.Label == label {
				return mi
			}
		}
	}
	t.Fatalf("TappedSecondary should pop up a menu with a %q item", label)
	return nil
}
