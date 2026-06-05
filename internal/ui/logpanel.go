package ui

import (
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ---- Log model + pure formatter (Fyne-free, unit-testable) ----

// logEntry is one line in the combined HTTP request log: a single send and its
// outcome. URL is the resolved (templates-expanded) URL actually requested; Name
// is the request's display name (may be empty). On success Status/StatusText/
// Duration/Size describe the Response. A non-empty Err means the send FAILED, in
// which case the status/size fields are not shown — only the error is.
type logEntry struct {
	Time       time.Time
	Name       string
	Method     string
	URL        string
	Status     int
	StatusText string
	Duration   time.Duration
	Size       int64
	Err        string
}

// formatLogEntry renders one log line. It is PURE (no Fyne) so the panel and the
// blind tester share one source of truth.
//
// Layout, with the timestamp formatted as 15:04:05:
//
//	success → "<HH:MM:SS>  [<Name>]  <Method> <URL> → <Status> <StatusText> · <dur> · <size>"
//	error   → "<HH:MM:SS>  [<Name>]  <Method> <URL> → ERROR: <Err>"
//
// The "[<Name>]" clause is omitted when Name is empty, and the "· <size>" clause
// is omitted when Size <= 0. Duration/size use the package's formatDuration /
// formatSize so the log matches the response viewer.
func formatLogEntry(e logEntry) string {
	var b strings.Builder
	b.WriteString(e.Time.Format("15:04:05"))
	b.WriteString("  ")
	if e.Name != "" {
		b.WriteString("[")
		b.WriteString(e.Name)
		b.WriteString("]  ")
	}
	b.WriteString(e.Method)
	b.WriteString(" ")
	b.WriteString(e.URL)
	b.WriteString(" → ")

	if e.Err != "" {
		b.WriteString("ERROR: ")
		b.WriteString(e.Err)
		return b.String()
	}

	b.WriteString(strings.TrimSpace(statusClause(e.Status, e.StatusText)))
	b.WriteString(" · ")
	b.WriteString(formatDuration(e.Duration))
	if e.Size > 0 {
		b.WriteString(" · ")
		b.WriteString(formatSize(e.Size))
	}
	return b.String()
}

// statusClause renders "<Status> <StatusText>", dropping the space when there is
// no status text.
func statusClause(status int, text string) string {
	if text == "" {
		return strconv.Itoa(status)
	}
	return strconv.Itoa(status) + " " + text
}

// formatLog joins every entry's formatLogEntry line with "\n" (no trailing
// newline). An empty slice yields "". PURE.
func formatLog(entries []logEntry) string {
	if len(entries) == 0 {
		return ""
	}
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = formatLogEntry(e)
	}
	return strings.Join(lines, "\n")
}

// ---- Panel UI (read-only) ----

// logPanel is the read-only combined request log: a selectable multiline view of
// every send this session, with Copy and Clear actions. It owns only display
// state; refresh() re-reads the Window's reqLog and rebuilds the text.
type logPanel struct {
	win       *Window
	container fyne.CanvasObject

	entry *widget.Entry // read-only-ish multiline showing formatLog(win.reqLog)
}

// newLogPanel builds the panel UI and stores it in .container. The text view has
// no OnChanged (refresh overwrites it, so any stray user edit is harmless), and a
// bottom bar carries Copy and Clear.
func newLogPanel(w *Window) *logPanel {
	p := &logPanel{win: w}

	header := widget.NewLabelWithStyle("Request log", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	p.entry = widget.NewMultiLineEntry()
	p.entry.Wrapping = fyne.TextWrapOff // one send per line; user can drag-select

	copyBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		if a := fyne.CurrentApp(); a != nil {
			a.Clipboard().SetContent(formatLog(p.win.reqLog))
		}
	})
	copyBtn.Importance = widget.HighImportance

	clearBtn := widget.NewButtonWithIcon("Clear", theme.DeleteIcon(), func() {
		p.win.clearLog()
		p.refresh()
	})

	bottomBar := container.NewBorder(nil, nil, copyBtn, clearBtn)

	p.container = container.NewBorder(header, bottomBar, nil, nil, p.entry)

	p.refresh()
	return p
}

// refresh re-reads the Window's reqLog, rebuilds the multiline text via
// formatLog, and scrolls to the newest (bottom) line. Safe to call anytime,
// including before the Window has any logged sends (empty log → empty text).
func (p *logPanel) refresh() {
	if p.entry == nil {
		return // not built yet
	}
	text := formatLog(p.win.reqLog)
	p.entry.SetText(text)
	// Scroll to the newest line: put the cursor on the last row.
	last := strings.Count(text, "\n")
	p.entry.CursorRow = last
	p.entry.CursorColumn = 0
	p.entry.Refresh()
}
