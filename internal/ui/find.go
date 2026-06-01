package ui

import (
	"image/color"
	"sort"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Find / text search: a reusable bar + a TextGrid-backed search engine that
// highlights case-insensitive matches and scrolls to the current one. Cmd+F /
// Ctrl+F opens it, Esc closes it.

var (
	findMatchBG   = color.NRGBA{R: 0xF2, G: 0xB4, B: 0x41, A: 0x55} // amber — all matches
	findCurrentBG = color.NRGBA{R: 0xF2, G: 0xB4, B: 0x41, A: 0xCC} // stronger — current match
)

// findRuneOffsets returns the rune-index start of each case-insensitive,
// single-line occurrence of query in text. Matches do not overlap.
func findRuneOffsets(text, query string) []int {
	if query == "" {
		return nil
	}
	tr := []rune(text)
	qr := []rune(query)
	for i := range qr {
		qr[i] = unicode.ToLower(qr[i])
	}
	var offs []int
	for i := 0; i+len(qr) <= len(tr); i++ {
		ok := true
		for j := 0; j < len(qr); j++ {
			if unicode.ToLower(tr[i+j]) != qr[j] {
				ok = false
				break
			}
		}
		if ok {
			offs = append(offs, i)
			i += len(qr) - 1 // non-overlapping
		}
	}
	return offs
}

// lineStarts returns the rune offset at which each line of text begins.
func lineStarts(text string) []int {
	starts := []int{0}
	for i, r := range []rune(text) {
		if r == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// rowColOf maps a rune offset to its (row, col) given precomputed line starts.
func rowColOf(starts []int, off int) (row, col int) {
	row = sort.Search(len(starts), func(i int) bool { return starts[i] > off }) - 1
	if row < 0 {
		row = 0
	}
	return row, off - starts[row]
}

// gridSearch highlights query matches in a read-only TextGrid and scrolls the
// enclosing Scroll to the current match. clearFn restores the grid to its
// unhighlighted state (re-render) when search text changes or find closes.
type gridSearch struct {
	grid    *widget.TextGrid
	scroll  *container.Scroll
	text    string
	clearFn func()

	matches []int
	current int
	qlen    int
}

// bind points the search at a TextGrid whose plain text is text; clearFn must
// re-render the grid cleanly (without highlights).
func (g *gridSearch) bind(grid *widget.TextGrid, scroll *container.Scroll, text string, clearFn func()) {
	g.grid, g.scroll, g.text, g.clearFn = grid, scroll, text, clearFn
	g.matches, g.current, g.qlen = nil, 0, 0
}

// search (re)runs the query: clears old highlights, finds matches, paints them,
// and scrolls to the first. Returns (current 1-based, total).
func (g *gridSearch) search(query string) (int, int) {
	if g.clearFn != nil {
		g.clearFn()
	}
	g.qlen = len([]rune(query))
	g.matches = findRuneOffsets(g.text, query)
	g.current = 0
	if len(g.matches) == 0 {
		g.grid.Refresh()
		return 0, 0
	}
	g.paint()
	return g.current + 1, len(g.matches)
}

// move advances the current match by delta (wrapping) and repaints.
func (g *gridSearch) move(delta int) (int, int) {
	if len(g.matches) == 0 {
		return 0, 0
	}
	if g.clearFn != nil {
		g.clearFn()
	}
	g.current = (g.current + delta + len(g.matches)) % len(g.matches)
	g.paint()
	return g.current + 1, len(g.matches)
}

// clear removes all highlights (restores the clean render).
func (g *gridSearch) clear() {
	if g.clearFn != nil {
		g.clearFn()
		g.grid.Refresh()
	}
	g.matches = nil
}

// paint applies the match backgrounds (current match stronger) and scrolls to it,
// preserving each cell's existing foreground colour.
func (g *gridSearch) paint() {
	starts := lineStarts(g.text)
	curRow := 0
	for k, off := range g.matches {
		row, col := rowColOf(starts, off)
		if row >= len(g.grid.Rows) {
			continue
		}
		bg := findMatchBG
		if k == g.current {
			bg = findCurrentBG
			curRow = row
		}
		cells := g.grid.Rows[row].Cells
		for c := col; c < col+g.qlen && c < len(cells); c++ {
			g.grid.Rows[row].Cells[c].Style = &widget.CustomTextGridStyle{
				FGColor: fgColorOf(cells[c].Style),
				BGColor: bg,
			}
		}
	}
	g.grid.Refresh()
	g.scrollToRow(curRow, len(starts))
}

// scrollToRow scrolls the enclosing Scroll so the given row is comfortably in view.
func (g *gridSearch) scrollToRow(row, totalRows int) {
	if g.scroll == nil || totalRows <= 1 {
		return
	}
	h := g.grid.MinSize().Height
	y := h * float32(row) / float32(totalRows)
	// Centre the row a little above the middle of the viewport.
	y -= g.scroll.Size().Height / 3
	if y < 0 {
		y = 0
	}
	g.scroll.ScrollToOffset(fyne.NewPos(0, y))
}

// fgColorOf returns the foreground colour of a TextGrid cell style (nil → default).
func fgColorOf(s widget.TextGridStyle) color.Color {
	if c, ok := s.(*widget.CustomTextGridStyle); ok && c != nil {
		return c.FGColor
	}
	return nil
}

// shortcutEntry is an Entry that intercepts Cmd/Ctrl+F and Esc, so find opens and
// closes even while the Entry holds keyboard focus. A focused Shortcutable widget
// otherwise swallows canvas-level shortcuts before they reach AddShortcut.
type shortcutEntry struct {
	widget.Entry
	onFind func()
	onEsc  func()
}

func newShortcutEntry(multiLine bool) *shortcutEntry {
	e := &shortcutEntry{}
	e.MultiLine = multiLine
	if multiLine {
		e.Wrapping = fyne.TextWrapOff
	}
	e.ExtendBaseWidget(e)
	return e
}

func (e *shortcutEntry) TypedShortcut(s fyne.Shortcut) {
	if cs, ok := s.(*desktop.CustomShortcut); ok &&
		cs.KeyName == fyne.KeyF && cs.Modifier == fyne.KeyModifierShortcutDefault {
		if e.onFind != nil {
			e.onFind()
		}
		return
	}
	e.Entry.TypedShortcut(s)
}

func (e *shortcutEntry) TypedKey(ev *fyne.KeyEvent) {
	if ev.Name == fyne.KeyEscape && e.onEsc != nil {
		e.onEsc()
		return
	}
	e.Entry.TypedKey(ev)
}

// findBar is the find UI: a query field, an "n/m" counter, prev/next and close.
type findBar struct {
	container *fyne.Container
	query     *shortcutEntry
	count     *widget.Label
}

// newFindBar builds the bar. onChange fires on every query edit; onNext/onPrev
// navigate; onClose hides it (also bound to Esc inside the query field).
func newFindBar(onChange func(string), onNext, onPrev, onClose func()) *findBar {
	f := &findBar{}
	f.query = newShortcutEntry(false)
	f.query.onEsc = onClose
	f.query.SetPlaceHolder("Find…")
	f.query.OnChanged = onChange
	f.query.OnSubmitted = func(string) { onNext() }

	f.count = widget.NewLabel("")

	prev := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), onPrev)
	next := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), onNext)
	closeBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), onClose)
	for _, b := range []*widget.Button{prev, next, closeBtn} {
		b.Importance = widget.LowImportance
	}

	right := container.NewHBox(f.count, prev, next, closeBtn)
	f.container = container.NewBorder(nil, nil, nil, right, f.query)
	f.container.Hide()
	return f
}

// setCount updates the "cur/total" label (blank when there is no query).
func (f *findBar) setCount(cur, total int) {
	switch {
	case f.query.Text == "":
		f.count.SetText("")
	case total == 0:
		f.count.SetText("0/0")
	default:
		f.count.SetText(strconvItoa(cur) + "/" + strconvItoa(total))
	}
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// addFindShortcuts registers Cmd/Ctrl+F (open) and Esc (close) on a window canvas.
func addFindShortcuts(w fyne.Window, open, closeFind func()) {
	c := w.Canvas()
	c.AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault},
		func(fyne.Shortcut) { open() },
	)
	c.AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyEscape},
		func(fyne.Shortcut) { closeFind() },
	)
}
