package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/ultramcu/yon/internal/model"
)

// TestFileMenu_SaveShortcuts pins that File ▸ Save carries Cmd/Ctrl+S and
// File ▸ Save As… carries Cmd/Ctrl+Shift+S, so the accelerators are shown and
// fire (a menu accelerator works even while a text field is focused).
func TestFileMenu_SaveShortcuts(t *testing.T) {
	w := newScopeWindow(t, model.NewCollection("T"))

	var file *fyne.Menu
	for _, m := range w.buildMainMenu().Items {
		if m.Label == "File" {
			file = m
		}
	}
	if file == nil {
		t.Fatal("no File menu")
	}

	itemByLabel := func(label string) *fyne.MenuItem {
		for _, it := range file.Items {
			if it.Label == label {
				return it
			}
		}
		return nil
	}

	cases := []struct {
		label string
		want  fyne.Shortcut
	}{
		{"Save", &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault}},
		{"Save As…", &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}},
	}
	for _, c := range cases {
		it := itemByLabel(c.label)
		if it == nil {
			t.Fatalf("no %q menu item", c.label)
		}
		sc, ok := it.Shortcut.(*desktop.CustomShortcut)
		if !ok {
			t.Fatalf("%q has no keyboard shortcut (%T)", c.label, it.Shortcut)
		}
		if sc.ShortcutName() != c.want.ShortcutName() {
			t.Fatalf("%q shortcut = %q, want %q", c.label, sc.ShortcutName(), c.want.ShortcutName())
		}
	}
}
