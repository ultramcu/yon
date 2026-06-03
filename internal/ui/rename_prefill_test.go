package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// TestRenamePrefill pins what the Rename dialog starts with: the existing name,
// or the file base name (no .yon) when the collection is file-backed but unnamed
// — so the field is never blank for a saved file (which felt like creating a new
// collection). A truly untitled collection prefills empty.
func TestRenamePrefill(t *testing.T) {
	cases := []struct {
		desc, name, path, want string
	}{
		{"named", "My API", "/tmp/api.yon", "My API"},
		{"file-backed, no name", "", "/tmp/orders.yon", "orders"},
		{"untitled", "", "", ""},
	}
	for _, c := range cases {
		a := test.NewApp()
		w := newWindow(New(a), model.NewCollection(c.name), c.path)
		got := w.renamePrefill()
		w.win.Close()
		if got != c.want {
			t.Errorf("%s: renamePrefill() = %q, want %q", c.desc, got, c.want)
		}
	}
}
