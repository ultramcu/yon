package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// These tests cover the window-integration side of the Variables dock (Dev B):
// the toggle flips visibility and rebuilds the center, refreshVarsPanel is a
// no-op while hidden, and the open/closed state round-trips through Preferences.

// TestVarsPanel_ToggleFlipsVisibilityAndContent asserts toggleVarsPanel flips
// varsVisible and that the window's content object actually changes (the center
// gains/loses the outer HSplit wrapping the Variables panel).
func TestVarsPanel_ToggleFlipsVisibilityAndContent(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	coll := model.NewCollection("Vars")
	w := app.OpenCollectionWindow(coll, "/tmp/vars.yon")

	if w.varsVisible {
		t.Fatal("Variables dock should start hidden (default pref)")
	}
	hiddenContent := w.win.Content()

	// Open it.
	w.toggleVarsPanel()
	if !w.varsVisible {
		t.Fatal("toggleVarsPanel did not set varsVisible=true")
	}
	if w.win.Content() == hiddenContent {
		t.Error("window content did not change when the dock opened")
	}

	// Close it again — back to the hidden layout.
	w.toggleVarsPanel()
	if w.varsVisible {
		t.Fatal("toggleVarsPanel did not set varsVisible=false")
	}
}

// TestVarsPanel_InnerSplitReusedAcrossToggle asserts the sidebar|editor split
// object is the SAME pointer before and after toggling, so its drag offset (the
// user's sizing) is preserved.
func TestVarsPanel_InnerSplitReusedAcrossToggle(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	w := app.OpenCollectionWindow(model.NewCollection("Vars"), "/tmp/vars.yon")
	before := w.contentSplit
	if before == nil {
		t.Fatal("contentSplit was not stored")
	}

	w.toggleVarsPanel() // open
	if w.contentSplit != before {
		t.Error("inner split was rebuilt on open; offset/sizing would be lost")
	}
	w.toggleVarsPanel() // close
	if w.contentSplit != before {
		t.Error("inner split was rebuilt on close; offset/sizing would be lost")
	}
}

// TestVarsPanel_RefreshNoopWhenHidden asserts refreshVarsPanel does nothing (no
// panic, no state change) while the dock is hidden, and is safe when visible.
func TestVarsPanel_RefreshNoopWhenHidden(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	w := app.OpenCollectionWindow(model.NewCollection("Vars"), "/tmp/vars.yon")
	if w.varsVisible {
		t.Fatal("expected hidden start")
	}
	// Hidden: must be a safe no-op.
	w.refreshVarsPanel()

	// Visible: must still be safe.
	w.toggleVarsPanel()
	w.refreshVarsPanel()
}

// TestVarsPanel_StatePersistsToPreferences asserts the open/closed state is
// written to Preferences and re-seeds a freshly built window.
func TestVarsPanel_StatePersistsToPreferences(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	w := app.OpenCollectionWindow(model.NewCollection("Vars"), "/tmp/vars.yon")
	if w.varsVisible {
		t.Fatal("expected hidden start")
	}

	// Open it — the pref must flip to true.
	w.toggleVarsPanel()
	if !app.prefs().BoolWithFallback(prefKeyVarsPanelOpen, false) {
		t.Fatal("opening the dock did not persist true to Preferences")
	}

	// A new window on the same app must seed varsVisible from that pref.
	w2 := app.OpenCollectionWindow(model.NewCollection("Vars2"), "/tmp/vars2.yon")
	if !w2.varsVisible {
		t.Error("new window did not re-open the dock from the persisted pref")
	}

	// Close on the first window — pref flips back to false.
	w.toggleVarsPanel()
	if app.prefs().BoolWithFallback(prefKeyVarsPanelOpen, true) {
		t.Error("closing the dock did not persist false to Preferences")
	}
}
