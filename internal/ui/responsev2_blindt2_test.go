package ui

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// Blind Test author #2 — independent verification of the v2 "Dark Pro" RESPONSE
// layout derived from assets/design/mockup-v2.png + the read-only-response rule.
//
// Expectations from the mockup/brief:
//   - A small valid JSON body renders via the coloured TextGrid (bodyGrid shown
//     through bodyScroll, List hidden) and the response HEADERS appear in a left
//     column with the keys coloured cyan.
//   - A Pretty body whose rendered text is >= largeBodyThreshold renders via the
//     virtualized widget.List (bodyList shown, bodyScroll hidden) — the perf path.
//   - A >256 KB body keeps the truncation notice + Save button and retains the
//     full body in memory (the read-only-response rule).
//   - The status pill colour tracks statusColor(code): 2xx/4xx/5xx classes, and
//     the status text reads e.g. "200 OK".
//   - No panic across small / large / error / pending states and Pretty<->Raw.

// bt2View spins up a fresh response view on the Fyne test driver.
func bt2View(t *testing.T) *responseView {
	t.Helper()
	test.NewApp()
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	return newResponseView(w)
}

// bt2HeadersText returns the plain text of the left-column headers TextGrid.
func bt2HeadersText(rv *responseView) string {
	return rv.headersGrid.Text()
}

// bt2KeySpanIsCyan reports whether every cell of the first `runes` cells on row
// `row` of the headers grid carries the cyan key style. This is the mockup-v2
// `.rh .k` contract for response-header keys.
func bt2KeySpanIsCyan(rv *responseView, row, runes int) bool {
	if row >= len(rv.headersGrid.Rows) {
		return false
	}
	cells := rv.headersGrid.Rows[row].Cells
	if runes > len(cells) {
		return false
	}
	for c := 0; c < runes; c++ {
		if cells[c].Style != styleRespHeaderKey {
			return false
		}
	}
	return true
}

// --- Small valid JSON: coloured TextGrid path + headers column -------------

func TestBT2_Response_SmallJSON_UsesColouredGridAndShowsHeaders(t *testing.T) {
	rv := bt2View(t)

	headers := []model.Param{
		{Key: "Content-Type", Value: "application/json"},
		{Key: "Content-Length", Value: "248"},
		{Key: "Server", Value: "yon-test/1.0"},
	}
	body := []byte(`{"args":{"page":"1","q":"hello"},"count":3,"ok":true}`)

	rv.setResponse(model.Response{
		Status:     200,
		StatusText: "OK",
		Headers:    headers,
		Body:       body,
		Size:       int64(len(body)),
	})

	// Body viewer: coloured TextGrid path -> bodyScroll (the grid) visible, the
	// virtualized List hidden, and no stale list lines.
	if !rv.bodyScroll.Visible() {
		t.Error("small JSON: bodyScroll (TextGrid) should be visible")
	}
	if rv.bodyList.Visible() {
		t.Error("small JSON: virtualized List should be hidden on the small path")
	}
	if len(rv.bodyLines) != 0 {
		t.Errorf("small JSON: bodyLines should be empty on grid path, got %d", len(rv.bodyLines))
	}

	// The coloured path builds styled rows directly via buildJSONRows; assert the
	// grid actually holds the JSON content (a key token must be present).
	gridText := rv.bodyGrid.Text()
	if !strings.Contains(gridText, "count") || !strings.Contains(gridText, "hello") {
		t.Errorf("small JSON: TextGrid text missing body content; got %q", gridText)
	}

	// At least one cell must be coloured (syntax highlight) — proves the coloured
	// path (buildJSONRows) ran rather than a plain SetText.
	coloured := false
	for _, row := range rv.bodyGrid.Rows {
		for _, cell := range row.Cells {
			if cell.Style != nil {
				coloured = true
				break
			}
		}
		if coloured {
			break
		}
	}
	if !coloured {
		t.Error("small valid JSON: expected at least one coloured cell (buildJSONRows path)")
	}

	// Headers column: every header key+value must be present in the left column.
	ht := bt2HeadersText(rv)
	for _, h := range headers {
		if !strings.Contains(ht, h.Key) {
			t.Errorf("headers column missing key %q; got %q", h.Key, ht)
		}
		if !strings.Contains(ht, h.Value) {
			t.Errorf("headers column missing value %q; got %q", h.Value, ht)
		}
	}
}

func TestBT2_Headers_KeysColouredCyanAndSorted(t *testing.T) {
	rv := bt2View(t)
	// Provide headers out of order; renderHeaders sorts by key, so the cyan key
	// span check must use the sorted order: Cache-Control, Content-Type, Server.
	rv.setResponse(model.Response{
		Status:     200,
		StatusText: "OK",
		Headers: []model.Param{
			{Key: "Server", Value: "yon-test/1.0"},
			{Key: "Cache-Control", Value: "no-store"},
			{Key: "Content-Type", Value: "application/json"},
		},
		Body: []byte(`{}`),
	})

	want := []struct {
		key   string
		runes int
	}{
		{"Cache-Control", len("Cache-Control")},
		{"Content-Type", len("Content-Type")},
		{"Server", len("Server")},
	}
	for row, w := range want {
		if !bt2KeySpanIsCyan(rv, row, w.runes) {
			t.Errorf("row %d (%s): header key span not cyan-styled", row, w.key)
		}
	}

	// The value portion (after ": ") must NOT carry the key style — proves the
	// colour is scoped to keys, not the whole row.
	row0 := rv.headersGrid.Rows[0].Cells
	valIdx := len("Cache-Control: ") // first value char index for "no-store"
	if valIdx < len(row0) && row0[valIdx].Style == styleRespHeaderKey {
		t.Error("header value should not carry the cyan key style")
	}
}

// --- Large Pretty body: virtualized List path (perf) -----------------------

// bt2LargePrettyBody returns a single valid JSON document whose pretty-printed
// form is comfortably above largeBodyThreshold but below the 256 KB cap, so the
// Pretty path takes the List without truncation.
func bt2LargePrettyBody() []byte {
	var b strings.Builder
	b.WriteString(`{"rows":[`)
	for i := 0; b.Len() < largeBodyThreshold+8*1024; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":12345,"name":"alpha","flag":true}`)
	}
	b.WriteString("]}")
	return []byte(b.String())
}

func TestBT2_Response_LargePretty_UsesVirtualizedList(t *testing.T) {
	rv := bt2View(t)
	body := bt2LargePrettyBody()

	// Sanity: the pretty-printed form must actually cross the threshold, else the
	// test would not exercise the perf path (guards against a too-small fixture).
	if pp, ok := prettyJSON(body); !ok || len(pp) < largeBodyThreshold {
		t.Fatalf("fixture pretty size = %d (ok=%v), want >= %d", len(pp), ok, largeBodyThreshold)
	}

	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})

	if !rv.bodyList.Visible() {
		t.Error("large Pretty body: virtualized List should be visible (perf path)")
	}
	if rv.bodyScroll.Visible() {
		t.Error("large Pretty body: bodyScroll (TextGrid) should be hidden")
	}
	if len(rv.bodyLines) == 0 {
		t.Error("large Pretty body: bodyLines should be populated for the List")
	}
	if got := rv.bodyList.Length(); got != len(rv.bodyLines) {
		t.Errorf("List.Length()=%d, want len(bodyLines)=%d", got, len(rv.bodyLines))
	}
	// Perf-path guard: the expensive coloured grid must be emptied, not stale.
	if rv.bodyGrid.Text() != "" {
		t.Error("large Pretty body: TextGrid should be cleared on the List path")
	}
	// Full body retained for Save.
	if len(rv.fullBody) != len(body) {
		t.Errorf("fullBody=%d, want retained %d", len(rv.fullBody), len(body))
	}
}

// Raw mode of a large body keeps the exact-bytes TextGrid (mockup Raw toggle).
func TestBT2_Response_LargeRaw_KeepsTextGrid(t *testing.T) {
	rv := bt2View(t)
	body := bt2LargePrettyBody()
	rv.setPretty(false) // Raw
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body})

	if rv.bodyList.Visible() {
		t.Error("large Raw body: List should be hidden")
	}
	if !rv.bodyScroll.Visible() {
		t.Error("large Raw body: bodyScroll (TextGrid) should be visible")
	}
	if rv.bodyGrid.Text() == "" {
		t.Error("large Raw body: TextGrid should hold the raw bytes")
	}
}

// --- >256 KB body: truncation notice + Save + full body retained -----------

func TestBT2_Truncation_OverCapKeepsNoticeSaveAndFullBody(t *testing.T) {
	rv := bt2View(t)

	// Build a valid JSON array that pretty-prints well past the 256 KB cap.
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; b.Len() < maxDisplayBytes+64*1024; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":1,"name":"item-xxxxx","note":"lorem ipsum dolor sit"}`)
	}
	b.WriteString("]}")
	full := []byte(b.String())
	if len(full) <= maxDisplayBytes {
		t.Fatalf("fixture only %d bytes, need > %d to exercise truncation", len(full), maxDisplayBytes)
	}

	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: full, Size: int64(len(full))})

	if !rv.noticeLabel.Visible() {
		t.Error("over-cap body: truncation notice should be visible")
	}
	if !strings.Contains(rv.noticeLabel.Text, "truncated") {
		t.Errorf("notice text should mention truncation; got %q", rv.noticeLabel.Text)
	}
	if !rv.saveBtn.Visible() {
		t.Error("over-cap body: Save-to-file button should be visible")
	}
	// CRITICAL the read-only-response rule guarantee: full body retained even though display is capped.
	if len(rv.fullBody) != len(full) {
		t.Errorf("over-cap body: fullBody=%d, want full %d retained", len(rv.fullBody), len(full))
	}
}

// A small body must NOT show the truncation notice or Save button.
func TestBT2_Truncation_SmallBodyHasNoNotice(t *testing.T) {
	rv := bt2View(t)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: []byte(`{"ok":true}`)})
	if rv.noticeLabel.Visible() {
		t.Error("small body: truncation notice should be hidden")
	}
	// Save Output As… is available for any response, truncated or not.
	if !rv.saveBtn.Visible() {
		t.Error("Save Output As… should be available for any response")
	}
}

// --- Status pill: colour tracks statusColor(code); text reads "<code> <text>"

func TestBT2_StatusPill_ColourTracksStatusColorByClass(t *testing.T) {
	cases := []struct {
		code int
		text string
	}{
		{200, "OK"},
		{201, "Created"},
		{301, "Moved Permanently"},
		{404, "Not Found"},
		{500, "Internal Server Error"},
	}
	for _, tc := range cases {
		rv := bt2View(t)
		rv.setResponse(model.Response{Status: tc.code, StatusText: tc.text, Body: []byte(`{}`)})

		want := statusColor(tc.code)
		// Status text: "<code> <statusText>" e.g. "200 OK".
		wantText := strings.TrimSpace(strings.Join([]string{itoa(tc.code), tc.text}, " "))
		if rv.statusLabel.Text != wantText {
			t.Errorf("%d: status text=%q, want %q", tc.code, rv.statusLabel.Text, wantText)
		}
		// Pill text colour and dot use the full class colour.
		if !colorsEqual(rv.statusLabel.Color, want) {
			t.Errorf("%d: pill text colour=%v, want statusColor()=%v", tc.code, rv.statusLabel.Color, want)
		}
		if !colorsEqual(rv.statusDot.FillColor, want) {
			t.Errorf("%d: pill dot colour=%v, want statusColor()=%v", tc.code, rv.statusDot.FillColor, want)
		}
		// Pill background is the same hue at low alpha (a tinted chip). The bg is
		// stored as a color.NRGBA carrying the raw class R/G/B with a reduced alpha;
		// compare the un-premultiplied NRGBA components, not RGBA() (which would
		// premultiply the low-alpha tint and hide the hue).
		bg, ok := rv.statusPillBG.FillColor.(color.NRGBA)
		if !ok {
			t.Fatalf("%d: pill bg FillColor is %T, want color.NRGBA tint", tc.code, rv.statusPillBG.FillColor)
		}
		wr8, wg8, wb8 := classRGB8(want)
		if bg.R != wr8 || bg.G != wg8 || bg.B != wb8 {
			t.Errorf("%d: pill bg hue=(%d,%d,%d), want class hue (%d,%d,%d)", tc.code,
				bg.R, bg.G, bg.B, wr8, wg8, wb8)
		}
		if bg.A >= 0xff {
			t.Errorf("%d: pill bg should be a tint (alpha < full), got alpha %d", tc.code, bg.A)
		}
	}
}

// The three required classes must map to DISTINCT colours (2xx != 4xx != 5xx),
// and 2xx green / 4xx orange / 5xx red specifically.
func TestBT2_StatusColor_ClassesAreDistinct(t *testing.T) {
	c2 := statusColor(200)
	c4 := statusColor(404)
	c5 := statusColor(500)
	if colorsEqual(c2, c4) || colorsEqual(c2, c5) || colorsEqual(c4, c5) {
		t.Errorf("status class colours not distinct: 2xx=%v 4xx=%v 5xx=%v", c2, c4, c5)
	}
	if !colorsEqual(c2, colorStatus2xx) {
		t.Errorf("200 colour=%v, want 2xx %v", c2, colorStatus2xx)
	}
	if !colorsEqual(c4, colorStatus4xx) {
		t.Errorf("404 colour=%v, want 4xx %v", c4, colorStatus4xx)
	}
	if !colorsEqual(c5, colorStatus5xx) {
		t.Errorf("500 colour=%v, want 5xx %v", c5, colorStatus5xx)
	}
}

// --- No-panic across all states and toggles --------------------------------

func TestBT2_Pane_NoPanicAcrossStatesAndToggles(t *testing.T) {
	rv := bt2View(t)

	// pending
	rv.setPending()
	// error
	rv.setError(errString("boom: connection refused"))
	// small ok
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: []byte(`{"a":1}`)})
	// toggle Raw then Pretty on a small body
	rv.setPretty(false)
	rv.setPretty(true)
	// large pretty
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: bt2LargePrettyBody()})
	// toggle on the large body (Pretty->Raw->Pretty exercises List<->Grid swap)
	rv.setPretty(false)
	rv.setPretty(true)
	// non-JSON small body in Pretty mode (prettyJSON fails -> plain SetText)
	rv.setResponse(model.Response{Status: 502, StatusText: "Bad Gateway", Body: []byte("not json <html>")})
	// back to pending then error again
	rv.setPending()
	rv.setError(errString("timeout"))

	// If we got here without a panic, the state machine is sound. Sanity: the
	// final state is the error pill (red) with no body viewers populated.
	if rv.statusLabel.Text != "Error" {
		t.Errorf("final state status text = %q, want %q", rv.statusLabel.Text, "Error")
	}
	if rv.fullBody != nil {
		t.Error("after setError, fullBody should be nil")
	}
}

// Empty-body / nil-body response must not panic and must clear the viewers.
func TestBT2_Response_EmptyBodyClearsViewers(t *testing.T) {
	rv := bt2View(t)
	rv.setResponse(model.Response{Status: 204, StatusText: "No Content", Body: nil})
	if rv.bodyList.Visible() {
		t.Error("nil body: List should not be visible")
	}
	if rv.bodyGrid.Text() != "" {
		t.Errorf("nil body: grid should be empty, got %q", rv.bodyGrid.Text())
	}
}

// --- tiny local helpers (kept distinct from other test files) --------------

// itoa avoids importing strconv just for the status-text assertion.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// classRGB8 returns the 8-bit R/G/B of a status-class colour. The class colours
// are opaque (alpha 0xff), so RGBA()>>8 equals the source component for them.
func classRGB8(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

// errString is a trivial error for the no-panic state walk.
type errString string

func (e errString) Error() string { return string(e) }
