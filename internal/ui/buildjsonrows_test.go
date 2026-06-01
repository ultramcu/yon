package ui

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// ---------------------------------------------------------------------------
// Blind Test author #2 — Dev-Rabbit team.
//
// Target: the new buildJSONRows fast path. Per the design note it must be
// VISUALLY IDENTICAL to the previous styleTextGridJSON colouring (just faster /
// fewer allocs): every cell that should be coloured carries the correct
// category colour, only the five known category colours appear, the rows
// reproduce the input text exactly, multi-byte runes stay aligned, and a small
// coloured body renders through the TextGrid path (not the List).
//
// These tests derive expectations from the documented design / the read-only-response rule and the
// package colour constants — not from reading buildJSONRows' implementation.
// ---------------------------------------------------------------------------

// rowColourAt returns the text colour of a cell in a buildJSONRows result, or
// nil if the cell is unstyled / out of range.
func rowColourAt(rows []widget.TextGridRow, row, col int) color.Color {
	if row < 0 || row >= len(rows) {
		return nil
	}
	cells := rows[row].Cells
	if col < 0 || col >= len(cells) {
		return nil
	}
	st := cells[col].Style
	if st == nil {
		return nil
	}
	return st.TextColor()
}

// TestBuildJSONRows_Categorisation asserts that specific cells of a
// representative pretty JSON carry the correct category colour: keys vs
// string-values vs numbers vs booleans/null vs punctuation.
func TestBuildJSONRows_Categorisation(t *testing.T) {
	// One line per token-of-interest keeps the row/col mapping trivial.
	text := `{"name":"yon","count":42,"ok":true,"v":null}`
	rows := buildJSONRows(text)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for single-line input, got %d", len(rows))
	}

	line := []rune(text)
	assertRun := func(label string, a, b int, want color.Color) {
		for i := a; i <= b; i++ {
			got := rowColourAt(rows, 0, i)
			if !eqColor(got, want) {
				t.Errorf("%s: col %d (%q) colour=%v, want %v", label, i, string(line[i]), got, want)
			}
		}
	}
	idx := func(sub string, from int) int {
		off := indexOf(text[from:], sub)
		if off < 0 {
			t.Fatalf("substring %q not found from %d", sub, from)
		}
		return from + off
	}

	// "name" is a key (followed by ':') -> key colour, quotes included.
	nameStart := idx(`"name"`, 0)
	assertRun("key name", nameStart, nameStart+5, colorJSONKey)

	// "yon" is a string value -> string colour.
	yonStart := idx(`"yon"`, nameStart+6)
	assertRun("string yon", yonStart, yonStart+4, colorJSONString)

	// "count" is a key -> key colour.
	countStart := idx(`"count"`, 0)
	assertRun("key count", countStart, countStart+6, colorJSONKey)

	// 42 -> number colour.
	numStart := idx(`42`, 0)
	assertRun("number 42", numStart, numStart+1, colorJSONNumber)

	// true -> keyword colour.
	trueStart := idx(`true`, 0)
	assertRun("keyword true", trueStart, trueStart+3, colorJSONKeyword)

	// null -> keyword colour.
	nullStart := idx(`null`, 0)
	assertRun("keyword null", nullStart, nullStart+3, colorJSONKeyword)

	// Structural punctuation: the leading '{' and a comma -> punct colour.
	if !eqColor(rowColourAt(rows, 0, 0), colorJSONPunct) {
		t.Errorf("opening brace col 0 colour=%v, want punct %v", rowColourAt(rows, 0, 0), colorJSONPunct)
	}
	commaIdx := idx(`,"count"`, 0) // the comma before "count"
	if !eqColor(rowColourAt(rows, 0, commaIdx), colorJSONPunct) {
		t.Errorf("comma col %d colour=%v, want punct %v", commaIdx, rowColourAt(rows, 0, commaIdx), colorJSONPunct)
	}
}

// TestBuildJSONRows_MatchesStyleTextGridJSON is the strongest "visually
// identical" guard: build the same pretty JSON two ways — the legacy
// SetText+styleTextGridJSON path and the new buildJSONRows path — and assert
// every cell's colour is identical. If the fast path mis-colours, mis-aligns,
// or drops a style relative to the proven legacy path, this fails.
func TestBuildJSONRows_MatchesStyleTextGridJSON(t *testing.T) {
	raw := `{"name":"yon","nested":{"count":42,"ratio":-1.5e3,"ok":true,"bad":false,"v":null},"list":[1,2,"three"]}`
	pretty, ok := prettyJSON([]byte(raw))
	if !ok {
		t.Fatalf("test fixture did not pretty-print")
	}
	text := string(pretty)

	// Legacy reference path.
	ref := widget.NewTextGrid()
	ref.SetText(text)
	styleTextGridJSON(ref, text)

	// New fast path.
	rows := buildJSONRows(text)

	refRows := splitLines(text)
	if len(rows) != len(refRows) {
		t.Fatalf("buildJSONRows produced %d rows, want %d lines", len(rows), len(refRows))
	}

	for r, lineRunes := range refRows {
		for c := range lineRunes {
			want := colourAt(ref, r, c)
			got := rowColourAt(rows, r, c)
			if !eqColor(got, want) {
				t.Errorf("row %d col %d (%q): buildJSONRows colour=%v, styleTextGridJSON colour=%v",
					r, c, string(lineRunes[c]), got, want)
			}
		}
	}
}

// TestBuildJSONRows_OnlyKnownColours asserts every styled cell uses one of the
// five known category colours (no stray colour). Whitespace / structural runes
// may be unstyled, which is allowed.
func TestBuildJSONRows_OnlyKnownColours(t *testing.T) {
	raw := "{\n  \"a\": 1,\n  \"b\": \"two\",\n  \"c\": false,\n  \"d\": null,\n  \"e\": [1, 2.5, -3]\n}"
	pretty, ok := prettyJSON([]byte(raw))
	if ok {
		raw = string(pretty)
	}
	rows := buildJSONRows(raw)

	known := []color.Color{
		colorJSONKey, colorJSONString, colorJSONNumber,
		colorJSONKeyword, colorJSONPunct,
	}
	lines := splitLines(raw)
	for r, line := range lines {
		for c := range line {
			got := rowColourAt(rows, r, c)
			if got == nil {
				continue
			}
			matched := false
			for _, k := range known {
				if eqColor(got, k) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("row %d col %d (%q): unknown colour %v (not one of the 5 categories)",
					r, c, string(line[c]), got)
			}
		}
	}
}

// TestBuildJSONRows_ReproducesText asserts the rows reproduce the input text
// exactly: concatenating each row's cell runes yields the corresponding line,
// and there is one row per line. This is the "no dropped/extra cell" guarantee.
func TestBuildJSONRows_ReproducesText(t *testing.T) {
	raw := `{"name":"yon","count":42,"nums":[1,2,3],"ok":true,"v":null,"s":"a b\tc"}`
	pretty, ok := prettyJSON([]byte(raw))
	if !ok {
		t.Fatalf("fixture did not pretty-print")
	}
	text := string(pretty)

	rows := buildJSONRows(text)
	lines := splitLines(text)
	if len(rows) != len(lines) {
		t.Fatalf("rows=%d, want one per line=%d", len(rows), len(lines))
	}
	for r, line := range lines {
		var b strings.Builder
		for _, cell := range rows[r].Cells {
			b.WriteRune(cell.Rune)
		}
		if got := b.String(); got != string(line) {
			t.Errorf("row %d text mismatch:\n got %q\nwant %q", r, got, string(line))
		}
		if len(rows[r].Cells) != len(line) {
			t.Errorf("row %d cell count=%d, want one per rune=%d", r, len(rows[r].Cells), len(line))
		}
	}
}

// TestBuildJSONRows_UTF8Alignment checks a JSON string value with multi-byte
// runes (Thai + emoji) keeps colour aligned to the right cells and that the
// build does not panic. Each rune of the multi-byte string value must be one
// cell carrying the string colour; the key before it must be the key colour.
func TestBuildJSONRows_UTF8Alignment(t *testing.T) {
	// "name" is a key, the value contains Thai "ค่า" (which itself is a base
	// consonant + combining tone marks = several runes) and an emoji.
	raw := `{"name":"ค่า 🎉 ok","n":7}`
	pretty, ok := prettyJSON([]byte(raw))
	if !ok {
		t.Fatalf("fixture did not pretty-print")
	}
	text := string(pretty)

	var rows []widget.TextGridRow
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("buildJSONRows panicked on multi-byte input: %v", p)
			}
		}()
		rows = buildJSONRows(text)
	}()

	// Single-line after pretty for this tiny object? Pretty-print indents, so map
	// by locating the value on whichever row it lands. Flatten to (row,col) via a
	// per-line scan so the test is robust to indentation.
	lines := splitLines(text)

	// Every cell must still reproduce its line exactly (alignment invariant).
	for r, line := range lines {
		if len(rows[r].Cells) != len(line) {
			t.Fatalf("row %d: cell count=%d != rune count=%d (UTF-8 misalignment)",
				r, len(rows[r].Cells), len(line))
		}
		for c, want := range line {
			if rows[r].Cells[c].Rune != want {
				t.Errorf("row %d col %d: rune=%q, want %q", r, c, rows[r].Cells[c].Rune, want)
			}
		}
	}

	// Locate the line that holds the value string and assert the whole quoted
	// value run (including the multi-byte runes) is the string colour, and that
	// the "name" key on that line is the key colour.
	found := false
	for r, line := range lines {
		ls := string(line)
		off := indexOf(ls, `"ค่า 🎉 ok"`)
		if off < 0 {
			continue
		}
		found = true
		// Convert the byte offset within the line into a rune column.
		valCol := len([]rune(ls[:off]))
		valRunes := []rune(`"ค่า 🎉 ok"`)
		for k := range valRunes {
			got := rowColourAt(rows, r, valCol+k)
			if !eqColor(got, colorJSONString) {
				t.Errorf("value run row %d col %d (%q): colour=%v, want string %v",
					r, valCol+k, string(valRunes[k]), got, colorJSONString)
			}
		}
		// The "name" key earlier on the same line must be key colour.
		koff := indexOf(ls, `"name"`)
		if koff >= 0 {
			kCol := len([]rune(ls[:koff]))
			for k := 0; k < len([]rune(`"name"`)); k++ {
				got := rowColourAt(rows, r, kCol+k)
				if !eqColor(got, colorJSONKey) {
					t.Errorf("key run row %d col %d: colour=%v, want key %v", r, kCol+k, got, colorJSONKey)
				}
			}
		}
	}
	if !found {
		t.Fatalf("multi-byte value string not found in pretty output:\n%s", text)
	}
}

// TestSmallColouredBody_RendersViaTextGrid checks that a small coloured (valid
// JSON) body below largeBodyThreshold renders through the TextGrid path: the
// TextGrid scroll is visible, the List is hidden, the grid holds the coloured
// rows, and at least one cell carries a category colour (i.e. the colour
// survived the buildJSONRows assignment, not a plain SetText).
func TestSmallColouredBody_RendersViaTextGrid(t *testing.T) {
	test.NewApp()
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	rv := newResponseView(w)

	body := []byte(`{"name":"yon","count":42,"ok":true,"v":null}`)
	// Sanity: this body's pretty form must stay below the large-body threshold.
	pretty, ok := prettyJSON(body)
	if !ok {
		t.Fatalf("fixture did not pretty-print")
	}
	if len(pretty) >= largeBodyThreshold {
		t.Fatalf("fixture too large: %d >= threshold %d", len(pretty), largeBodyThreshold)
	}

	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: body, Size: int64(len(body))})

	if !rv.bodyScroll.Visible() {
		t.Error("small coloured body: TextGrid scroll should be visible")
	}
	if rv.bodyList.Visible() {
		t.Error("small coloured body: List viewer should be hidden")
	}
	if len(rv.bodyLines) != 0 {
		t.Error("small coloured body: List lines should be empty")
	}

	// The grid must hold the pretty text and carry colour on at least one cell.
	if got := rv.bodyGrid.Text(); got != string(pretty) {
		t.Errorf("grid text mismatch:\n got %q\nwant %q", got, string(pretty))
	}
	coloured := false
	for r := 0; r < len(rv.bodyGrid.Rows); r++ {
		for _, cell := range rv.bodyGrid.Rows[r].Cells {
			if cell.Style != nil && cell.Style.TextColor() != nil {
				coloured = true
				break
			}
		}
		if coloured {
			break
		}
	}
	if !coloured {
		t.Error("small coloured body: no cell carried a colour — TextGrid path did not colourise")
	}

	// And the colour present must be a known category colour (not stray).
	known := map[color.Color]bool{
		colorJSONKey: true, colorJSONString: true, colorJSONNumber: true,
		colorJSONKeyword: true, colorJSONPunct: true,
	}
	for r := 0; r < len(rv.bodyGrid.Rows); r++ {
		for _, cell := range rv.bodyGrid.Rows[r].Cells {
			if cell.Style == nil {
				continue
			}
			c := cell.Style.TextColor()
			if c == nil {
				continue
			}
			ok := false
			for k := range known {
				if eqColor(c, k) {
					ok = true
					break
				}
			}
			if !ok {
				t.Errorf("grid cell %q carried unknown colour %v", string(cell.Rune), c)
			}
		}
	}
}
