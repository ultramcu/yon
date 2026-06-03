package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/ultramcu/yon/internal/model"
)

// findMenu returns the menu with the given label from the main menu, or nil.
func findMenu(mm *fyne.MainMenu, label string) *fyne.Menu {
	for _, m := range mm.Items {
		if m.Label == label {
			return m
		}
	}
	return nil
}

// findItem returns the menu item with the given label, or nil.
func findItem(m *fyne.Menu, label string) *fyne.MenuItem {
	for _, it := range m.Items {
		if it.Label == label {
			return it
		}
	}
	return nil
}

// TestSaveShortcut_BlindSpecA pins SPEC A: File ▸ Save carries Cmd/Ctrl+S and
// File ▸ Save As… carries Cmd/Ctrl+Shift+S.
func TestSaveShortcut_BlindSpecA(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))
	mm := w.buildMainMenu()

	fileMenu := findMenu(mm, "File")
	if fileMenu == nil {
		t.Fatalf("no File menu found in main menu")
	}

	saveItem := findItem(fileMenu, "Save")
	if saveItem == nil {
		t.Fatalf("no \"Save\" item found in File menu")
	}
	wantSave := (&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierShortcutDefault,
	}).ShortcutName()
	if saveItem.Shortcut == nil {
		t.Fatalf("Save item has nil Shortcut, want %q", wantSave)
	}
	if got := saveItem.Shortcut.ShortcutName(); got != wantSave {
		t.Errorf("Save shortcut = %q, want %q", got, wantSave)
	} else {
		t.Logf("Save shortcut = %q (PASS)", got)
	}

	saveAsItem := findItem(fileMenu, "Save As…")
	if saveAsItem == nil {
		t.Fatalf("no \"Save As…\" item found in File menu")
	}
	wantSaveAs := (&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift,
	}).ShortcutName()
	if saveAsItem.Shortcut == nil {
		t.Fatalf("Save As… item has nil Shortcut, want %q", wantSaveAs)
	}
	if got := saveAsItem.Shortcut.ShortcutName(); got != wantSaveAs {
		t.Errorf("Save As… shortcut = %q, want %q", got, wantSaveAs)
	} else {
		t.Logf("Save As… shortcut = %q (PASS)", got)
	}
}
