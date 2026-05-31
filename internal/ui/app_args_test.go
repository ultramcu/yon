package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// TestOpenPath_OpensCollectionWindow verifies that opening a .yon path on
// startup loads that Collection into its own window (the CLI-arg path).
func TestOpenPath_OpensCollectionWindow(t *testing.T) {
	a := New(test.NewApp())

	path := filepath.Join(t.TempDir(), "demo.yon")
	coll := model.NewCollection("Demo")
	coll.Requests = append(coll.Requests, model.Request{
		Name: "R1", Method: model.MethodGet, URL: "http://example.com",
	})
	if err := store.Save(path, coll); err != nil {
		t.Fatalf("save fixture: %v", err)
	}

	if err := a.OpenPath(path); err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	if len(a.windows) != 1 {
		t.Fatalf("want 1 open window, got %d", len(a.windows))
	}
	for w := range a.windows {
		if w.coll.Name != "Demo" || len(w.coll.Requests) != 1 {
			t.Fatalf("loaded collection wrong: %+v", w.coll)
		}
		if w.path != path {
			t.Fatalf("window path = %q, want %q", w.path, path)
		}
	}
}

// TestOpenPath_BadFileErrors verifies a missing/invalid file returns an error
// and opens no window (so main can report it to stderr and fall back).
func TestOpenPath_BadFileErrors(t *testing.T) {
	a := New(test.NewApp())

	if err := a.OpenPath(filepath.Join(t.TempDir(), "nope.yon")); err == nil {
		t.Fatal("expected an error opening a missing file")
	}
	if len(a.windows) != 0 {
		t.Fatalf("no window should open on error, got %d", len(a.windows))
	}
}
