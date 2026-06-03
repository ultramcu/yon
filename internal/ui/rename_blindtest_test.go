package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// fileBackedWindow builds a Window backed by coll at the given file path, the
// way SPEC B prescribes for the file-backed cases.
func fileBackedWindow(t *testing.T, coll model.Collection, path string) *Window {
	t.Helper()
	a := test.NewApp()
	w := newWindow(New(a), coll, path)
	t.Cleanup(w.win.Close)
	return w
}

// TestRenameCollection_SetsNameHeaderDirty_BlindSpecB pins that renameCollection
// trims the name, sets coll.Name, marks dirty, and updates the sidebar header.
func TestRenameCollection_SetsNameHeaderDirty_BlindSpecB(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))
	w.dirty = false // start clean to observe the dirty flip

	w.renameCollection("  spaced  ")

	if got := w.coll.Name; got != "spaced" {
		t.Errorf("coll.Name = %q, want %q (trimmed)", got, "spaced")
	} else {
		t.Logf("coll.Name = %q (PASS)", got)
	}
	if got := w.sidebarTitle.Text; got != "spaced" {
		t.Errorf("sidebarTitle.Text = %q, want %q", got, "spaced")
	} else {
		t.Logf("sidebarTitle.Text = %q (PASS)", got)
	}
	if !w.dirty {
		t.Errorf("dirty = false, want true after renameCollection")
	} else {
		t.Logf("dirty = true (PASS)")
	}
}

// TestRenameCollection_EmptyFallsBackToFileBase_BlindSpecB pins that an empty
// name clears coll.Name so the header falls back to the file base name.
func TestRenameCollection_EmptyFallsBackToFileBase_BlindSpecB(t *testing.T) {
	w := fileBackedWindow(t, model.NewCollection("Named"), "/tmp/orders.yon")

	w.renameCollection("")

	if got := w.coll.Name; got != "" {
		t.Errorf("coll.Name = %q, want empty after rename to \"\"", got)
	} else {
		t.Logf("coll.Name = %q (PASS)", got)
	}
	if got := w.sidebarTitle.Text; got != "orders.yon" {
		t.Errorf("sidebarTitle.Text = %q, want %q (file base)", got, "orders.yon")
	} else {
		t.Logf("sidebarTitle.Text = %q (PASS)", got)
	}
}

// TestRenamePrefill_Named_BlindSpecB: a named collection prefills its name.
func TestRenamePrefill_Named_BlindSpecB(t *testing.T) {
	w := fileBackedWindow(t, model.NewCollection("My Coll"), "/tmp/orders.yon")
	if got := w.renamePrefill(); got != "My Coll" {
		t.Errorf("renamePrefill() = %q, want %q", got, "My Coll")
	} else {
		t.Logf("renamePrefill() = %q (PASS)", got)
	}
}

// TestRenamePrefill_FileBackedUnnamed_BlindSpecB: an unnamed, file-backed
// collection prefills the file base name without the .yon extension.
func TestRenamePrefill_FileBackedUnnamed_BlindSpecB(t *testing.T) {
	coll := model.NewCollection("Named")
	coll.Name = "" // unnamed
	w := fileBackedWindow(t, coll, "/tmp/orders.yon")
	if got := w.renamePrefill(); got != "orders" {
		t.Errorf("renamePrefill() = %q, want %q", got, "orders")
	} else {
		t.Logf("renamePrefill() = %q (PASS)", got)
	}
}

// TestRenamePrefill_Untitled_BlindSpecB: an untitled, unnamed collection (no
// file path) prefills empty.
func TestRenamePrefill_Untitled_BlindSpecB(t *testing.T) {
	coll := model.NewCollection("Named")
	coll.Name = "" // unnamed
	w := newScopeWindow(t, coll)
	if got := w.renamePrefill(); got != "" {
		t.Errorf("renamePrefill() = %q, want empty", got)
	} else {
		t.Logf("renamePrefill() = %q (PASS)", got)
	}
}
