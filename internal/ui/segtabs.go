package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// segActiveBG is the lighter fill of the selected segment (matches the active
// request-tab card surface), so the current sub-tab reads as a raised button.
var segActiveBG = color.NRGBA{R: 0x18, G: 0x2A, B: 0x47, A: 0xff}

// segTabs is a segmented sub-tab control (Params / Headers / Auth / Body) shown
// inside a request editor: a row of clearly-separated rounded segment buttons
// above a content area that shows the selected segment. It replaces
// container.AppTabs so the sub-tabs read as distinct buttons instead of one
// continuous bar.
type segTabs struct {
	bar      *fyne.Container // HBox of *segButton
	content  *fyne.Container // Stack; shows the selected segment's content
	root     *fyne.Container
	segs     []*segButton
	contents []fyne.CanvasObject
	selected int
}

func newSegTabs() *segTabs {
	s := &segTabs{selected: -1}
	s.bar = container.NewHBox()
	s.content = container.NewStack()
	s.root = container.NewBorder(
		container.NewVBox(container.NewPadded(s.bar), widget.NewSeparator()),
		nil, nil, nil, s.content)
	return s
}

// object returns the root to embed in the request editor.
func (s *segTabs) object() fyne.CanvasObject { return s.root }

// Append adds a segment with the given label + content and returns its button so
// the caller can update the label later (e.g. count badges). The first appended
// segment is auto-selected.
func (s *segTabs) Append(label string, content fyne.CanvasObject) *segButton {
	idx := len(s.segs)
	b := newSegButton(label, func() { s.Select(idx) })
	s.segs = append(s.segs, b)
	s.contents = append(s.contents, content)
	s.bar.Add(b)
	if s.selected < 0 {
		s.Select(0)
	}
	return b
}

// Select shows segment i and marks its button active.
func (s *segTabs) Select(i int) {
	if i < 0 || i >= len(s.segs) {
		return
	}
	s.selected = i
	for j, b := range s.segs {
		b.setActive(j == i)
	}
	s.content.Objects = []fyne.CanvasObject{s.contents[i]}
	s.content.Refresh()
}

// segButton is one tappable segment of a segTabs.
type segButton struct {
	widget.BaseWidget
	label  string
	active bool
	onTap  func()
}

func newSegButton(label string, onTap func()) *segButton {
	b := &segButton{label: label, onTap: onTap}
	b.ExtendBaseWidget(b)
	return b
}

func (b *segButton) Tapped(*fyne.PointEvent) {
	if b.onTap != nil {
		b.onTap()
	}
}

func (b *segButton) setActive(a bool) {
	if b.active != a {
		b.active = a
		b.Refresh()
	}
}

// setLabel updates the segment's caption (used for "Params 3"-style badges).
func (b *segButton) setLabel(s string) {
	if b.label != s {
		b.label = s
		b.Refresh()
	}
}

func (b *segButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 6
	underline := canvas.NewRectangle(color.Transparent)
	underline.CornerRadius = 1
	text := canvas.NewText(b.label, color.Gray{Y: 0x88})
	text.TextStyle = fyne.TextStyle{Bold: true}
	text.TextSize = theme.TextSize() - 1
	r := &segButtonRenderer{b: b, bg: bg, underline: underline, text: text}
	r.objects = []fyne.CanvasObject{bg, underline, text}
	r.Refresh()
	return r
}

type segButtonRenderer struct {
	b         *segButton
	bg        *canvas.Rectangle
	underline *canvas.Rectangle
	text      *canvas.Text
	objects   []fyne.CanvasObject
}

// segPadX / segPadY are the horizontal / vertical padding inside a segment.
const (
	segPadX = 14
	segPadY = 5
)

func (r *segButtonRenderer) MinSize() fyne.Size {
	t := fyne.MeasureText(r.text.Text, r.text.TextSize, r.text.TextStyle)
	return fyne.NewSize(t.Width+2*segPadX, t.Height+2*segPadY)
}

func (r *segButtonRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))
	// 2px accent underline along the bottom of the segment.
	r.underline.Resize(fyne.NewSize(size.Width, 2))
	r.underline.Move(fyne.NewPos(0, size.Height-2))
	ts := fyne.MeasureText(r.text.Text, r.text.TextSize, r.text.TextStyle)
	r.text.Move(fyne.NewPos((size.Width-ts.Width)/2, (size.Height-ts.Height)/2))
}

func (r *segButtonRenderer) Refresh() {
	r.text.Text = r.b.label
	if r.b.active {
		r.bg.FillColor = segActiveBG
		r.underline.FillColor = theme.Color(theme.ColorNamePrimary)
		r.text.Color = theme.Color(theme.ColorNamePrimary)
	} else {
		// A faint raised box so every segment reads as its own button, with the
		// active one clearly brighter on top.
		r.bg.FillColor = theme.Color(theme.ColorNameButton)
		r.underline.FillColor = color.Transparent
		r.text.Color = theme.Color(theme.ColorNamePlaceHolder)
	}
	r.bg.Refresh()
	r.underline.Refresh()
	r.text.Refresh()
	canvas.Refresh(r.b)
}

func (r *segButtonRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *segButtonRenderer) Destroy()                     {}
