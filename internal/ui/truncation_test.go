package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// The truncation boundary is exercised through responseView (the read-only-response rule):
//   - body <= 256 KB  -> rendered whole, no truncation notice, save button hidden
//   - body  > 256 KB  -> only the head (maxDisplayBytes) shown, notice + save shown,
//                        full body retained for "Save to file".
//
// The behavioural tests below previously panicked on newResponseView under the
// headless Fyne test driver (prettyToggle.SetChecked fired OnChanged ->
// renderBody() while rv.bodyGrid was still nil). The source owner fixed the
// construction ordering (prettyToggle is now built with a nil handler,
// SetChecked'd, and OnChanged wired only after bodyGrid exists), so the real
// truncation behaviour is exercised here.

// TestTruncation_BoundaryConstant pins the documented 256 KB display cap from
// the read-only-response rule / responseview.go. This is the only truncation fact reachable without
// constructing the (currently panicking) responseView.
func TestTruncation_BoundaryConstant(t *testing.T) {
	if maxDisplayBytes != 256*1024 {
		t.Errorf("maxDisplayBytes = %d, want 256*1024 (the read-only-response rule 256 KB display cap)", maxDisplayBytes)
	}
}

func newTestResponseView(t *testing.T) *responseView {
	t.Helper()
	test.NewApp()
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	return newResponseView(w)
}

func TestTruncation_SmallBodyRenderedWhole(t *testing.T) {
	rv := newTestResponseView(t)
	body := []byte(strings.Repeat("x", maxDisplayBytes-1))
	rv.setPretty(false)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})

	if got := len(rv.bodyGrid.Text()); got != len(body) {
		t.Errorf("small body: rendered length = %d, want whole %d", got, len(body))
	}
	if rv.noticeLabel.Visible() {
		t.Errorf("small body: truncation notice should be hidden")
	}
	if rv.saveBtn.Visible() {
		t.Errorf("small body: Save-to-file button should be hidden")
	}
}

func TestTruncation_LargeBodyShowsHeadAndSignals(t *testing.T) {
	rv := newTestResponseView(t)
	full := []byte(strings.Repeat("z", maxDisplayBytes+5000))
	rv.setPretty(false)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: full, Size: int64(len(full))})

	if got := len(rv.bodyGrid.Text()); got != maxDisplayBytes {
		t.Errorf("large body: displayed length = %d, want head cap %d", got, maxDisplayBytes)
	}
	if !rv.noticeLabel.Visible() {
		t.Errorf("large body: truncation notice should be visible")
	}
	if !rv.saveBtn.Visible() {
		t.Errorf("large body: Save-to-file button should be visible")
	}
	if len(rv.fullBody) != len(full) {
		t.Errorf("large body: fullBody length = %d, want retained %d", len(rv.fullBody), len(full))
	}
}
