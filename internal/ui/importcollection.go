package ui

import (
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"github.com/ncruces/zenity"

	"github.com/ultramcu/yon/internal/importer"
)

// jsonFilter selects Collection (JSON) export files in the native open dialog.
var jsonFilter = zenity.FileFilters{
	{Name: "Collection JSON", Patterns: []string{"*.json"}, CaseFold: true},
}

// nativeOpenJSON shows a native open dialog filtered to .json files. It returns
// (path, ok, err) like the other native wrappers: ok is false on cancel, and a
// non-nil err means the native backend is unavailable so the caller should fall
// back to Fyne's in-app dialog.
func nativeOpenJSON(title string) (string, bool, error) {
	return nativeResult(zenity.SelectFile(zenity.Title(title), jsonFilter))
}

// ImportCollectionData converts a Collection (JSON) export (raw JSON bytes) into a Collection
// and opens it in a new, untitled window. The empty path means the Collection
// has no backing .yon file yet, so the user must Save As to write one. It returns
// the new Window and the conversion Report on success, or the error from
// importer.Import otherwise. Kept free of dialogs/IO so it can be unit-tested.
func (a *App) ImportCollectionData(data []byte) (*Window, importer.Report, error) {
	coll, report, err := importer.Import(data)
	if err != nil {
		return nil, report, err
	}
	w := a.OpenCollectionWindow(coll, "")
	return w, report, nil
}

// importCollection drives the File ▸ Import Collection (JSON)… action: it shows a native
// open dialog filtered to *.json (falling back to Fyne's in-app dialog when the
// native backend is unavailable), reads the chosen file, converts it into a new
// untitled Collection window, and — when the conversion skipped requests or
// recorded notes — shows a summary dialog. Any error is reported via dialog.
func (w *Window) importCollection() {
	go func() {
		path, ok, err := nativeOpenJSON("Import Collection (JSON)")
		fyne.Do(func() {
			switch {
			case err != nil:
				w.importCollectionFyne() // native unavailable → in-app dialog
			case !ok:
				// cancelled — nothing to do
			default:
				w.importCollectionPath(path)
			}
		})
	}()
}

// importCollectionFyne is the Fyne in-app fallback for importCollection().
func (w *Window) importCollectionFyne() {
	d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil || rc == nil {
			return
		}
		path := rc.URI().Path()
		_ = rc.Close()
		w.importCollectionPath(path)
	}, w.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
	d.Show()
}

// importCollectionPath reads the file at path, imports it, and surfaces the outcome:
// an error dialog on failure, or a notes dialog when the import had anything to
// report. Shared by the native and Fyne open flows.
func (w *Window) importCollectionPath(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		dialog.ShowError(err, w.win)
		return
	}
	_, report, err := w.app.ImportCollectionData(data)
	if err != nil {
		dialog.ShowError(err, w.win)
		return
	}
	if summary := importReportSummary(report); summary != "" {
		dialog.ShowInformation("Imported with notes", summary, w.win)
	}
}

// importReportSummary formats a conversion Report into a readable, multi-line
// message: a "Skipped:" block listing any dropped requests, then a "Notes:"
// block. It returns "" when the Report is empty (nothing worth a dialog).
func importReportSummary(report importer.Report) string {
	if len(report.SkippedRequests) == 0 && len(report.Notes) == 0 {
		return ""
	}
	var b strings.Builder
	if len(report.SkippedRequests) > 0 {
		fmt.Fprintf(&b, "Skipped %d request(s):\n", len(report.SkippedRequests))
		for _, s := range report.SkippedRequests {
			fmt.Fprintf(&b, "  • %s\n", s)
		}
	}
	if len(report.Notes) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("Notes:\n")
		for _, n := range report.Notes {
			fmt.Fprintf(&b, "  • %s\n", n)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
