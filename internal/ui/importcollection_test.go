package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// validPostman is a minimal but schema-valid Postman Collection v2.1 document
// with info.name "Imported" and a single GET request.
const validPostman = `{
  "info": {
    "name": "Imported",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "item": [
    {
      "name": "Get Users",
      "request": {
        "method": "GET",
        "header": [],
        "url": "https://api.example.com/users"
      }
    }
  ]
}`

// patchPostman has one GET (supported) and one PATCH (unsupported) request, so
// importing it produces a non-empty Report.SkippedRequests while still opening a
// window for the GET.
const patchPostman = `{
  "info": {
    "name": "Mixed",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "item": [
    {
      "name": "List",
      "request": {
        "method": "GET",
        "header": [],
        "url": "https://api.example.com/list"
      }
    },
    {
      "name": "Patch It",
      "request": {
        "method": "PATCH",
        "header": [],
        "url": "https://api.example.com/patch"
      }
    }
  ]
}`

// TestImportCollectionData_OpensWindow feeds a valid one-GET Postman document and
// asserts a new untitled collection window is opened carrying the imported
// collection name + the single mapped request, and that the app now tracks it.
func TestImportCollectionData_OpensWindow(t *testing.T) {
	a := test.NewApp()
	app := New(a)

	before := len(app.windows)

	w, _, err := app.ImportCollectionData([]byte(validPostman))
	if err != nil {
		t.Fatalf("ImportCollectionData returned error on valid JSON: %v", err)
	}
	if w == nil {
		t.Fatal("ImportCollectionData returned a nil window on success")
	}
	t.Cleanup(w.win.Close)

	if w.coll.Name != "Imported" {
		t.Fatalf("collection name = %q, want %q", w.coll.Name, "Imported")
	}
	if got := len(w.coll.Requests); got != 1 {
		t.Fatalf("imported request count = %d, want 1", got)
	}

	req := w.coll.Requests[0]
	if req.Method != model.MethodGet {
		t.Fatalf("request method = %q, want %q", req.Method, model.MethodGet)
	}
	if req.URL != "https://api.example.com/users" {
		t.Fatalf("request URL = %q, want %q", req.URL, "https://api.example.com/users")
	}

	if len(app.windows) != before+1 {
		t.Fatalf("app window count = %d, want %d (new window should be tracked)", len(app.windows), before+1)
	}
	if _, ok := app.windows[w]; !ok {
		t.Fatal("returned window is not tracked in app.windows")
	}
}

// TestImportCollectionData_BadJSON asserts invalid JSON yields an error and no
// window (nothing should be opened).
func TestImportCollectionData_BadJSON(t *testing.T) {
	a := test.NewApp()
	app := New(a)

	before := len(app.windows)

	w, _, err := app.ImportCollectionData([]byte("nope"))
	if err == nil {
		t.Fatal("ImportCollectionData returned nil error on invalid JSON")
	}
	if w != nil {
		t.Cleanup(w.win.Close)
		t.Fatalf("ImportCollectionData returned a non-nil window on error")
	}
	if len(app.windows) != before {
		t.Fatalf("app window count changed on failed import: got %d, want %d", len(app.windows), before)
	}
}

// TestImportCollectionData_ReportSurfaced asserts the lossy-import Report is
// returned to the caller: an unsupported PATCH request is recorded in
// SkippedRequests, while the window still opens with the supported GET.
func TestImportCollectionData_ReportSurfaced(t *testing.T) {
	a := test.NewApp()
	app := New(a)

	w, report, err := app.ImportCollectionData([]byte(patchPostman))
	if err != nil {
		t.Fatalf("ImportCollectionData returned error: %v", err)
	}
	if w == nil {
		t.Fatal("ImportCollectionData returned a nil window despite supported requests")
	}
	t.Cleanup(w.win.Close)

	if len(report.SkippedRequests) == 0 {
		t.Fatal("report.SkippedRequests is empty, want an entry for the PATCH request")
	}
	foundPatch := false
	for _, s := range report.SkippedRequests {
		if strings.Contains(s, "PATCH") {
			foundPatch = true
			break
		}
	}
	if !foundPatch {
		t.Fatalf("no SkippedRequests entry mentions PATCH: %v", report.SkippedRequests)
	}

	// The supported GET should still have been imported into the open window.
	if got := len(w.coll.Requests); got != 1 {
		t.Fatalf("imported request count = %d, want 1 (only the GET)", got)
	}
	if w.coll.Requests[0].Method != model.MethodGet {
		t.Fatalf("surviving request method = %q, want %q", w.coll.Requests[0].Method, model.MethodGet)
	}
}
