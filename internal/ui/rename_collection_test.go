package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// TestRenameCollection_SetsNameHeaderAndDirty pins that renaming a collection
// updates its Name, refreshes the sidebar header, and marks the window dirty —
// independent of the file name.
func TestRenameCollection_SetsNameHeaderAndDirty(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("")) // name-less

	w.renameCollection("  My API  ") // surrounding space is trimmed

	if w.coll.Name != "My API" {
		t.Fatalf("coll.Name = %q, want %q", w.coll.Name, "My API")
	}
	if w.sidebarTitle.Text != "My API" {
		t.Fatalf("sidebar header = %q, want %q", w.sidebarTitle.Text, "My API")
	}
	if !w.dirty {
		t.Fatal("rename should mark the window dirty")
	}
}

// TestRenameCollection_EmptyFallsBackToFilename pins that clearing the name falls
// back to the file-name display (collectionDisplayName), not a stale value.
func TestRenameCollection_EmptyFallsBackToFilename(t *testing.T) {
	a := test.NewApp()
	w := newWindow(New(a), model.NewCollection("My API"), "/tmp/orders.yon")
	t.Cleanup(w.win.Close)

	w.renameCollection("")

	if w.coll.Name != "" {
		t.Fatalf("coll.Name = %q, want empty", w.coll.Name)
	}
	if w.sidebarTitle.Text != "orders.yon" {
		t.Fatalf("sidebar header = %q, want file-name fallback %q", w.sidebarTitle.Text, "orders.yon")
	}
}
