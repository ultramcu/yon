package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// tabStrip is a custom "browser cards" tab bar that replaces Fyne's
// container.DocTabs for the request editors. It renders a horizontally
// scrolling row of clearly separated card tabs above the selected card's
// content, with the active card raised and tied into the content by a cyan
// accent strip. It owns the cards, the current selection, and swaps the
// content area to match the selection.
type tabStrip struct {
	// OnSelected fires after Select makes a card current (including the
	// neighbour promoted when the selected card is removed).
	OnSelected func(*tabCard)
	// OnClose fires when a card's close affordance ("×") is tapped. The strip
	// does NOT remove the card itself on close — the owner decides (so it can
	// cancel in-flight work and update bookkeeping, then call Remove).
	OnClose func(*tabCard)

	cards    []*tabCard
	selected *tabCard

	// row holds the card widgets in order; scroll wraps it for many tabs.
	row     *fyne.Container
	scroll  *container.Scroll
	content *fyne.Container // Stack: shows the selected card's content
	root    fyne.CanvasObject
}

// newTabStrip builds an empty strip ready for Append/Select.
func newTabStrip() *tabStrip {
	s := &tabStrip{}
	s.row = container.NewHBox()
	s.scroll = container.NewHScroll(s.row)
	s.content = container.NewStack()
	// Card row docked at the top, content filling the rest. The active card's
	// cyan top-accent connects visually into the content below it.
	s.root = container.NewBorder(s.scroll, nil, nil, nil, s.content)
	return s
}

// object returns the strip's root canvas object for the window to embed where
// the DocTabs used to live.
func (s *tabStrip) object() fyne.CanvasObject {
	return s.root
}

// Append adds a card to the end of the strip (does not change the selection).
func (s *tabStrip) Append(c *tabCard) {
	c.strip = s
	s.cards = append(s.cards, c)
	s.row.Add(c)
	s.row.Refresh()
}

// Remove drops a card from the strip. If the removed card was the current
// selection, a remaining neighbour (the next card, else the previous) is
// selected; when the last card is removed the selection becomes nil and the
// content area is cleared.
func (s *tabStrip) Remove(c *tabCard) {
	idx := -1
	for i, card := range s.cards {
		if card == c {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	wasSelected := s.selected == c
	s.cards = append(s.cards[:idx:idx], s.cards[idx+1:]...)
	s.row.Remove(c)
	s.row.Refresh()

	if !wasSelected {
		return
	}

	// The removed card was selected: promote a neighbour, or clear when empty.
	if len(s.cards) == 0 {
		s.selected = nil
		s.content.Objects = nil
		s.content.Refresh()
		return
	}
	next := idx
	if next >= len(s.cards) {
		next = len(s.cards) - 1
	}
	s.Select(s.cards[next])
}

// Select makes c the current card: it updates the active styling, swaps the
// content area to c's content, and fires OnSelected(c). Selecting a card not in
// the strip is a no-op.
func (s *tabStrip) Select(c *tabCard) {
	found := false
	for _, card := range s.cards {
		if card == c {
			found = true
			break
		}
	}
	if !found {
		return
	}

	prev := s.selected
	s.selected = c
	if prev != nil && prev != c {
		prev.setActive(false)
	}
	c.setActive(true)

	s.content.Objects = []fyne.CanvasObject{c.content}
	s.content.Refresh()
	s.scrollTo(c)

	if s.OnSelected != nil {
		s.OnSelected(c)
	}
}

// Selected returns the current card, or nil when the strip is empty.
func (s *tabStrip) Selected() *tabCard {
	return s.selected
}

// Cards returns the cards in on-screen (left-to-right) order. The returned
// slice is a copy; mutating it does not affect the strip.
func (s *tabStrip) Cards() []*tabCard {
	out := make([]*tabCard, len(s.cards))
	copy(out, s.cards)
	return out
}

// SelectIndex selects the card at position i in on-screen order; out-of-range
// indices are ignored.
func (s *tabStrip) SelectIndex(i int) {
	if i < 0 || i >= len(s.cards) {
		return
	}
	s.Select(s.cards[i])
}

// scrollTo nudges the horizontal scroll so c is visible (best-effort).
func (s *tabStrip) scrollTo(c *tabCard) {
	if s.scroll == nil {
		return
	}
	pos := c.Position()
	size := c.Size()
	visMin := s.scroll.Offset.X
	visMax := visMin + s.scroll.Size().Width
	switch {
	case pos.X < visMin:
		s.scroll.Offset.X = pos.X
	case pos.X+size.Width > visMax:
		s.scroll.Offset.X = pos.X + size.Width - s.scroll.Size().Width
	default:
		return
	}
	if s.scroll.Offset.X < 0 {
		s.scroll.Offset.X = 0
	}
	s.scroll.Refresh()
}

// fireClose is called by a card's close button so the strip relays OnClose.
func (s *tabStrip) fireClose(c *tabCard) {
	if s.OnClose != nil {
		s.OnClose(c)
	}
}

// fireSelect is called when a card body (not its ×) is tapped.
func (s *tabStrip) fireSelect(c *tabCard) {
	s.Select(c)
}

// ---- tab card ----

// tabCard is one browser-style tab: a coloured method chip, the request display
// name (ellipsis-clamped), a dirty "●" marker, and a tappable close "×". It is a
// custom widget so it can paint its own raised/flat card background and the cyan
// top-accent that ties the active card into the content below.
type tabCard struct {
	widget.BaseWidget

	// content is the editor shown when this card is selected.
	content fyne.CanvasObject

	// Text mirrors the old DocTab label ("● Name" when dirty, else "Name"),
	// kept so the rest of the window — and existing tests — can read the same
	// dirty-prefixed string.
	Text string

	method model.Method
	name   string
	dirty  bool
	active bool

	strip *tabStrip
}

// newTabCard builds a card whose body shows content when the card is selected.
func newTabCard(content fyne.CanvasObject) *tabCard {
	c := &tabCard{content: content}
	c.ExtendBaseWidget(c)
	return c
}

// setRequest re-binds the card to a request: its method (for the chip colour),
// its display name (ellipsis-clamped), and whether it has unsaved edits (shows a
// leading "●"). Updates the mirrored Text label and repaints.
func (c *tabCard) setRequest(method model.Method, name string, dirty bool) {
	c.method = method
	c.name = name
	c.dirty = dirty
	if name == "" {
		name = "Untitled"
		c.name = name
	}
	if dirty {
		c.Text = "● " + name
	} else {
		c.Text = name
	}
	c.Refresh()
}

// setActive marks the card raised (selected) or flat, repainting on change.
func (c *tabCard) setActive(active bool) {
	if c.active == active {
		return
	}
	c.active = active
	c.Refresh()
}

// CreateRenderer builds the card renderer (background, accent, chip, name,
// dirty dot, close ×).
func (c *tabCard) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = theme.InputRadiusSize()

	accent := canvas.NewRectangle(color.Transparent)

	chip := canvas.NewText("", methodColorSlate)
	// Bold (not Monospace+Bold): the test font driver has no bold-monospace face,
	// and bold alone reads well for a short method label like "POST".
	chip.TextStyle = fyne.TextStyle{Bold: true}
	chip.TextSize = theme.CaptionTextSize()

	name := canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
	name.TextSize = theme.CaptionTextSize()

	closeBtn := newTabClose(func() {
		if c.strip != nil {
			c.strip.fireClose(c)
		}
	})

	r := &tabCardRenderer{
		card:    c,
		bg:      bg,
		accent:  accent,
		chip:    chip,
		name:    name,
		close:   closeBtn,
		objects: []fyne.CanvasObject{bg, accent, chip, name, closeBtn},
	}
	r.Refresh()
	return r
}

// Tapped selects the card (clicking anywhere on the body, except the ×, which
// has its own tappable). Implements fyne.Tappable.
func (c *tabCard) Tapped(*fyne.PointEvent) {
	if c.strip != nil {
		c.strip.fireSelect(c)
	}
}

// tabCardRenderer lays out and paints a tab card.
type tabCardRenderer struct {
	card   *tabCard
	bg     *canvas.Rectangle
	accent *canvas.Rectangle
	chip   *canvas.Text
	name   *canvas.Text
	close  *tabClose

	objects []fyne.CanvasObject
}

const (
	tabCardPad       = 8  // inner horizontal padding
	tabCardGap       = 6  // gap between chip / name / close
	tabAccentH       = 3  // height of the cyan top-accent on the active card
	tabCardMinW      = 96 // floor so short names still read as a card
	tabCardMaxNameW  = 150
	tabCardCloseSize = 16
)

func (r *tabCardRenderer) Destroy() {}

func (r *tabCardRenderer) MinSize() fyne.Size {
	chipMin := r.chip.MinSize()
	nameMin := r.name.MinSize()
	nameW := nameMin.Width
	if nameW > tabCardMaxNameW {
		nameW = tabCardMaxNameW
	}
	h := chipMin.Height
	if nameMin.Height > h {
		h = nameMin.Height
	}
	if tabCardCloseSize > h {
		h = tabCardCloseSize
	}
	w := float32(tabCardPad*2) +
		chipMin.Width + tabCardGap +
		nameW + tabCardGap +
		tabCardCloseSize
	if w < tabCardMinW {
		w = tabCardMinW
	}
	return fyne.NewSize(w, h+tabAccentH+float32(tabCardPad))
}

func (r *tabCardRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))

	// Cyan accent strip pinned to the TOP edge of the active card.
	r.accent.Resize(fyne.NewSize(size.Width, tabAccentH))
	r.accent.Move(fyne.NewPos(0, 0))

	// Content baseline sits below the accent strip; centre vertically.
	innerH := size.Height - tabAccentH
	x := float32(tabCardPad)

	chipSize := r.chip.MinSize()
	r.chip.Move(fyne.NewPos(x, tabAccentH+(innerH-chipSize.Height)/2))
	r.chip.Resize(chipSize)
	x += chipSize.Width + tabCardGap

	// Close button is right-aligned; name takes the middle, clamped.
	closeX := size.Width - tabCardPad - tabCardCloseSize
	r.close.Resize(fyne.NewSize(tabCardCloseSize, tabCardCloseSize))
	r.close.Move(fyne.NewPos(closeX, tabAccentH+(innerH-tabCardCloseSize)/2))

	nameSize := r.name.MinSize()
	availW := closeX - tabCardGap - x
	if availW < 0 {
		availW = 0
	}
	if nameSize.Width > availW {
		nameSize.Width = availW
	}
	r.name.Resize(nameSize)
	r.name.Move(fyne.NewPos(x, tabAccentH+(innerH-nameSize.Height)/2))
}

func (r *tabCardRenderer) Refresh() {
	c := r.card

	// Method chip.
	r.chip.Text = methodAbbrev(c.method)
	r.chip.Color = methodColor(c.method)
	r.chip.Refresh()

	// Display name with a leading dirty dot, ellipsis-clamped by the renderer's
	// clipped width. Bright when active, dim otherwise.
	label := c.name
	if label == "" {
		label = "Untitled"
	}
	if c.dirty {
		label = "● " + label
	}
	r.name.Text = clampEllipsis(label, tabCardMaxNameW, r.name.TextSize)
	if c.active {
		r.name.Color = theme.Color(theme.ColorNameForeground) // bright
	} else {
		r.name.Color = theme.Color(theme.ColorNamePlaceHolder) // dim
	}
	r.name.Refresh()

	// Card surface + accent: active = raised lighter card (#13203A=ColorNameButton)
	// with the cyan (#18C5E8=ColorNamePrimary) top-accent; inactive = flatter,
	// darker (#1A2940=ColorNameSeparator), no accent.
	if c.active {
		r.bg.FillColor = theme.Color(theme.ColorNameButton)
		r.accent.FillColor = theme.Color(theme.ColorNamePrimary)
	} else {
		r.bg.FillColor = theme.Color(theme.ColorNameSeparator)
		r.accent.FillColor = color.Transparent
	}
	r.bg.Refresh()
	r.accent.Refresh()

	canvas.Refresh(r.card)
}

func (r *tabCardRenderer) Objects() []fyne.CanvasObject { return r.objects }

// clampEllipsis truncates s with a trailing "…" so its rendered width stays
// within maxW at the given text size. A best-effort estimate that never panics
// (the renderer also clips the text object's width as a hard backstop).
func clampEllipsis(s string, maxW, textSize float32) string {
	if fyne.MeasureText(s, textSize, fyne.TextStyle{}).Width <= maxW {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		cand := string(runes) + "…"
		if fyne.MeasureText(cand, textSize, fyne.TextStyle{}).Width <= maxW {
			return cand
		}
	}
	return "…"
}

// ---- close affordance ----

// tabClose is the small "×" on each card: a separate tappable so clicking it
// fires the close callback without selecting the card.
type tabClose struct {
	widget.BaseWidget
	onTap   func()
	hovered bool
}

func newTabClose(onTap func()) *tabClose {
	c := &tabClose{onTap: onTap}
	c.ExtendBaseWidget(c)
	return c
}

func (c *tabClose) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *tabClose) MouseIn(*desktop.MouseEvent)    { c.hovered = true; c.Refresh() }
func (c *tabClose) MouseOut()                      { c.hovered = false; c.Refresh() }
func (c *tabClose) MouseMoved(*desktop.MouseEvent) {}

func (c *tabClose) CreateRenderer() fyne.WidgetRenderer {
	x := canvas.NewText("×", theme.Color(theme.ColorNamePlaceHolder))
	x.Alignment = fyne.TextAlignCenter
	x.TextStyle = fyne.TextStyle{Bold: true}
	r := &tabCloseRenderer{btn: c, x: x, objects: []fyne.CanvasObject{x}}
	r.Refresh()
	return r
}

type tabCloseRenderer struct {
	btn     *tabClose
	x       *canvas.Text
	objects []fyne.CanvasObject
}

func (r *tabCloseRenderer) Destroy()                     {}
func (r *tabCloseRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *tabCloseRenderer) MinSize() fyne.Size {
	return fyne.NewSize(tabCardCloseSize, tabCardCloseSize)
}

func (r *tabCloseRenderer) Layout(size fyne.Size) {
	r.x.Resize(size)
	r.x.Move(fyne.NewPos(0, 0))
}

func (r *tabCloseRenderer) Refresh() {
	if r.btn.hovered {
		r.x.Color = theme.Color(theme.ColorNameForeground)
	} else {
		r.x.Color = theme.Color(theme.ColorNamePlaceHolder)
	}
	r.x.Refresh()
}
