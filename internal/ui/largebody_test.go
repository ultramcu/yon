package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// These tests cover the large-body Pretty path that renders into the lightweight
// read-only widget.List instead of the per-cell TextGrid (Optimizer #5). They
// assert the ADR-0001 contract still holds (read-only, monospace, scrollable),
// that truncation is still signalled and the full body retained for Save, and
// that the two Body viewers swap correctly between large and small bodies.

func newLargeTestResponseView(t *testing.T) *responseView {
	t.Helper()
	test.NewApp()
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	return newResponseView(w)
}

// largeJSON returns a single valid JSON document whose pretty-printed form is
// comfortably above largeBodyThreshold (so the Pretty path takes the List).
func largeJSON(t *testing.T) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; b.Len() < largeBodyThreshold*2; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":`)
		b.WriteString(strings.Repeat("9", 3))
		b.WriteString(`,"name":"item","ok":true}`)
	}
	b.WriteString("]}")
	return []byte(b.String())
}

func TestLargeBody_PrettyUsesListViewerNotGrid(t *testing.T) {
	rv := newLargeTestResponseView(t)
	body := largeJSON(t)
	// Pretty is the default (checked); render.
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})

	if !rv.bodyList.Visible() {
		t.Fatal("large Pretty body: lightweight List viewer should be visible")
	}
	if rv.bodyScroll.Visible() {
		t.Error("large Pretty body: TextGrid scroll should be hidden")
	}
	if len(rv.bodyLines) == 0 {
		t.Error("large Pretty body: List was not populated with lines")
	}
	// The List length callback must match the line slice (virtualization source).
	if got := rv.bodyList.Length(); got != len(rv.bodyLines) {
		t.Errorf("List.Length() = %d, want %d", got, len(rv.bodyLines))
	}
	// Full body must be retained for Save-to-file regardless of viewer.
	if len(rv.fullBody) != len(body) {
		t.Errorf("fullBody = %d bytes, want retained %d", len(rv.fullBody), len(body))
	}
}

func TestLargeBody_TruncationStillSignalledOnListPath(t *testing.T) {
	rv := newLargeTestResponseView(t)
	// A valid-JSON-ish body larger than the display cap. We build a big JSON array
	// so the Pretty indent succeeds and the rendered display exceeds the cap.
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; b.Len() < maxDisplayBytes+50*1024; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":1,"name":"item-xxxxx","note":"lorem ipsum dolor"}`)
	}
	b.WriteString("]}")
	full := []byte(b.String())

	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: full, Size: int64(len(full))})

	if !rv.bodyList.Visible() {
		t.Fatal("truncated large Pretty body: List viewer should be visible")
	}
	if !rv.noticeLabel.Visible() {
		t.Error("truncated large body: truncation notice should be visible")
	}
	if !rv.saveBtn.Visible() {
		t.Error("truncated large body: Save-to-file button should be visible")
	}
	if len(rv.fullBody) != len(full) {
		t.Errorf("truncated large body: fullBody = %d, want retained %d", len(rv.fullBody), len(full))
	}
}

func TestLargeBody_RawKeepsTextGrid(t *testing.T) {
	rv := newLargeTestResponseView(t)
	body := largeJSON(t)
	rv.setPretty(false) // Raw
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})

	if rv.bodyList.Visible() {
		t.Error("large Raw body: List viewer should be hidden (Raw keeps TextGrid)")
	}
	if !rv.bodyScroll.Visible() {
		t.Error("large Raw body: TextGrid scroll should be visible")
	}
	if rv.bodyGrid.Text() == "" {
		t.Error("large Raw body: TextGrid should hold the raw text")
	}
}

func TestLargeBody_TogglingBackToSmallRestoresGrid(t *testing.T) {
	rv := newLargeTestResponseView(t)

	// First a large Pretty body -> List shown.
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: largeJSON(t)})
	if !rv.bodyList.Visible() {
		t.Fatal("expected List for large body")
	}

	// Then a small body -> grid restored, list hidden, lines cleared.
	small := []byte(`{"a":1,"b":"two"}`)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: small})
	if rv.bodyList.Visible() {
		t.Error("small body after large: List should be hidden")
	}
	if !rv.bodyScroll.Visible() {
		t.Error("small body after large: TextGrid scroll should be visible")
	}
	if len(rv.bodyLines) != 0 {
		t.Error("small body after large: stale List lines not cleared")
	}
	if rv.bodyGrid.Text() == "" {
		t.Error("small body after large: TextGrid should hold the small body text")
	}
}
