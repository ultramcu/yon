package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// Blind Test author #1 (Dev-Rabbit). Independent tests for the virtualized
// large-Pretty-body viewer (widget.List path) added by the perf fix.
//
// EXPECTED behaviour is derived from SCOPE.md / CONTEXT.md / ADR-0001 and the
// documented design:
//   - Pretty mode, displayed text > largeBodyThreshold (64 KB) -> List viewer
//     (bodyList visible; bodyGrid/bodyScroll hidden); bodyLines == pretty lines.
//   - Small Pretty bodies and ALL Raw bodies keep the TextGrid.
//   - maxDisplayBytes (256 KB) truncation + notice + Save button + full-body
//     retention apply on BOTH paths.
//   - Read-only monospace Labels (ADR-0001).
//
// Pretty line count is computed independently via the documented prettyJSON()
// signature, NOT by reading the viewer's rendering logic.

func newBlindT1RV(t *testing.T) *responseView {
	t.Helper()
	test.NewApp()
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	return newResponseView(w)
}

// bigValidJSON returns a single valid JSON document whose pretty-printed form is
// well above largeBodyThreshold (so the Pretty path must take the List), and
// optionally above maxDisplayBytes when huge is true.
func bigValidJSON(t *testing.T, huge bool) []byte {
	t.Helper()
	target := largeBodyThreshold * 2
	if huge {
		target = maxDisplayBytes + 80*1024
	}
	var b strings.Builder
	b.WriteString(`{"rows":[`)
	for i := 0; b.Len() < target; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":12345,"name":"widget","ok":true,"tags":["a","b","c"]}`)
	}
	b.WriteString("]}")
	return []byte(b.String())
}

// expectedPrettyLineCount derives the number of lines the pretty viewer must
// hold, using only the documented prettyJSON() transform.
func expectedPrettyLineCount(t *testing.T, body []byte) int {
	t.Helper()
	pretty, ok := prettyJSON(body)
	if !ok {
		t.Fatalf("test fixture is not valid JSON; prettyJSON returned ok=false")
	}
	s := string(pretty)
	// A trailing newline should not add a phantom empty final line.
	s = strings.TrimRight(s, "\n")
	return strings.Count(s, "\n") + 1
}

// --- 1. Large valid JSON in Pretty -> List is the visible viewer, lines match ---

func TestBlindT1_LargePretty_ListIsVisibleViewer(t *testing.T) {
	rv := newBlindT1RV(t)
	body := bigValidJSON(t, false)

	// Pretty is the default. setResponse must route the large body to the List.
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})

	if !rv.bodyList.Visible() {
		t.Fatal("large Pretty body: bodyList (virtualized viewer) must be visible")
	}
	// The TextGrid is hidden on screen via its scroll parent (bodyScroll);
	// bodyGrid lives inside bodyScroll, so the scroll's visibility is the real
	// on-screen gate. (bodyGrid.Visible() reports only its own flag, not the
	// ancestor's, so it is not a reliable signal here.)
	if rv.bodyScroll.Visible() {
		t.Error("large Pretty body: bodyScroll (TextGrid container) must be hidden")
	}
}

func TestBlindT1_LargePretty_BodyLinesMatchPrettyText(t *testing.T) {
	rv := newBlindT1RV(t)
	body := bigValidJSON(t, false)
	want := expectedPrettyLineCount(t, body)

	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})

	if len(rv.bodyLines) != want {
		t.Errorf("bodyLines = %d lines, want pretty-text line count %d", len(rv.bodyLines), want)
	}
	// The List's virtualization source (Length) must agree with the slice.
	if got := rv.bodyList.Length(); got != len(rv.bodyLines) {
		t.Errorf("bodyList.Length() = %d, want len(bodyLines) %d", got, len(rv.bodyLines))
	}
	// Sanity: the lines must reproduce the pretty text exactly (read-only mirror).
	pretty, _ := prettyJSON(body)
	wantText := strings.TrimRight(string(pretty), "\n")
	if got := strings.Join(rv.bodyLines, "\n"); got != wantText {
		t.Errorf("joined bodyLines do not equal pretty text\n got len=%d\nwant len=%d", len(got), len(wantText))
	}
}

// --- 2. Large body over 256 KB on the List path -> notice + Save + full retain ---

func TestBlindT1_LargePretty_TruncationNoticeAndSave(t *testing.T) {
	rv := newBlindT1RV(t)
	full := bigValidJSON(t, true)
	if len(full) <= maxDisplayBytes {
		t.Fatalf("fixture too small: %d bytes, need > maxDisplayBytes %d", len(full), maxDisplayBytes)
	}

	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: full, Size: int64(len(full))})

	if !rv.bodyList.Visible() {
		t.Fatal("huge Pretty body: List viewer must be visible")
	}
	if !rv.noticeLabel.Visible() {
		t.Error("huge Pretty body: truncation notice must be visible on the List path")
	}
	if !rv.saveBtn.Visible() {
		t.Error("huge Pretty body: Save-to-file button must be visible on the List path")
	}
	if len(rv.fullBody) != len(full) {
		t.Errorf("huge Pretty body: fullBody = %d bytes, want COMPLETE %d", len(rv.fullBody), len(full))
	}
}

// --- 3. Raw mode on a large body -> TextGrid shown, List hidden ---

func TestBlindT1_LargeRaw_KeepsTextGrid(t *testing.T) {
	rv := newBlindT1RV(t)
	body := bigValidJSON(t, false)
	rv.setPretty(false) // Raw

	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})

	if rv.bodyList.Visible() {
		t.Error("large Raw body: List viewer must be hidden (Raw keeps TextGrid)")
	}
	if !rv.bodyScroll.Visible() {
		t.Error("large Raw body: bodyScroll (TextGrid) must be visible")
	}
	if rv.bodyGrid.Text() == "" {
		t.Error("large Raw body: TextGrid must hold the raw text")
	}
}

// --- 4. Toggling Pretty->Raw->Pretty on a large body ends in correct viewer ---

func TestBlindT1_LargeBody_ToggleCyclesViewer(t *testing.T) {
	rv := newBlindT1RV(t)
	body := bigValidJSON(t, false)

	// Start Pretty -> List.
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})
	if !rv.bodyList.Visible() || rv.bodyScroll.Visible() {
		t.Fatalf("step Pretty: want List visible & scroll hidden; got list=%v scroll=%v",
			rv.bodyList.Visible(), rv.bodyScroll.Visible())
	}

	// Toggle to Raw -> TextGrid, List hidden.
	rv.setPretty(false)
	if rv.bodyList.Visible() {
		t.Error("step Raw: List must be hidden")
	}
	if !rv.bodyScroll.Visible() {
		t.Error("step Raw: TextGrid scroll must be visible")
	}

	// Toggle back to Pretty -> List again, no stale TextGrid left visible.
	rv.setPretty(true)
	if !rv.bodyList.Visible() {
		t.Error("step Pretty-again: List must be visible")
	}
	if rv.bodyScroll.Visible() {
		t.Error("step Pretty-again: TextGrid scroll must be hidden (stale viewer)")
	}
	if len(rv.bodyLines) == 0 {
		t.Error("step Pretty-again: bodyLines must be repopulated")
	}
}

// --- 5. Large<->small swaps restore the right viewer with no stale state ---

func TestBlindT1_LargeToSmallColoured_RestoresGrid(t *testing.T) {
	rv := newBlindT1RV(t)

	// Large Pretty -> List.
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: bigValidJSON(t, false)})
	if !rv.bodyList.Visible() {
		t.Fatal("expected List for large Pretty body")
	}

	// Small coloured (valid JSON) body -> coloured TextGrid restored, List hidden.
	small := []byte(`{"a":1,"b":"two","c":[true,false,null]}`)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: small})

	if rv.bodyList.Visible() {
		t.Error("small coloured body: List must be hidden")
	}
	if !rv.bodyScroll.Visible() {
		t.Error("small coloured body: TextGrid scroll must be visible")
	}
	if len(rv.bodyLines) != 0 {
		t.Error("small coloured body: stale bodyLines must be cleared")
	}
	if rv.bodyGrid.Text() == "" {
		t.Error("small coloured body: TextGrid must hold the body text")
	}
}

func TestBlindT1_SmallToLarge_SwitchesToList(t *testing.T) {
	rv := newBlindT1RV(t)

	// Small Pretty body first -> TextGrid.
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: []byte(`{"x":1}`)})
	if rv.bodyList.Visible() {
		t.Fatal("small body should use TextGrid, not List")
	}
	if !rv.bodyScroll.Visible() {
		t.Fatal("small body: TextGrid scroll should be visible")
	}

	// Then a large Pretty body -> List shown, TextGrid hidden.
	big := bigValidJSON(t, false)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: big, Size: int64(len(big))})
	if !rv.bodyList.Visible() {
		t.Error("large body after small: List must be visible")
	}
	if rv.bodyScroll.Visible() {
		t.Error("large body after small: TextGrid scroll must be hidden")
	}
}

// --- 6. No panic constructing/rendering any of these paths ---

func TestBlindT1_NoPanicAcrossPaths(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic across viewer paths: %v", r)
		}
	}()
	rv := newBlindT1RV(t)

	// small pretty, small raw, large pretty, large raw, huge pretty, back to small.
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: []byte(`{"k":1}`)})
	rv.setPretty(false)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: []byte("plain text body")})
	rv.setPretty(true)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: bigValidJSON(t, false)})
	rv.setPretty(false)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: bigValidJSON(t, false)})
	rv.setPretty(true)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: bigValidJSON(t, true)})
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: []byte(`{"done":true}`)})
}
