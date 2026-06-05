package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// Regression-focused blind tests (author B) for the change that moved the
// response HEADERS out of a left HSplit pane and into a TAB. The response area is
// now a segTabs with [Body, Headers]; Body is default and gets the full pane.
// These tests verify nothing else broke: the body perf architecture (TextGrid for
// small bodies, virtualized List for large), header population/clearing, the
// Pretty/Raw + Find behaviours, and that the pop-out still builds.

// --- helpers (unique names, no collisions with other authors' files) ---

func btbNewRV(t *testing.T) *responseView {
	t.Helper()
	test.NewApp()
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	return newResponseView(w)
}

// btbSegLabels returns the current sub-tab labels of a segTabs, in order.
func btbSegLabels(s *segTabs) []string {
	out := make([]string, 0, len(s.segs))
	for _, b := range s.segs {
		out = append(out, b.label)
	}
	return out
}

// btbLargeJSON returns a JSON document whose pretty-printed form is comfortably
// above largeBodyThreshold so the Body falls to the virtualized List path.
func btbLargeJSON() []byte {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; b.Len() < largeBodyThreshold*2; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":12345,"name":"widget","ok":true,"note":"hello"}`)
	}
	b.WriteString("]}")
	return []byte(b.String())
}

// =====================================================================
// SPEC 1: tab strip is exactly [Body, Headers], Body selected first.
// =====================================================================

func TestBTB_TabStripIsBodyThenHeaders_BodyDefault(t *testing.T) {
	rv := btbNewRV(t)

	if rv.respTabs == nil {
		t.Fatal("respTabs is nil — the response view should host a segTabs")
	}
	labels := btbSegLabels(rv.respTabs)
	// Issue #27 appended a third "Tests" tab after Headers, so the strip now leads
	// with Body then Headers (Body default) and may carry a trailing Tests tab.
	if len(labels) < 2 {
		t.Fatalf("tab strip has %d tabs %v, want at least 2 [Body, Headers]", len(labels), labels)
	}
	if labels[0] != "Body" {
		t.Errorf("first tab label = %q, want %q", labels[0], "Body")
	}
	// Headers may carry a trailing count; before any response it should be just
	// "Headers" (badge zero).
	if labels[1] != "Headers" && !strings.HasPrefix(labels[1], "Headers ") {
		t.Errorf("second tab label = %q, want %q (optionally with a count)", labels[1], "Headers")
	}
	if rv.respTabs.selected != 0 {
		t.Errorf("selected tab index = %d, want 0 (Body default)", rv.respTabs.selected)
	}
}

// =====================================================================
// SPEC 2: Body perf architecture intact — bodyStack holds both the scrolled
// TextGrid and the List; small body -> TextGrid, large body -> List.
// =====================================================================

func TestBTB_BodyStackHoldsBothViewers(t *testing.T) {
	rv := btbNewRV(t)

	if rv.bodyStack == nil {
		t.Fatal("bodyStack is nil — Body tab must hold the stacked viewers")
	}
	objs := rv.bodyStack.Objects
	if len(objs) != 2 {
		t.Fatalf("bodyStack has %d objects, want 2 (bodyScroll + bodyList)", len(objs))
	}
	foundScroll, foundList := false, false
	for _, o := range objs {
		if o == rv.bodyScroll {
			foundScroll = true
		}
		if o == rv.bodyList {
			foundList = true
		}
	}
	if !foundScroll {
		t.Error("bodyStack does not contain bodyScroll (the TextGrid scroller)")
	}
	if !foundList {
		t.Error("bodyStack does not contain bodyList (the large-body List)")
	}
	// And bodyStack must be the content of the Body (index 0) tab.
	if rv.respTabs.contents[0] != rv.bodyStack {
		t.Error("Body tab content is not bodyStack — full-pane body regressed")
	}
}

func TestBTB_SmallBodyRendersIntoTextGrid(t *testing.T) {
	rv := btbNewRV(t)
	small := []byte(`{"a":1,"b":"two","c":[1,2,3]}`)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: small})

	if rv.bodyList.Visible() {
		t.Error("small body: List viewer should be hidden")
	}
	if !rv.bodyScroll.Visible() {
		t.Error("small body: TextGrid scroller should be visible")
	}
	if rv.bodyGrid.Text() == "" {
		t.Error("small body: TextGrid should hold the rendered body text")
	}
	if len(rv.bodyLines) != 0 {
		t.Error("small body: bodyLines should be empty (List not used)")
	}
}

func TestBTB_LargeBodySwitchesToList(t *testing.T) {
	rv := btbNewRV(t)
	body := btbLargeJSON()
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})

	if !rv.bodyList.Visible() {
		t.Fatal("large Pretty body: List viewer should be visible")
	}
	if rv.bodyScroll.Visible() {
		t.Error("large Pretty body: TextGrid scroller should be hidden")
	}
	if len(rv.bodyLines) == 0 {
		t.Error("large Pretty body: bodyLines should be populated for the List")
	}
	if got := rv.bodyList.Length(); got != len(rv.bodyLines) {
		t.Errorf("List.Length() = %d, want %d (virtualization source mismatch)", got, len(rv.bodyLines))
	}
	if len(rv.fullBody) != len(body) {
		t.Errorf("fullBody = %d bytes, want full %d retained", len(rv.fullBody), len(body))
	}
}

// =====================================================================
// SPEC 3: Headers populate in the Headers tab; state changes clear them.
// =====================================================================

func TestBTB_HeadersPopulateInHeadersTab(t *testing.T) {
	rv := btbNewRV(t)
	headers := []model.Param{
		{Key: "Content-Type", Value: "application/json"},
		{Key: "X-Request-Id", Value: "abc-123"},
		{Key: "Cache-Control", Value: "no-cache"},
		{Key: "Server", Value: "yon-test"},
	}
	rv.setResponse(model.Response{
		Status: 200, StatusText: "OK",
		Headers: headers,
		Body:    []byte(`{"ok":true}`),
	})

	text := rv.headersGrid.Text()
	for _, h := range headers {
		if !strings.Contains(text, h.Key) {
			t.Errorf("headersGrid missing key %q; got:\n%s", h.Key, text)
		}
		if !strings.Contains(text, h.Value) {
			t.Errorf("headersGrid missing value %q; got:\n%s", h.Value, text)
		}
	}

	// The Headers tab is index 1; its content holds the headersGrid (in a scroll).
	if rv.respTabs.contents[1] == nil {
		t.Fatal("Headers tab content is nil")
	}

	// Badge should reflect the header count.
	want := tabBadge("Headers", len(headers))
	if rv.headersSeg.label != want {
		t.Errorf("Headers tab label = %q, want %q", rv.headersSeg.label, want)
	}
}

func TestBTB_PendingClearsHeaders(t *testing.T) {
	rv := btbNewRV(t)
	rv.setResponse(model.Response{
		Status: 200, StatusText: "OK",
		Headers: []model.Param{{Key: "Server", Value: "yon"}, {Key: "X-A", Value: "1"}},
		Body:    []byte(`{}`),
	})
	if rv.headersGrid.Text() == "" {
		t.Fatal("precondition: headers should be populated after a response")
	}

	rv.setPending() // must not panic
	if rv.headersGrid.Text() != "" {
		t.Errorf("setPending: stale headers remain: %q", rv.headersGrid.Text())
	}
	if rv.headersSeg.label != "Headers" {
		t.Errorf("setPending: Headers badge = %q, want reset to %q", rv.headersSeg.label, "Headers")
	}
}

func TestBTB_ErrorClearsHeaders(t *testing.T) {
	rv := btbNewRV(t)
	rv.setResponse(model.Response{
		Status: 200, StatusText: "OK",
		Headers: []model.Param{{Key: "Server", Value: "yon"}},
		Body:    []byte(`{}`),
	})
	rv.setError(errTestBTB("boom")) // must not panic
	if rv.headersGrid.Text() != "" {
		t.Errorf("setError: stale headers remain: %q", rv.headersGrid.Text())
	}
	if rv.headersSeg.label != "Headers" {
		t.Errorf("setError: Headers badge = %q, want reset to %q", rv.headersSeg.label, "Headers")
	}
}

type errTestBTB string

func (e errTestBTB) Error() string { return string(e) }

// =====================================================================
// SPEC 4: Pretty/Raw re-renders; Find highlights body and surfaces Body tab.
// =====================================================================

func TestBTB_PrettyRawRerendersBody(t *testing.T) {
	rv := btbNewRV(t)
	// Compact JSON; Pretty should indent it (multi-line), Raw should show as-is.
	rv.setResponse(model.Response{
		Status: 200, StatusText: "OK",
		Headers: []model.Param{{Key: "Content-Type", Value: "application/json"}},
		Body:    []byte(`{"a":1,"b":2,"c":3}`),
	})
	prettyText := rv.bodyGrid.Text()
	if !strings.Contains(prettyText, "\n") {
		t.Errorf("Pretty body should be indented/multi-line, got: %q", prettyText)
	}

	rv.setPretty(false) // Raw
	rawText := rv.bodyGrid.Text()
	if strings.Contains(rawText, "\n") {
		t.Errorf("Raw body should be the single-line raw bytes, got: %q", rawText)
	}
	if rawText == prettyText {
		t.Error("Pretty/Raw toggle did not re-render the body (identical text)")
	}

	rv.setPretty(true) // back to Pretty
	if rv.bodyGrid.Text() != prettyText {
		t.Error("toggling back to Pretty did not restore the indented render")
	}
}

func TestBTB_FindHighlightsBodyAndSurfacesBodyTab(t *testing.T) {
	rv := btbNewRV(t)
	rv.setResponse(model.Response{
		Status: 200, StatusText: "OK",
		Headers: []model.Param{{Key: "Content-Type", Value: "application/json"}},
		Body:    []byte(`{"needle":"needle","other":"needle"}`),
	})

	// Switch to the Headers tab first, then invoke Find — it must surface Body.
	rv.respTabs.Select(1)
	if rv.respTabs.selected != 1 {
		t.Fatal("precondition: should be on Headers tab")
	}

	rv.openFind() // must not panic
	if rv.respTabs.selected != 0 {
		t.Errorf("openFind: selected tab = %d, want 0 (Find should surface Body)", rv.respTabs.selected)
	}
	if !rv.findActive {
		t.Error("openFind: findActive should be true")
	}
	if !rv.find.container.Visible() {
		t.Error("openFind: find bar should be visible")
	}

	// Drive the search and confirm matches are found in the body.
	c, total := rv.search.search("needle")
	if total < 2 {
		t.Errorf("find on body: total matches = %d, want >= 2 for 'needle'", total)
	}
	if c < 1 {
		t.Errorf("find on body: current match = %d, want >= 1", c)
	}

	rv.closeFind() // must not panic
	if rv.findActive {
		t.Error("closeFind: findActive should be false")
	}
	if rv.find.container.Visible() {
		t.Error("closeFind: find bar should be hidden")
	}
}

// =====================================================================
// SPEC 5: Pop-out still builds/shows without panicking.
// =====================================================================

func TestBTB_PopoutBuildsWithoutPanic(t *testing.T) {
	rv := btbNewRV(t)
	rv.setResponse(model.Response{
		Status: 200, StatusText: "OK",
		Headers: []model.Param{{Key: "Content-Type", Value: "application/json"}},
		Body:    []byte(`{"hello":"world","n":42}`),
	})
	rv.showPopout() // opens its own window; must not panic
}

func TestBTB_PopoutLargeBodyBuildsWithoutPanic(t *testing.T) {
	rv := btbNewRV(t)
	body := btbLargeJSON()
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})
	rv.showPopout() // large bodies are capped in the pop-out; must not panic
}
