package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/yonner"
)

// newRequestFromCurlText parses a curl command line into a model.Request. It is
// the pure, Fyne-free core of the File ▸ New Request from cURL… action: it just
// delegates to yonner.FromCurl, leaving the request exactly as parsed (Auth left
// at whatever FromCurl set, FolderID/Name empty). Kept dialog-free so the parse
// path is unit-testable without rendering a window.
func newRequestFromCurlText(text string) (model.Request, error) {
	return yonner.FromCurl(text)
}

// newRequestFromCurl drives the File ▸ New Request from cURL… action: it shows a
// dialog with a wide multiline entry for a curl command. On Create it parses the
// text with newRequestFromCurlText; a parse error is shown inline (a red label
// under the entry) and the dialog is re-shown with the text intact so the user
// can fix it. On success the new request is appended to the Collection and
// opened, and the dialog closes.
func (w *Window) newRequestFromCurl() {
	entry := widget.NewMultiLineEntry()
	entry.SetPlaceHolder("curl https://api.example.com/users -H 'Accept: application/json'")
	entry.Wrapping = fyne.TextWrapWord

	errLabel := canvas.NewText("", theme.Color(theme.ColorNameError))
	errLabel.TextStyle = fyne.TextStyle{Bold: true}
	errLabel.Hide()

	// Give the entry room to breathe inside the resized dialog.
	entryBox := container.NewGridWrap(fyne.NewSize(600, 240), entry)
	body := container.NewVBox(
		widget.NewLabel("Paste a curl command:"),
		entryBox,
		errLabel,
	)

	// NewCustomConfirm hides the dialog before invoking the callback, so on a
	// parse error we re-show this same dialog instance — its widget tree (the
	// entry's text and the error label) is preserved, so the user keeps their
	// input and sees the message.
	var d *dialog.ConfirmDialog
	d = dialog.NewCustomConfirm("New Request from cURL", "Create", "Cancel", body,
		func(ok bool) {
			if !ok {
				return
			}
			req, err := newRequestFromCurlText(entry.Text)
			if err != nil {
				errLabel.Text = err.Error()
				errLabel.Refresh()
				errLabel.Show()
				d.Show() // re-open; text + error preserved
				return
			}
			w.appendAndOpenRequest(req)
		}, w.win)
	d.Resize(fyne.NewSize(640, 360))
	d.Show()
}

// appendAndOpenRequest appends req to the Collection, marks the window dirty,
// refreshes the sidebar so the new top-level row exists, and selects (opens) it.
// Factored out of addRequest's tail so other request-creation flows (e.g. New
// Request from cURL) share the identical append+open behavior.
func (w *Window) appendAndOpenRequest(req model.Request) {
	w.coll.Requests = append(w.coll.Requests, req)
	w.markDirty()
	// A new request is top-level (FolderID ""); recompute the grouped rows so its
	// row exists before we select it.
	w.refreshSidebar()
	// Select via the sidebar (not openRequestTab directly) so the new row's
	// selection + cyan accent stay in sync with the opened tab. selectByReqIdx maps
	// the flat index to the visible row.
	w.selectByReqIdx(len(w.coll.Requests) - 1)
}
