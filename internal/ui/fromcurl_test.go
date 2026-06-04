package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// newRequestFromCurlText is the Fyne-free parse core. A valid curl line yields a
// request with the parsed Method/URL; invalid input returns an error.
func TestNewRequestFromCurlText_ValidAndInvalid(t *testing.T) {
	req, err := newRequestFromCurlText("curl https://api.example.com/users -H 'Accept: application/json'")
	if err != nil {
		t.Fatalf("valid curl: unexpected error: %v", err)
	}
	if req.URL != "https://api.example.com/users" {
		t.Errorf("URL = %q, want https://api.example.com/users", req.URL)
	}
	if req.Method != model.MethodGet {
		t.Errorf("Method = %q, want GET", req.Method)
	}

	if _, err := newRequestFromCurlText(""); err == nil {
		t.Error("empty input: expected an error, got nil")
	}
}

// appendAndOpenRequest appends to the Collection, marks dirty, and opens (selects)
// the new request. Exercised on the headless driver so the sidebar/tab wiring runs.
func TestAppendAndOpenRequest_AppendsMarksDirtyOpens(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)
	w := app.NewCollectionWindow()

	before := len(w.coll.Requests)
	w.dirty = false

	req := model.Request{
		Method: model.MethodGet,
		URL:    "https://example.com/from-curl",
		Auth:   model.Auth{Kind: model.AuthNone},
		Body:   model.Body{Type: model.BodyNone},
	}
	w.appendAndOpenRequest(req)

	if got := len(w.coll.Requests); got != before+1 {
		t.Fatalf("Requests len = %d, want %d", got, before+1)
	}
	if w.coll.Requests[before].URL != "https://example.com/from-curl" {
		t.Errorf("appended URL = %q", w.coll.Requests[before].URL)
	}
	if !w.dirty {
		t.Error("expected window marked dirty after append")
	}
}

// newRequestFromCurl builds and shows its dialog without panicking on the
// headless driver (construction smoke).
func TestNewRequestFromCurl_DialogBuilds(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)
	w := app.NewCollectionWindow()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("newRequestFromCurl panicked: %v", r)
		}
	}()
	w.newRequestFromCurl()
}
