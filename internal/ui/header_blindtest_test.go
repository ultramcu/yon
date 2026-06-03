package ui

import (
	"testing"

	fynetest "fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// BT_B1 — SPEC B: collectionDisplayName returns the Collection Name when set.
func TestBT_CollectionDisplayName_NameWins(t *testing.T) {
	if got := collectionDisplayName("My Coll", "/tmp/whatever.yon"); got != "My Coll" {
		t.Errorf("name should win; got %q want %q", got, "My Coll")
	}
}

// BT_B2 — SPEC B: with no Name but a path, it returns the file's base name.
func TestBT_CollectionDisplayName_PathBaseFallback(t *testing.T) {
	if got := collectionDisplayName("", "/Users/x/api/orders.yon"); got != "orders.yon" {
		t.Errorf("path base should be used; got %q want %q", got, "orders.yon")
	}
}

// BT_B3 — SPEC B: with neither Name nor path, it returns "Untitled".
func TestBT_CollectionDisplayName_UntitledFallback(t *testing.T) {
	if got := collectionDisplayName("", ""); got != "Untitled" {
		t.Errorf("empty name+path should be Untitled; got %q", got)
	}
}

// BT_B4 — SPEC B (the bug): the sidebar header label for a name-less collection
// opened FROM A FILE must show the file's base name, not "Untitled".
func TestBT_SidebarTitle_NameLessFromFileShowsBase(t *testing.T) {
	app := New(fynetest.NewApp())
	coll := model.NewCollection("") // name-less
	w := newWindow(app, coll, "/Users/x/api/orders.yon")

	if w.sidebarTitle == nil {
		t.Fatal("sidebarTitle is nil")
	}
	if got := w.sidebarTitle.Text; got != "orders.yon" {
		t.Errorf("sidebar header for name-less file-backed collection = %q; want %q (regression: bug showed Untitled)", got, "orders.yon")
	}
}

// BT_B5 — SPEC B: a named collection's sidebar header shows the Name.
func TestBT_SidebarTitle_NamedShowsName(t *testing.T) {
	app := New(fynetest.NewApp())
	coll := model.NewCollection("Orders API")
	w := newWindow(app, coll, "/Users/x/api/orders.yon")

	if w.sidebarTitle == nil {
		t.Fatal("sidebarTitle is nil")
	}
	if got := w.sidebarTitle.Text; got != "Orders API" {
		t.Errorf("sidebar header = %q; want %q", got, "Orders API")
	}
}

// BT_B6 — SPEC B: a name-less collection with NO path shows "Untitled".
func TestBT_SidebarTitle_NoNameNoPathUntitled(t *testing.T) {
	app := New(fynetest.NewApp())
	coll := model.NewCollection("")
	w := newWindow(app, coll, "")

	if w.sidebarTitle == nil {
		t.Fatal("sidebarTitle is nil")
	}
	if got := w.sidebarTitle.Text; got != "Untitled" {
		t.Errorf("sidebar header = %q; want %q", got, "Untitled")
	}
}
