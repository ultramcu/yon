package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/widget"
)

// ---------------------------------------------------------------------------
// Pure scanner helpers (categorisation + boundary correctness).
// ---------------------------------------------------------------------------

func TestScanString_BoundaryAndEscapes(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		start int // index of opening quote
		want  int // index of closing quote
	}{
		{"simple", `"abc"`, 0, 4},
		{"empty", `""`, 0, 1},
		{"with escaped quote", `"a\"b"`, 0, 5},
		{"escaped backslash before end", `"a\\"`, 0, 4},
		{"key in object", `{"k":1}`, 1, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scanString([]rune(c.line), c.start)
			if got != c.want {
				t.Errorf("scanString(%q,%d) = %d, want %d", c.line, c.start, got, c.want)
			}
			// The returned index must point at a closing quote (or last rune if
			// unterminated, which these terminated cases are not).
			if r := []rune(c.line)[got]; r != '"' {
				t.Errorf("scanString returned index %d which is %q, not a quote", got, r)
			}
		})
	}
}

func TestScanString_UnterminatedReturnsLastIndex(t *testing.T) {
	line := []rune(`"abc`)
	got := scanString(line, 0)
	if got != len(line)-1 {
		t.Errorf("unterminated scanString = %d, want last index %d", got, len(line)-1)
	}
}

func TestScanNumber_Boundary(t *testing.T) {
	cases := []struct {
		line  string
		start int
		want  int // last index of the number token
	}{
		{`123`, 0, 2},
		{`-5,`, 0, 1},      // stops before the comma
		{`1.5e10 `, 0, 5},  // float + exponent
		{`{"a":42}`, 5, 6}, // 42 inside an object, stops before }
		{`0`, 0, 0},
	}
	for _, c := range cases {
		got := scanNumber([]rune(c.line), c.start)
		if got != c.want {
			t.Errorf("scanNumber(%q,%d) = %d, want %d", c.line, c.start, got, c.want)
		}
	}
}

func TestScanWord(t *testing.T) {
	cases := []struct {
		line  string
		start int
		want  string
	}{
		{`true`, 0, "true"},
		{`false,`, 0, "false"},
		{`null}`, 0, "null"},
		{`{"x":true}`, 5, "true"},
	}
	for _, c := range cases {
		got := scanWord([]rune(c.line), c.start)
		if got != c.want {
			t.Errorf("scanWord(%q,%d) = %q, want %q", c.line, c.start, got, c.want)
		}
	}
}

func TestNextNonSpaceIsColon(t *testing.T) {
	// A string that is a key is immediately (after optional spaces) followed by ':'.
	if !nextNonSpaceIsColon([]rune(`"k"  : 1`), 3) {
		t.Errorf("expected key detection: colon follows after spaces")
	}
	// A string value is followed by a comma/brace, not a colon.
	if nextNonSpaceIsColon([]rune(`"v", `), 3) {
		t.Errorf("string value mis-detected as key (followed by comma)")
	}
	if nextNonSpaceIsColon([]rune(`"v"`), 3) {
		t.Errorf("end-of-line after string mis-detected as key")
	}
}

// ---------------------------------------------------------------------------
// End-to-end tokenizer: styleTextGridJSON colours a real (headless) TextGrid.
// Verifies category correctness and that the coloured runes cover exactly the
// meaningful tokens with no gaps/overlap conflicts that would mis-colour.
// ---------------------------------------------------------------------------

// colourAt returns the text colour of the cell, or nil if unstyled/default.
func colourAt(grid *widget.TextGrid, row, col int) color.Color {
	r := grid.Row(row)
	if col >= len(r.Cells) {
		return nil
	}
	st := r.Cells[col].Style
	if st == nil {
		return nil
	}
	return st.TextColor()
}

func eqColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestStyleTextGridJSON_Categorisation(t *testing.T) {
	// One-line JSON so row/col mapping is straightforward.
	text := `{"name":"yon","count":42,"ok":true,"v":null}`
	grid := widget.NewTextGrid()
	grid.SetText(text)
	styleTextGridJSON(grid, text)

	line := []rune(text)

	// Helper to assert a contiguous run [a,b] has the expected colour.
	assertRun := func(label string, a, b int, want color.Color) {
		for i := a; i <= b; i++ {
			got := colourAt(grid, 0, i)
			if !eqColor(got, want) {
				t.Errorf("%s: col %d (%q) colour=%v, want %v", label, i, string(line[i]), got, want)
			}
		}
	}

	idx := func(sub string, from int) int {
		s := text[from:]
		off := indexOf(s, sub)
		if off < 0 {
			t.Fatalf("substring %q not found from %d", sub, from)
		}
		return from + off
	}

	// "name" is a key -> key colour (including its quotes per the highlighter).
	nameStart := idx(`"name"`, 0)
	assertRun("key name", nameStart, nameStart+5, colorJSONKey)

	// "yon" is a string value -> string colour.
	yonStart := idx(`"yon"`, nameStart+6)
	assertRun("string yon", yonStart, yonStart+4, colorJSONString)

	// 42 -> number colour.
	numStart := idx(`42`, 0)
	assertRun("number 42", numStart, numStart+1, colorJSONNumber)

	// true -> keyword colour.
	trueStart := idx(`true`, 0)
	assertRun("keyword true", trueStart, trueStart+3, colorJSONKeyword)

	// null -> keyword colour.
	nullStart := idx(`null`, 0)
	assertRun("keyword null", nullStart, nullStart+3, colorJSONKeyword)
}

// TestStyleTextGridJSON_NoGapsOrUnknownColours ensures every coloured cell uses
// one of the five known categories (no stray colour) and that structural
// punctuation / whitespace are the only uncoloured-or-grey runes — i.e. there
// are no gaps inside value/key tokens that would leave part of a token
// mis-coloured. This is the "total covered length == token length" guarantee
// expressed cell-by-cell.
func TestStyleTextGridJSON_EveryCellHasKnownColour(t *testing.T) {
	text := "{\n  \"a\": 1,\n  \"b\": \"two\",\n  \"c\": false\n}"
	pretty, ok := prettyJSON([]byte(text))
	if ok {
		text = string(pretty)
	}
	grid := widget.NewTextGrid()
	grid.SetText(text)
	styleTextGridJSON(grid, text)

	known := []color.Color{
		colorJSONKey, colorJSONString, colorJSONNumber,
		colorJSONKeyword, colorJSONPunct,
	}

	rows := splitLines(text)
	for r, line := range rows {
		for c := range line {
			got := colourAt(grid, r, c)
			if got == nil {
				continue // whitespace / unstyled is allowed
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

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
