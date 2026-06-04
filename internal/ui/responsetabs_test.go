package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"github.com/ultramcu/yon/internal/model"
)

// The response area now uses a Body/Headers segTabs (same component as the
// request editor) so the body gets the full pane, instead of the old HSplit
// with a left headers column. These tests pin that layout and confirm the
// body perf architecture (bodyStack: TextGrid/List) is preserved.

// segLabels returns the current captions of the response sub-tab buttons.
func segLabels(s *segTabs) []string {
	out := make([]string, len(s.segs))
	for i, b := range s.segs {
		out[i] = b.label
	}
	return out
}

func TestRespTabs_HasBodyAndHeadersWithBodySelected(t *testing.T) {
	rv := bt2View(t)

	if rv.respTabs == nil {
		t.Fatal("response view should build a segTabs")
	}
	// Body and Headers, in that order; Body selected initially.
	if got := segLabels(rv.respTabs); len(got) != 2 || got[0] != "Body" || got[1] != "Headers" {
		t.Fatalf("tabs = %v, want [Body Headers]", got)
	}
	if rv.respTabs.selected != 0 {
		t.Errorf("selected tab = %d, want 0 (Body)", rv.respTabs.selected)
	}
}

// The Body tab must hold the existing bodyStack (the perf-critical TextGrid +
// List renderer) — i.e. the perf architecture is intact.
func TestRespTabs_BodyTabIsBodyStack(t *testing.T) {
	rv := bt2View(t)
	if rv.respTabs.contents[0] != fyne.CanvasObject(rv.bodyStack) {
		t.Error("Body tab content should be rv.bodyStack (TextGrid/List perf renderer)")
	}
	// Sanity: bodyStack still wraps both viewers.
	found := false
	for _, o := range rv.bodyStack.Objects {
		if o == fyne.CanvasObject(rv.bodyScroll) {
			found = true
		}
	}
	if !found {
		t.Error("bodyStack should still contain bodyScroll (the TextGrid scroll)")
	}
}

// The Headers tab must contain the headersGrid (in a scroll), and after a
// response with headers the grid is populated while the body lands in the
// bodyStack.
func TestRespTabs_HeadersPopulateHeadersTabBodyInBodyTab(t *testing.T) {
	rv := bt2View(t)

	headers := []model.Param{
		{Key: "Content-Type", Value: "application/json"},
		{Key: "Server", Value: "yon-test/1.0"},
	}
	body := []byte(`{"ok":true,"name":"alpha"}`)
	rv.setResponse(model.Response{
		Status: 200, StatusText: "OK", Headers: headers, Body: body,
	})

	// Headers tab content must reach the headersGrid and it must hold the headers.
	hg := findTextGridInHeadersTab(t, rv)
	if hg != rv.headersGrid {
		t.Error("Headers tab should contain rv.headersGrid")
	}
	ht := rv.headersGrid.Text()
	for _, h := range headers {
		if !strings.Contains(ht, h.Key) || !strings.Contains(ht, h.Value) {
			t.Errorf("Headers tab missing %q: %q", h.Key, ht)
		}
	}

	// Body content lands in the bodyStack (TextGrid path for this small body).
	if !rv.bodyScroll.Visible() {
		t.Error("small body: bodyScroll (TextGrid) should be visible in the Body tab")
	}
	if !strings.Contains(rv.bodyGrid.Text(), "alpha") {
		t.Errorf("Body tab grid missing body content; got %q", rv.bodyGrid.Text())
	}
}

// The count badge reflects the number of response headers.
func TestRespTabs_HeadersBadgeShowsCount(t *testing.T) {
	rv := bt2View(t)
	rv.setResponse(model.Response{
		Status: 200, StatusText: "OK",
		Headers: []model.Param{
			{Key: "A", Value: "1"}, {Key: "B", Value: "2"}, {Key: "C", Value: "3"},
		},
		Body: []byte(`{}`),
	})
	if got := rv.headersSeg.label; got != "Headers 3" {
		t.Errorf("Headers tab label = %q, want %q", got, "Headers 3")
	}
	// Cleared back to bare label on a fresh send/pending.
	rv.setPending()
	if got := rv.headersSeg.label; got != "Headers" {
		t.Errorf("after pending, Headers label = %q, want %q", got, "Headers")
	}
}

// findTextGridInHeadersTab walks the Headers tab content for the headersGrid,
// asserting the tab wraps it (in a scroll) rather than referencing the field
// directly.
func findTextGridInHeadersTab(t *testing.T, rv *responseView) fyne.CanvasObject {
	t.Helper()
	var walk func(o fyne.CanvasObject) fyne.CanvasObject
	walk = func(o fyne.CanvasObject) fyne.CanvasObject {
		if o == fyne.CanvasObject(rv.headersGrid) {
			return o
		}
		switch c := o.(type) {
		case *fyne.Container:
			for _, ch := range c.Objects {
				if r := walk(ch); r != nil {
					return r
				}
			}
		case *container.Scroll:
			return walk(c.Content)
		}
		return nil
	}
	if r := walk(rv.respTabs.contents[1]); r != nil {
		return r
	}
	t.Fatal("headersGrid not found inside the Headers tab content")
	return nil
}
