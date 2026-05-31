package ui

import (
	"bytes"
	"encoding/json"
	"image/color"
	"unicode/utf8"

	"fyne.io/fyne/v2/widget"
)

// JSON syntax-highlight colours. Tuned to read on both light and dark themes
// (mid-saturation hues that contrast against either background).
var (
	colorJSONKey     = color.NRGBA{R: 0x4f, G: 0x9c, B: 0xff, A: 0xff} // blue
	colorJSONString  = color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff} // green
	colorJSONNumber  = color.NRGBA{R: 0xe0, G: 0x80, B: 0x2b, A: 0xff} // orange
	colorJSONKeyword = color.NRGBA{R: 0xb0, G: 0x6c, B: 0xff, A: 0xff} // purple (true/false/null)
	colorJSONPunct   = color.NRGBA{R: 0x9a, G: 0x9a, B: 0x9a, A: 0xff} // grey
)

// Shared, immutable style objects — one per colour category — reused across
// every coloured cell instead of allocating a fresh *CustomTextGridStyle per
// token. TextGrid reads only FGColor from a style, so pointing many cells at the
// same singleton is safe and removes the dominant per-token allocation that the
// old SetStyleRange path incurred (one CustomTextGridStyle per styled range).
var (
	styleJSONKey     = &widget.CustomTextGridStyle{FGColor: colorJSONKey}
	styleJSONString  = &widget.CustomTextGridStyle{FGColor: colorJSONString}
	styleJSONNumber  = &widget.CustomTextGridStyle{FGColor: colorJSONNumber}
	styleJSONKeyword = &widget.CustomTextGridStyle{FGColor: colorJSONKeyword}
	styleJSONPunct   = &widget.CustomTextGridStyle{FGColor: colorJSONPunct}
)

// prettyJSON indents JSON data with two-space indentation. It returns the
// indented bytes and true on success, or (nil, false) when data is not valid
// JSON (so the caller can fall back to showing the raw body).
func prettyJSON(data []byte) ([]byte, bool) {
	if !json.Valid(data) {
		return nil, false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// buildJSONRows tokenizes already-indented JSON text in a SINGLE zero-allocation
// pass and constructs the []widget.TextGridRow directly: each cell's Rune is set
// and, for coloured tokens, its Style points at one of the five shared per-colour
// singletons. The caller assigns the result to grid.Rows and Refreshes — bypassing
// both TextGrid.SetText (which reparses every grapheme through uniseg) and a second
// styling walk over the grid. No *CustomTextGridStyle is allocated per token.
//
// Cell layout matches SetText for the JSON we display (space-indented, so no tab
// expansion): one cell per rune. Strings are distinguished into "key" (a string
// immediately followed, after optional whitespace, by ':') vs ordinary string
// values. true/false/null are keywords; digits/sign/exponent are numbers;
// structural punctuation is greyed. The scanner walks bytes with ASCII fast paths
// and tracks the rune/cell column so multi-byte UTF-8 inside strings stays aligned.
func buildJSONRows(text string) []widget.TextGridRow {
	// Count lines up front so we can allocate the rows slice exactly once.
	n := 1
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			n++
		}
	}
	rows := make([]widget.TextGridRow, 0, n)

	row := 0
	for {
		nl := indexByte(text, '\n')
		var line string
		last := false
		if nl < 0 {
			line = text
			last = true
		} else {
			line = text[:nl]
		}
		rows = append(rows, widget.TextGridRow{Cells: buildJSONCells(line)})
		row++
		if last {
			break
		}
		text = text[nl+1:]
	}
	return rows
}

// buildJSONCells builds one row's cells, classifying and styling in a single
// zero-allocation byte pass. It first lays down one cell per rune (Rune set,
// Style nil), then walks the line again assigning shared style pointers to the
// cells that belong to a coloured token. Cell columns are tracked explicitly so
// multi-byte runes (one cell each) stay aligned with the byte index.
func buildJSONCells(line string) []widget.TextGridCell {
	// Number of cells == number of runes in the line.
	cells := make([]widget.TextGridCell, 0, len(line))
	for i := 0; i < len(line); {
		r, sz := decodeRune(line[i:])
		cells = append(cells, widget.TextGridCell{Rune: r})
		i += sz
	}

	// set colours cells [startCol..endCol] (inclusive, in cell columns).
	set := func(startCol, endCol int, st widget.TextGridStyle) {
		if startCol < 0 {
			startCol = 0
		}
		if endCol >= len(cells) {
			endCol = len(cells) - 1
		}
		for c := startCol; c <= endCol; c++ {
			cells[c].Style = st
		}
	}

	col := 0 // cell (rune) column
	i := 0   // byte index into line
	for i < len(line) {
		b := line[i]
		switch {
		case b == '"':
			endByte, endCol := scanStringBytes(line, i, col)
			st := styleJSONString
			if nextNonSpaceIsColonBytes(line, endByte+1) {
				st = styleJSONKey
			}
			set(col, endCol, st)
			i = endByte + 1
			col = endCol + 1
		case b == 't' || b == 'f' || b == 'n':
			wlen := scanWordLen(line, i)
			word := line[i : i+wlen]
			if word == "true" || word == "false" || word == "null" {
				set(col, col+wlen-1, styleJSONKeyword)
				i += wlen
				col += wlen
			} else {
				i++
				col++
			}
		case b == '-' || (b >= '0' && b <= '9'):
			nlen := scanNumberLen(line, i)
			set(col, col+nlen-1, styleJSONNumber)
			i += nlen
			col += nlen
		case b == '{' || b == '}' || b == '[' || b == ']' || b == ':' || b == ',':
			set(col, col, styleJSONPunct)
			i++
			col++
		default:
			// Advance one rune; multi-byte runes occupy one cell.
			_, sz := decodeRune(line[i:])
			i += sz
			col++
		}
	}
	return cells
}

// styleTextGridJSON applies per-cell syntax colouring to a TextGrid that already
// holds indented JSON text. It re-tokenizes the grid's own text so the styling
// always matches what is displayed, writing the five shared style pointers
// straight into the grid's already-parsed cell slices in a single zero-allocation
// byte pass and refreshing the widget once. Retained for callers/tests that have
// already SetText'd the grid; buildJSONRows is the faster path when the rows can
// be assigned directly.
//
// Strings are distinguished into "key" (a string immediately followed, after
// optional whitespace, by ':') vs ordinary string values. true/false/null are
// keywords; digits/sign/exponent are numbers; structural punctuation is greyed.
func styleTextGridJSON(grid *widget.TextGrid, text string) {
	row := 0
	for len(text) > 0 {
		nl := indexByte(text, '\n')
		var line string
		if nl < 0 {
			line = text
			text = ""
		} else {
			line = text[:nl]
			text = text[nl+1:]
		}
		styleJSONLineFast(grid, row, line)
		row++
		if nl < 0 {
			break
		}
	}
	grid.Refresh()
}

// styleJSONLineFast classifies and colours one row of JSON, writing shared style
// pointers directly into grid.Rows[row].Cells. It mirrors the classification of
// buildJSONCells exactly but operates over a grid whose cells already exist.
func styleJSONLineFast(grid *widget.TextGrid, row int, line string) {
	if row < 0 || row >= len(grid.Rows) {
		return
	}
	cells := grid.Rows[row].Cells

	set := func(startCol, endCol int, st widget.TextGridStyle) {
		if startCol < 0 {
			startCol = 0
		}
		if endCol >= len(cells) {
			endCol = len(cells) - 1
		}
		for c := startCol; c <= endCol; c++ {
			cells[c].Style = st
		}
	}

	col := 0 // cell (rune) column
	i := 0   // byte index into line
	for i < len(line) {
		b := line[i]
		switch {
		case b == '"':
			endByte, endCol := scanStringBytes(line, i, col)
			st := styleJSONString
			if nextNonSpaceIsColonBytes(line, endByte+1) {
				st = styleJSONKey
			}
			set(col, endCol, st)
			i = endByte + 1
			col = endCol + 1
		case b == 't' || b == 'f' || b == 'n':
			wlen := scanWordLen(line, i)
			word := line[i : i+wlen]
			if word == "true" || word == "false" || word == "null" {
				set(col, col+wlen-1, styleJSONKeyword)
				i += wlen
				col += wlen
			} else {
				i++
				col++
			}
		case b == '-' || (b >= '0' && b <= '9'):
			nlen := scanNumberLen(line, i)
			set(col, col+nlen-1, styleJSONNumber)
			i += nlen
			col += nlen
		case b == '{' || b == '}' || b == '[' || b == ']' || b == ':' || b == ',':
			set(col, col, styleJSONPunct)
			i++
			col++
		default:
			_, sz := decodeRune(line[i:])
			i += sz
			col++
		}
	}
}

// indexByte returns the index of the first b in s, or -1. Plain byte loop so it
// allocates nothing (no []byte copy).
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// decodeRune returns the first rune of s and its byte width, handling the ASCII
// fast path inline so the common case allocates nothing and skips the unicode
// tables.
func decodeRune(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	if s[0] < 0x80 {
		return rune(s[0]), 1
	}
	r, sz := utf8.DecodeRuneInString(s)
	return r, sz
}

// scanStringBytes returns the byte index of the closing quote and the cell
// column of that closing quote, for the JSON string whose opening quote is at
// byte index startByte / cell column startCol. Mirrors scanString but tracks the
// rune column so multi-byte content inside the string keeps cells aligned. If
// unterminated it returns the line's last byte/cell.
func scanStringBytes(line string, startByte, startCol int) (endByte, endCol int) {
	i := startByte + 1
	col := startCol + 1
	for i < len(line) {
		b := line[i]
		switch {
		case b == '\\':
			// Skip the backslash and the (possibly multi-byte) escaped rune.
			i++
			col++
			if i < len(line) {
				_, sz := decodeRune(line[i:])
				i += sz
				col++
			}
			continue
		case b == '"':
			return i, col
		}
		if b < 0x80 {
			i++
		} else {
			_, sz := decodeRune(line[i:])
			i += sz
		}
		col++
	}
	// Unterminated: point at the last cell of the line.
	return len(line) - 1, col - 1
}

// scanNumberLen returns the byte length of the JSON number token starting at
// byte index start (numbers are ASCII, so length in bytes == length in cells).
func scanNumberLen(line string, start int) int {
	i := start
	if i < len(line) && line[i] == '-' {
		i++
	}
	for i < len(line) {
		r := line[i]
		if (r >= '0' && r <= '9') || r == '.' || r == 'e' || r == 'E' || r == '+' || r == '-' {
			i++
			continue
		}
		break
	}
	if i == start {
		return 1
	}
	return i - start
}

// scanWordLen returns the byte length of the run of lowercase ASCII letters
// starting at byte index start (length in bytes == length in cells).
func scanWordLen(line string, start int) int {
	i := start
	for i < len(line) {
		r := line[i]
		if r >= 'a' && r <= 'z' {
			i++
			continue
		}
		break
	}
	return i - start
}

// nextNonSpaceIsColonBytes reports whether the first non-space byte at/after the
// byte index idx is a colon. Spaces/tabs are ASCII so byte scanning suffices.
func nextNonSpaceIsColonBytes(line string, idx int) bool {
	for i := idx; i < len(line); i++ {
		if line[i] == ' ' || line[i] == '\t' {
			continue
		}
		return line[i] == ':'
	}
	return false
}

// scanString returns the index of the closing quote of a JSON string starting at
// the opening quote at start, honouring backslash escapes. If unterminated it
// returns the last index of the line. Retained for the rune-slice scanner tests.
func scanString(line []rune, start int) int {
	i := start + 1
	for i < len(line) {
		switch line[i] {
		case '\\':
			i += 2
			continue
		case '"':
			return i
		}
		i++
	}
	return len(line) - 1
}

// scanNumber returns the last index of a JSON number token starting at start.
// Retained for the rune-slice scanner tests.
func scanNumber(line []rune, start int) int {
	i := start
	if i < len(line) && line[i] == '-' {
		i++
	}
	for i < len(line) {
		r := line[i]
		if (r >= '0' && r <= '9') || r == '.' || r == 'e' || r == 'E' || r == '+' || r == '-' {
			i++
			continue
		}
		break
	}
	if i == start {
		return start
	}
	return i - 1
}

// scanWord reads a run of ASCII letters starting at start. Retained for tests.
func scanWord(line []rune, start int) string {
	i := start
	for i < len(line) {
		r := line[i]
		if r >= 'a' && r <= 'z' {
			i++
			continue
		}
		break
	}
	return string(line[start:i])
}

// nextNonSpaceIsColon reports whether the first non-space rune at/after idx is a
// colon (used to tell a JSON object key from a string value). Retained for tests.
func nextNonSpaceIsColon(line []rune, idx int) bool {
	for i := idx; i < len(line); i++ {
		if line[i] == ' ' || line[i] == '\t' {
			continue
		}
		return line[i] == ':'
	}
	return false
}

// splitLines splits text into []rune lines, dropping a trailing empty line so
// row indices line up with the TextGrid's parsed rows. Retained for tests.
func splitLines(text string) [][]rune {
	var out [][]rune
	var cur []rune
	for _, r := range text {
		if r == '\n' {
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, r)
	}
	out = append(out, cur)
	return out
}
