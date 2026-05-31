package ui

import (
	"fmt"
	"image/color"
	"os"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// maxDisplayBytes caps how much of a response body is rendered on screen. Larger
// bodies show only the head plus a notice and a "Save to file" button; the full
// body is retained in memory (ADR-0001).
const maxDisplayBytes = 256 * 1024

// largeBodyThreshold is the display-text size at or above which the coloured
// per-cell TextGrid is too expensive to build (its parseRows allocates one cell
// per rune — ~1.44M allocations for a 256 KB body — and the JSON colouring adds a
// SetStyleRange per token). For bodies this large the Pretty view is rendered
// into a lighter, virtualized read-only monospace widget.List instead (one
// Label per visible line; off-screen lines are never laid out). Smaller bodies
// keep the coloured TextGrid path. The split is "small body = coloured TextGrid,
// large body = lightweight read-only line list" and stays read-only per ADR-0001.
const largeBodyThreshold = 64 * 1024

// Status-class colours for the status label.
var (
	colorStatus2xx = color.NRGBA{R: 0x2e, G: 0x7d, B: 0x32, A: 0xff} // green
	colorStatus3xx = color.NRGBA{R: 0x15, G: 0x65, B: 0xc0, A: 0xff} // blue
	colorStatus4xx = color.NRGBA{R: 0xef, G: 0x6c, B: 0x00, A: 0xff} // orange
	colorStatus5xx = color.NRGBA{R: 0xc6, G: 0x28, B: 0x28, A: 0xff} // red
	colorStatusErr = color.NRGBA{R: 0xc6, G: 0x28, B: 0x28, A: 0xff} // red
)

// colorRespHeaderKey is the cyan used for response-header keys in the left
// column, matching mockup-v2's `.rh .k` (--cyan #18C5E8).
var (
	colorRespHeaderKey = color.NRGBA{R: 0x18, G: 0xc5, B: 0xe8, A: 0xff}
	styleRespHeaderKey = &widget.CustomTextGridStyle{FGColor: colorRespHeaderKey}
)

// responseView renders a model.Response read-only: a status/time/size line, a
// response-headers list, and the body in a read-only TextGrid (never an Entry,
// ADR-0001) with a Pretty/Raw toggle (Pretty default).
type responseView struct {
	parent fyne.Window

	container *fyne.Container

	statusLabel *canvas.Text
	metaLabel   *widget.Label
	headersGrid *widget.TextGrid
	bodyGrid    *widget.TextGrid
	saveBtn     *widget.Button
	noticeLabel *widget.Label

	// Pretty/Raw is a two-button segmented control (mockup-v2): pretty holds the
	// current mode; prettyBtn/rawBtn show which is active (HighImportance = on).
	pretty    bool
	prettyBtn *widget.Button
	rawBtn    *widget.Button

	// copyBtn copies the response body to the clipboard (shown once there is one).
	copyBtn *widget.Button

	// Status pill: a rounded coloured rectangle with a small status-class dot
	// behind the statusLabel text (mockup-v2 `.status-pill`). The pill background,
	// dot and text colour are all driven by statusColor(code).
	statusPillBG *canvas.Rectangle
	statusDot    *canvas.Circle
	statusPill   *fyne.Container

	// bodyList is the lightweight read-only viewer used for large bodies instead
	// of the per-cell TextGrid (ADR-0001: still read-only, monospace, scrollable).
	// It is virtualized — only on-screen lines are realised into a widget.
	bodyList  *widget.List
	bodyLines []string

	// bodyStack holds both viewers; renderBody shows exactly one of bodyScroll
	// (the TextGrid) or bodyList depending on body size.
	bodyStack  *fyne.Container
	bodyScroll *container.Scroll

	// fullBody is the complete response body, retained even when the display is
	// truncated, so "Save to file" always writes everything.
	fullBody []byte
}

// newResponseView builds an empty response pane bound to parent (used for the
// Save-to-file dialog).
func newResponseView(parent fyne.Window) *responseView {
	rv := &responseView{parent: parent}

	rv.statusLabel = canvas.NewText("No response yet", color.Gray{Y: 0x88})
	rv.statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	rv.statusLabel.TextSize = theme.TextSize() - 1

	// Status pill (mockup-v2 `.status-pill`): rounded coloured rectangle + a small
	// status-class dot in front of the text. The pill colours track statusColor().
	rv.statusPillBG = canvas.NewRectangle(color.NRGBA{})
	rv.statusPillBG.CornerRadius = 7
	rv.statusDot = canvas.NewCircle(color.Gray{Y: 0x88})

	rv.metaLabel = widget.NewLabel("")

	// Pretty/Raw segmented control + Copy. pretty defaults true; the active
	// segment is HighImportance. The handlers call setPretty (which renders) and
	// only fire on user click — after the body viewers exist — so there is no
	// construction-time render.
	rv.pretty = true
	rv.prettyBtn = widget.NewButton("Pretty", func() { rv.setPretty(true) })
	rv.rawBtn = widget.NewButton("Raw", func() { rv.setPretty(false) })
	rv.prettyBtn.Importance = widget.HighImportance
	rv.rawBtn.Importance = widget.MediumImportance

	// Icon-only Copy (compact so the response bar's right cluster never clips).
	rv.copyBtn = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), rv.copyBody)
	rv.copyBtn.Importance = widget.LowImportance
	rv.copyBtn.Hide()

	rv.saveBtn = widget.NewButton("Save to file", rv.saveToFile)
	rv.saveBtn.Hide()

	rv.noticeLabel = widget.NewLabel("")
	rv.noticeLabel.Hide()

	rv.headersGrid = widget.NewTextGrid()
	rv.bodyGrid = widget.NewTextGrid()

	// Lightweight read-only viewer for large bodies. Each row is a non-editable,
	// non-wrapping monospace Label; widget.List only realises the visible rows, so
	// rendering cost is independent of body size (no per-rune cell allocation).
	rv.bodyList = widget.NewList(
		func() int { return len(rv.bodyLines) },
		func() fyne.CanvasObject {
			lbl := widget.NewLabel("")
			lbl.TextStyle = fyne.TextStyle{Monospace: true}
			lbl.Wrapping = fyne.TextWrapOff
			return lbl
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= 0 && id < len(rv.bodyLines) {
				obj.(*widget.Label).SetText(rv.bodyLines[id])
			}
		},
	)
	rv.bodyList.Hide()

	// Status pill: dot + text over a rounded coloured background. A thin spacer
	// keeps the pill snug around its content (mockup-v2 `.status-pill` padding).
	dotBox := container.New(layout.NewCenterLayout(), rv.statusDot)
	pillContent := container.New(
		layout.NewHBoxLayout(),
		dotBox, rv.statusLabel,
	)
	rv.statusPill = container.NewStack(
		rv.statusPillBG,
		container.NewPadded(pillContent),
	)

	// Response header row: section label + status pill + meta on the LEFT, the
	// Pretty/Raw toggle (and Save) pinned to the RIGHT (mockup-v2 `.resp-bar`).
	respTitle := canvas.NewText("RESPONSE", theme.Color(theme.ColorNamePlaceHolder))
	respTitle.TextStyle = fyne.TextStyle{Bold: true}
	respTitle.TextSize = theme.TextSize() - 3

	left := container.New(layout.NewHBoxLayout(),
		respTitle, rv.statusPill, rv.metaLabel)
	// Save (when truncated) · Copy · Pretty | Raw — a single flat row pinned right
	// (adjacent Pretty/Raw buttons read as a segmented control).
	right := container.New(layout.NewHBoxLayout(),
		rv.saveBtn, rv.copyBtn, rv.prettyBtn, rv.rawBtn)
	headerRow := container.NewBorder(nil, nil, left, right)

	header := container.NewVBox(headerRow, rv.noticeLabel)

	// Body content: the coloured TextGrid (small bodies, scrolled) stacked with
	// the lightweight List (large bodies). renderBody shows exactly one. UNCHANGED
	// perf architecture — only its surrounding layout differs.
	rv.bodyScroll = container.NewVScroll(rv.bodyGrid)
	rv.bodyStack = container.NewStack(rv.bodyScroll, rv.bodyList)

	// Left column: response HEADERS (key cyan / value), beside the body on the
	// right, as an HSplit (mockup-v2 `.resp-body-wrap`).
	headersTitle := canvas.NewText("HEADERS", theme.Color(theme.ColorNamePlaceHolder))
	headersTitle.TextStyle = fyne.TextStyle{Bold: true}
	headersTitle.TextSize = theme.TextSize() - 3
	headersCol := container.NewBorder(
		container.NewPadded(headersTitle), nil, nil, nil,
		container.NewVScroll(rv.headersGrid),
	)

	split := container.NewHSplit(headersCol, rv.bodyStack)
	split.SetOffset(0.26) // headers strip is the narrower pane (mockup ~268/1180)

	rv.container = container.NewBorder(header, nil, nil, nil, split)

	// Initialise the pill to the neutral "no response yet" tint.
	rv.applyStatusPill(color.Gray{Y: 0x88})
	return rv
}

// applyStatusPill tints the pill background, dot and text from a status-class
// colour. The background is the same hue at low alpha (a tinted chip, like
// mockup-v2's `--green-bg`); the dot and text use the full colour.
func (rv *responseView) applyStatusPill(c color.Color) {
	rv.statusLabel.Color = c
	rv.statusDot.FillColor = c
	r, g, b, _ := c.RGBA()
	rv.statusPillBG.FillColor = color.NRGBA{
		R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0x26,
	}
	rv.statusPillBG.Refresh()
	rv.statusDot.Refresh()
	rv.statusLabel.Refresh()
}

// setPretty switches the Pretty/Raw segmented control (active = HighImportance)
// and re-renders the body. Called from the two segment buttons.
func (rv *responseView) setPretty(p bool) {
	rv.pretty = p
	if p {
		rv.prettyBtn.Importance = widget.HighImportance
		rv.rawBtn.Importance = widget.MediumImportance
	} else {
		rv.prettyBtn.Importance = widget.MediumImportance
		rv.rawBtn.Importance = widget.HighImportance
	}
	rv.prettyBtn.Refresh()
	rv.rawBtn.Refresh()
	rv.renderBody()
}

// copyBody copies the full response body to the clipboard — pretty-printed when
// in Pretty mode and the body is JSON, otherwise the raw bytes. No-op when there
// is no body.
func (rv *responseView) copyBody() {
	if len(rv.fullBody) == 0 {
		return
	}
	data := rv.fullBody
	if rv.pretty {
		if indented, ok := prettyJSON(data); ok {
			data = indented
		}
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(string(data))
	}
}

// setPending shows an in-flight indicator while a send runs.
func (rv *responseView) setPending() {
	rv.statusLabel.Text = "Sending…"
	rv.applyStatusPill(color.Gray{Y: 0x88})
	rv.metaLabel.SetText("")
	rv.noticeLabel.Hide()
	rv.saveBtn.Hide()
	rv.copyBtn.Hide()
	rv.fullBody = nil
	rv.clearBody()
	rv.headersGrid.SetText("")
}

// setError shows a transport/timeout/cancel error in place of a response.
func (rv *responseView) setError(err error) {
	rv.statusLabel.Text = "Error"
	rv.applyStatusPill(colorStatusErr)
	rv.metaLabel.SetText(err.Error())
	rv.noticeLabel.Hide()
	rv.saveBtn.Hide()
	rv.copyBtn.Hide()
	rv.fullBody = nil
	rv.clearBody()
	rv.headersGrid.SetText("")
}

// setResponse renders a completed Response.
func (rv *responseView) setResponse(resp model.Response) {
	rv.fullBody = resp.Body
	rv.copyBtn.Show()

	rv.statusLabel.Text = fmt.Sprintf("%d %s", resp.Status, resp.StatusText)
	rv.applyStatusPill(statusColor(resp.Status))

	rv.metaLabel.SetText(fmt.Sprintf("   %s   ·   %s",
		formatDuration(resp.Duration), formatSize(resp.Size)))

	rv.renderHeaders(resp.Headers)
	rv.renderBody()
}

// renderHeaders writes the response headers into the left-column TextGrid,
// sorted by key for stable display, with each header key coloured cyan (mockup-v2
// `.rh .k`) and the value left in the default foreground.
func (rv *responseView) renderHeaders(headers []model.Param) {
	sorted := append([]model.Param(nil), headers...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })

	var b strings.Builder
	for _, h := range sorted {
		b.WriteString(h.Key)
		b.WriteString(": ")
		b.WriteString(h.Value)
		b.WriteByte('\n')
	}
	rv.headersGrid.SetText(b.String())

	// Colour the key (everything up to the first ": ") of each row cyan. Header
	// keys are ASCII, so the rune column equals the byte index for the key span.
	for r, h := range sorted {
		if r >= len(rv.headersGrid.Rows) {
			break
		}
		cells := rv.headersGrid.Rows[r].Cells
		end := len([]rune(h.Key))
		if end > len(cells) {
			end = len(cells)
		}
		for c := 0; c < end; c++ {
			cells[c].Style = styleRespHeaderKey
		}
	}
	rv.headersGrid.Refresh()
}

// renderBody (re)renders the body honouring the Pretty/Raw toggle and the
// 256 KB display cap. It is called on send completion and on toggle.
func (rv *responseView) renderBody() {
	if rv.fullBody == nil {
		rv.clearBody()
		return
	}

	body := rv.fullBody
	truncated := false
	if len(body) > maxDisplayBytes {
		body = body[:maxDisplayBytes]
		truncated = true
	}

	pretty := rv.pretty
	var display string
	var coloured bool

	if pretty {
		if indented, ok := prettyJSON(body); ok {
			display = string(indented)
			coloured = true
		} else {
			display = string(body)
		}
	} else {
		display = string(body)
	}

	if pretty && len(display) >= largeBodyThreshold {
		// Large body in Pretty mode — the path the user reported as laggy. The
		// expensive part is the TextGrid itself: parseRows calls runewidth on a
		// freshly-allocated string per rune (~256K string allocations per 256 KB)
		// and, when the JSON is valid, the colouring pass adds a style per token.
		// Render into the lightweight virtualized List instead: only the on-screen
		// lines become widgets, so cost is independent of body size. Syntax colour
		// is dropped on this path (the trade-off); the view stays read-only,
		// monospace and scrollable per ADR-0001. Raw mode keeps the plain TextGrid
		// so a user can opt back into the exact-bytes grid view.
		rv.showLargeBody(display)
	} else {
		// Small bodies, and Raw bodies of any size, keep the (optionally coloured)
		// TextGrid path.
		rv.showSmallBody(display, coloured)
	}

	if truncated {
		rv.noticeLabel.SetText(fmt.Sprintf(
			"Showing first %s of %s — body truncated for display. Use “Save to file” for the full body.",
			formatSize(int64(maxDisplayBytes)), formatSize(int64(len(rv.fullBody)))))
		rv.noticeLabel.Show()
		rv.saveBtn.Show()
	} else {
		rv.noticeLabel.Hide()
		rv.saveBtn.Hide()
	}
}

// showSmallBody renders display into the TextGrid and makes it the visible Body
// viewer (hiding the large-body List). For a coloured (valid-JSON) small body it
// builds the styled rows in one pass via buildJSONRows (jsonpretty.go) and
// assigns them directly, avoiding the SetText parse + per-token SetStyleRange
// round-trip. A non-JSON body keeps plain SetText.
func (rv *responseView) showSmallBody(display string, coloured bool) {
	rv.bodyLines = nil
	rv.bodyList.Hide()

	if coloured {
		rv.bodyGrid.Rows = buildJSONRows(display)
		rv.bodyGrid.Refresh()
	} else {
		rv.bodyGrid.SetText(display)
	}
	rv.bodyList.Refresh()
	rv.bodyScroll.Show()
}

// showLargeBody renders display into the lightweight virtualized List and makes
// it the visible Body viewer (hiding the TextGrid). The TextGrid is cleared so
// it neither holds a stale large body nor pays its per-cell cost.
func (rv *responseView) showLargeBody(display string) {
	rv.bodyGrid.SetText("")
	rv.bodyScroll.Hide()

	rv.bodyLines = strings.Split(display, "\n")
	rv.bodyList.Refresh()
	rv.bodyList.ScrollToTop()
	rv.bodyList.Show()
}

// clearBody resets both Body viewers to empty.
func (rv *responseView) clearBody() {
	rv.bodyLines = nil
	rv.bodyGrid.SetText("")
	rv.bodyList.Refresh()
	rv.bodyList.Hide()
	rv.bodyScroll.Show()
}

// saveToFile writes the full (un-truncated) body to a user-chosen file via a
// native OS save dialog, falling back to Fyne's in-app dialog if unavailable.
func (rv *responseView) saveToFile() {
	if rv.fullBody == nil {
		return
	}
	go func() {
		path, ok, err := nativeSaveAny("Save Response Body", "response.txt")
		fyne.Do(func() {
			switch {
			case err != nil:
				rv.saveToFileFyne()
			case !ok:
				// cancelled
			default:
				if werr := os.WriteFile(path, rv.fullBody, 0o644); werr != nil {
					dialog.ShowError(werr, rv.parent)
				}
			}
		})
	}()
}

// saveToFileFyne is the Fyne in-app fallback for saveToFile().
func (rv *responseView) saveToFileFyne() {
	dialog.ShowFileSave(func(wc fyne.URIWriteCloser, err error) {
		if err != nil || wc == nil {
			return
		}
		defer wc.Close()
		if _, werr := wc.Write(rv.fullBody); werr != nil {
			dialog.ShowError(werr, rv.parent)
		}
	}, rv.parent)
}

// statusColor maps an HTTP status code to its class colour.
func statusColor(code int) color.Color {
	switch {
	case code >= 200 && code < 300:
		return colorStatus2xx
	case code >= 300 && code < 400:
		return colorStatus3xx
	case code >= 400 && code < 500:
		return colorStatus4xx
	case code >= 500:
		return colorStatus5xx
	default:
		return color.Gray{Y: 0x88}
	}
}

// formatDuration renders a send duration compactly.
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%d µs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2f s", d.Seconds())
}

// formatSize renders a byte count as B / KB / MB.
func formatSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}
