package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// vfbtWindow builds a Window the same way the other ui tests do (test.NewApp +
// New(a) + newWindow), independent of the statusbar_test helpers. t.Cleanup
// closes the underlying window.
func vfbtWindow(t *testing.T) *Window {
	t.Helper()
	a := test.NewApp()
	w := newWindow(New(a), model.NewCollection("VFBT"), "")
	t.Cleanup(w.win.Close)
	return w
}

// vfbtFindHBox walks the canvas object tree (recursing into *fyne.Container and
// into widget renderers) looking for an *fyne.Container whose direct Objects
// include both want1 and want2. It returns that container and the direct-child
// indexes of want1 and want2 (or nil / -1 / -1 if not found).
func vfbtFindHBox(root fyne.CanvasObject, want1, want2 fyne.CanvasObject) (*fyne.Container, int, int) {
	var found *fyne.Container
	i1, i2 := -1, -1

	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if found != nil || o == nil {
			return
		}
		switch v := o.(type) {
		case *fyne.Container:
			a, b := -1, -1
			for idx, child := range v.Objects {
				if child == want1 {
					a = idx
				}
				if child == want2 {
					b = idx
				}
			}
			if a >= 0 && b >= 0 {
				found, i1, i2 = v, a, b
				return
			}
			for _, child := range v.Objects {
				walk(child)
			}
		case fyne.Widget:
			r := test.WidgetRenderer(v)
			if r == nil {
				return
			}
			for _, child := range r.Objects() {
				walk(child)
			}
		}
	}
	walk(root)
	return found, i1, i2
}

// TestVersionFooter_BlindSpec is an independent verification of the SPEC: the
// footer shows the app version as a *canvas.Text at the far LEFT, before the
// response status, and the existing status-bar fields survive the change.
func TestVersionFooter_BlindSpec(t *testing.T) {
	w := vfbtWindow(t)

	// SPEC 4 (part): building did not panic — reaching here proves it.

	// SPEC 1: w.sbVersion is a non-nil *canvas.Text.
	if w.sbVersion == nil {
		t.Fatalf("SPEC 1 FAIL: w.sbVersion is nil")
	}
	var _ *canvas.Text = w.sbVersion // compile-time type assertion of the field
	t.Logf("SPEC 1 PASS: w.sbVersion non-nil, text=%q", w.sbVersion.Text)

	// SPEC 2: text is "v"+currentVersion() when non-empty, else "dev".
	cur := currentVersion()
	if cur == "" {
		// Exercise the versioned case if nothing is set yet. SetBuildVersion only
		// takes effect when buildVersion was empty (an ldflags override wins), so
		// this is a no-op if a version is already injected.
		SetBuildVersion("9.9.9-blindtest")
		cur = currentVersion()
	}
	var wantVer string
	if cur != "" {
		wantVer = "v" + cur
	} else {
		wantVer = "dev"
	}
	// Rebuild so sbVersion reflects the (possibly just-set) currentVersion().
	w2 := vfbtWindow(t)
	if w2.sbVersion.Text != wantVer {
		t.Fatalf("SPEC 2 FAIL: sbVersion.Text=%q, currentVersion()=%q want %q",
			w2.sbVersion.Text, cur, wantVer)
	}
	t.Logf("SPEC 2 PASS: currentVersion()=%q, sbVersion.Text=%q (want %q)", cur, w2.sbVersion.Text, wantVer)
	w = w2 // continue ordering/survival checks against the rebuilt window

	// SPEC 3: sbVersion appears before sbStatus in the footer's left HBox.
	if w.sbStatus == nil {
		t.Fatalf("SPEC 3/4 FAIL: w.sbStatus is nil")
	}
	root := w.win.Canvas().Content()
	box, idxVer, idxStatus := vfbtFindHBox(root, w.sbVersion, w.sbStatus)
	if box == nil {
		t.Fatalf("SPEC 3 FAIL: no container holds both sbVersion and sbStatus")
	}
	if idxVer >= idxStatus {
		t.Fatalf("SPEC 3 FAIL: sbVersion index=%d not before sbStatus index=%d", idxVer, idxStatus)
	}
	t.Logf("SPEC 3 PASS: in left container (len=%d), sbVersion index=%d < sbStatus index=%d (firstItem=%v)",
		len(box.Objects), idxVer, idxStatus, idxVer == 0)

	// SPEC 4: sbStatus and sbReqInfo still present (not broken by the change).
	if w.sbStatus == nil {
		t.Fatalf("SPEC 4 FAIL: w.sbStatus is nil")
	}
	if w.sbReqInfo == nil {
		t.Fatalf("SPEC 4 FAIL: w.sbReqInfo is nil")
	}
	t.Logf("SPEC 4 PASS: sbStatus=%q, sbReqInfo present, no panic during build", w.sbStatus.Text)
}
