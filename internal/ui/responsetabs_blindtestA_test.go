package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// blindA_containsObject reports whether target is reachable anywhere in the
// CanvasObject tree rooted at root (including via container children and widget
// renderers). Used to confirm a specific grid/stack lives inside a tab's content.
func blindA_containsObject(root, target fyne.CanvasObject, depth int) bool {
	if root == nil || depth > 40 {
		return false
	}
	if root == target {
		return true
	}
	switch o := root.(type) {
	case *fyne.Container:
		for _, c := range o.Objects {
			if blindA_containsObject(c, target, depth+1) {
				return true
			}
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(o).Objects() {
			if blindA_containsObject(c, target, depth+1) {
				return true
			}
		}
	}
	return false
}

// blindA_segLabels returns the (current) labels of every segment button.
func blindA_segLabels(s *segTabs) []string {
	out := make([]string, 0, len(s.segs))
	for _, b := range s.segs {
		out = append(out, b.label)
	}
	return out
}

// blindA_newView builds a fresh response view on a throwaway test app/window.
func blindA_newView(t *testing.T) *responseView {
	t.Helper()
	test.NewApp()
	return newResponseView(test.NewWindow(nil))
}

func blindA_sampleResponse() model.Response {
	return model.Response{
		Status:     200,
		StatusText: "OK",
		Headers: []model.Param{
			{Key: "Content-Type", Value: "application/json", Enabled: true},
			{Key: "X-Trace-Id", Value: "abc-123", Enabled: true},
			{Key: "Server", Value: "yon-test", Enabled: true},
		},
		Body: []byte(`{"hello":"world","n":42}`),
	}
}

// SPEC 1: the first two tabs are "Body" + "Headers" (in that order), with Body
// initially selected. Issue #27 appended a third "Tests" tab after Headers, so
// the count is now ≥2 and Body/Headers lead — the original Body-default,
// Headers-second intent is preserved.
func TestBlindA_Spec1_TwoTabsBodyDefault(t *testing.T) {
	rv := blindA_newView(t)
	tabs := rv.respTabs
	if tabs == nil {
		t.Fatal("rv.respTabs is nil")
	}
	labels := blindA_segLabels(tabs)
	if len(tabs.segs) < 2 {
		t.Fatalf("expected at least 2 tabs (Body, Headers), got %d: %v", len(tabs.segs), labels)
	}
	if labels[0] != "Body" {
		t.Errorf("tab 0 label = %q, want %q", labels[0], "Body")
	}
	if !strings.HasPrefix(labels[1], "Headers") {
		t.Errorf("tab 1 label = %q, want prefix %q", labels[1], "Headers")
	}
	if tabs.selected != 0 {
		t.Errorf("initially-selected tab index = %d, want 0 (Body)", tabs.selected)
	}
	if !tabs.segs[0].active {
		t.Error("Body segment is not marked active initially")
	}
	t.Logf("SPEC1 observed: labels=%v selected=%d", labels, tabs.selected)
}

// SPEC 2: Body tab content is/contains rv.bodyStack (and the perf stack holds
// both the scrolled TextGrid and the List).
func TestBlindA_Spec2_BodyTabHoldsBodyStack(t *testing.T) {
	rv := blindA_newView(t)
	bodyContent := rv.respTabs.contents[0]
	if rv.bodyStack == nil {
		t.Fatal("rv.bodyStack is nil")
	}
	if !blindA_containsObject(bodyContent, rv.bodyStack, 0) {
		t.Fatalf("Body tab content does not contain rv.bodyStack")
	}
	// Perf architecture intact: stack contains both viewers.
	if !blindA_containsObject(rv.bodyStack, rv.bodyScroll, 0) {
		t.Error("bodyStack does not contain the scrolled TextGrid (bodyScroll)")
	}
	if !blindA_containsObject(rv.bodyStack, rv.bodyList, 0) {
		t.Error("bodyStack does not contain the large-body List (bodyList)")
	}
	if !blindA_containsObject(rv.bodyScroll, rv.bodyGrid, 0) {
		t.Error("bodyScroll does not contain the bodyGrid TextGrid")
	}
	t.Log("SPEC2 observed: Body tab -> bodyStack -> {bodyScroll(TextGrid), bodyList}")
}

// SPEC 3: Headers tab content reaches rv.headersGrid.
func TestBlindA_Spec3_HeadersTabReachesHeadersGrid(t *testing.T) {
	rv := blindA_newView(t)
	headersContent := rv.respTabs.contents[1]
	if rv.headersGrid == nil {
		t.Fatal("rv.headersGrid is nil")
	}
	if !blindA_containsObject(headersContent, rv.headersGrid, 0) {
		t.Fatal("Headers tab content does not reach rv.headersGrid")
	}
	t.Log("SPEC3 observed: Headers tab content -> headersGrid reachable")
}

// SPEC 4: after setResponse, headers text in headersGrid, body text in body renderer.
func TestBlindA_Spec4_ResponseTextLandsInTabs(t *testing.T) {
	rv := blindA_newView(t)
	resp := blindA_sampleResponse()
	rv.setResponse(resp)

	headersText := rv.headersGrid.Text()
	for _, h := range resp.Headers {
		if !strings.Contains(headersText, h.Key) {
			t.Errorf("headersGrid missing header key %q; got:\n%s", h.Key, headersText)
		}
		if !strings.Contains(headersText, h.Value) {
			t.Errorf("headersGrid missing header value %q; got:\n%s", h.Value, headersText)
		}
	}

	bodyText := rv.bodyGrid.Text()
	// Pretty JSON reformats whitespace, so assert on a stable token.
	if !strings.Contains(bodyText, "hello") || !strings.Contains(bodyText, "world") {
		t.Errorf("bodyGrid missing body content; got:\n%s", bodyText)
	}

	// Header count badge appears on the Headers tab after a response.
	wantBadge := tabBadge("Headers", len(resp.Headers))
	if rv.headersSeg.label != wantBadge {
		t.Errorf("Headers tab label = %q, want %q", rv.headersSeg.label, wantBadge)
	}
	t.Logf("SPEC4 observed: headersGrid=%q bodyGrid=%q badge=%q",
		strings.ReplaceAll(headersText, "\n", "\\n"), bodyText, rv.headersSeg.label)
}

// SPEC 5: body controls reachable in the toolbar; openFind surfaces Body tab.
func TestBlindA_Spec5_ControlsReachableAndFindSurfacesBody(t *testing.T) {
	rv := blindA_newView(t)
	// Controls must exist (and not be inside the tab contents).
	for name, b := range map[string]fyne.CanvasObject{
		"prettyBtn": rv.prettyBtn,
		"rawBtn":    rv.rawBtn,
		"copyBtn":   rv.copyBtn,
		"saveBtn":   rv.saveBtn,
		"find":      rv.find.container,
	} {
		if b == nil {
			t.Errorf("control %s is nil", name)
		}
	}
	// Controls live in the header, not inside either tab's content.
	for i, c := range rv.respTabs.contents {
		if blindA_containsObject(c, rv.prettyBtn, 0) ||
			blindA_containsObject(c, rv.copyBtn, 0) ||
			blindA_containsObject(c, rv.find.container, 0) {
			t.Errorf("a body control leaked into tab content index %d", i)
		}
	}

	// Set a response, switch to Headers tab, then openFind must surface Body.
	rv.setResponse(blindA_sampleResponse())
	rv.respTabs.Select(1)
	if rv.respTabs.selected != 1 {
		t.Fatalf("precondition failed: expected Headers selected, got %d", rv.respTabs.selected)
	}
	rv.openFind()
	if rv.respTabs.selected != 0 {
		t.Errorf("after openFind, selected tab = %d, want 0 (Body)", rv.respTabs.selected)
	}
	if !rv.findActive {
		t.Error("openFind did not set findActive")
	}
	t.Logf("SPEC5 observed: controls present; openFind selected tab=%d", rv.respTabs.selected)
}
