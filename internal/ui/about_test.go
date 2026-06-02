package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestShowAboutDialog(t *testing.T) {
	a := test.NewApp()
	app := New(a)
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)

	app.showAboutDialog(w)
	if len(w.Canvas().Overlays().List()) == 0 {
		t.Fatal("About Yon dialog should appear as an overlay")
	}
}
