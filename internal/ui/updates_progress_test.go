package ui

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// TestCheckForUpdates_ShowsCheckingProgress pins that the "Checking for updates…"
// indicator appears (an overlay on the canvas) and is removed when hidden, so a
// manual check gives visible feedback instead of seeming to do nothing.
func TestCheckForUpdates_ShowsCheckingProgress(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))

	if n := len(w.win.Canvas().Overlays().List()); n != 0 {
		t.Fatalf("expected no overlay before check, got %d", n)
	}

	d := w.showCheckingDialog()
	if n := len(w.win.Canvas().Overlays().List()); n == 0 {
		t.Fatal("expected a 'Checking for updates…' progress overlay while checking, got none")
	}

	d.Hide()
	if n := len(w.win.Canvas().Overlays().List()); n != 0 {
		t.Fatalf("expected the overlay to be removed after Hide, got %d", n)
	}
}
