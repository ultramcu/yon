package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// saveCollection writes coll to a fresh .yon file under t.TempDir and returns
// the path, so a restore can actually load it back from disk.
func saveCollection(t *testing.T, coll model.Collection, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := store.Save(path, coll); err != nil {
		t.Fatalf("seed save %s: %v", name, err)
	}
	return path
}

// TestSession_XCloseLastWindowRestores reproduces the red-X close bug: closing
// the only/last saved window via finishClose (followed by the post-quit
// OnStopped saveSession that fires after the window is forgotten) must keep the
// collection in the session so the next launch restores it.
func TestSession_XCloseLastWindowRestores(t *testing.T) {
	fa := test.NewApp() // ONE in-memory Preferences shared across both launches
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{{Name: "R", Method: model.MethodGet, URL: "http://x/y"}}
	path := saveCollection(t, coll, "c.yon")

	// Launch 1: open the saved collection, then close it via the red X.
	a1 := New(fa)
	w := a1.OpenCollectionWindow(coll, path)
	w.openRequestTab(0)
	w.finishClose()
	a1.saveSession() // model the OnStopped hook firing after the window is forgotten

	// Launch 2: same Preferences → the collection must come back.
	a2 := New(fa)
	if !a2.restoreSession() {
		t.Fatal("restoreSession should reopen the closed collection")
	}
	got := a2.anyWindow()
	if got == nil {
		t.Fatal("restore produced no window")
	}
	if got.path != path {
		t.Fatalf("restored path = %q, want %q", got.path, path)
	}
	if len(got.coll.Requests) != 1 || got.coll.Requests[0].Name != "R" {
		t.Fatalf("restored collection not the saved one: %+v", got.coll.Requests)
	}
}

// TestSession_CloseOneOfTwoPersistsOther verifies that closing one of two open
// windows persists only the remaining window (the closed one is NOT restored).
func TestSession_CloseOneOfTwoPersistsOther(t *testing.T) {
	fa := test.NewApp()
	collA := model.NewCollection("A")
	collA.Requests = []model.Request{{Name: "ra", Method: model.MethodGet, URL: "http://a/"}}
	collB := model.NewCollection("B")
	collB.Requests = []model.Request{{Name: "rb", Method: model.MethodGet, URL: "http://b/"}}
	pathA := saveCollection(t, collA, "a.yon")
	pathB := saveCollection(t, collB, "b.yon")

	a1 := New(fa)
	a1.OpenCollectionWindow(collA, pathA)
	wb := a1.OpenCollectionWindow(collB, pathB)
	wb.finishClose() // close B while A remains open

	a2 := New(fa)
	if !a2.restoreSession() {
		t.Fatal("restoreSession should reopen the remaining window")
	}
	var paths []string
	for w := range a2.windows {
		paths = append(paths, w.path)
	}
	if len(paths) != 1 || paths[0] != pathA {
		t.Fatalf("expected only %q restored, got %v", pathA, paths)
	}
}

// TestSession_UntitledCloseDoesNotClobber verifies that closing an untitled
// (never-saved) last window does not overwrite a previously-good session.
func TestSession_UntitledCloseDoesNotClobber(t *testing.T) {
	fa := test.NewApp()
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{{Name: "R", Method: model.MethodGet, URL: "http://x/y"}}
	path := saveCollection(t, coll, "c.yon")

	// Seed a good session from a saved-window launch.
	a1 := New(fa)
	w := a1.OpenCollectionWindow(coll, path)
	w.finishClose()
	a1.saveSession()

	// A later launch with only an untitled window, closed via the X, must not
	// clobber the good session above.
	a2 := New(fa)
	wu := a2.NewCollectionWindow() // untitled, path == ""
	wu.finishClose()
	a2.saveSession()

	a3 := New(fa)
	if !a3.restoreSession() {
		t.Fatal("the prior good session must survive an untitled close")
	}
	if got := a3.anyWindow(); got == nil || got.path != path {
		t.Fatalf("restored window path = %v, want %q", got, path)
	}
}

// TestSession_EmptySaveIsNoOp verifies the guard: saving with no rememberable
// windows must not write (and so must not clobber) the session key.
func TestSession_EmptySaveIsNoOp(t *testing.T) {
	fa := test.NewApp()
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{{Name: "R", Method: model.MethodGet, URL: "http://x/y"}}
	path := saveCollection(t, coll, "c.yon")

	a := New(fa)
	w := a.OpenCollectionWindow(coll, path)
	a.saveSession() // write a good session
	w.finishClose() // also persists (last window)
	want := fa.Preferences().String(prefKeySession)
	if want == "" {
		t.Fatal("precondition: a good session should have been written")
	}

	// Now the live set is empty; saveSession must be a no-op and leave the key intact.
	a.saveSession()
	if got := fa.Preferences().String(prefKeySession); got != want {
		t.Fatalf("empty saveSession clobbered the session: got %q, want %q", got, want)
	}
}
