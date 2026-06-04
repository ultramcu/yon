package ui

// Blind tests for the "New Request from cURL" UI lane (issue #22), written from
// the Dev B contract WITHOUT seeing the implementation:
//
//   - newRequestFromCurlText(text) (model.Request, error)
//       Fyne-free helper wrapping yonner.FromCurl(text) with UI defaulting.
//   - (*Window).appendAndOpenRequest(req model.Request)
//       appends req to w.coll.Requests, marks dirty, refreshes the sidebar, and
//       selects/opens it — the same flow as addRequest.
//
// These exercise behaviour, not rendering. The Window is built with the same
// test.NewApp() + OpenCollectionWindow pattern the smoke tests use.

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// newCurlTestWindow builds a headless Window backed by the given Collection,
// using the SAME construction path as the smoke tests (test.NewApp →
// OpenCollectionWindow). Returns the window so a test can read w.coll, w.dirty
// and w.selectedID directly (same package).
func newCurlTestWindow(t *testing.T, coll model.Collection) *Window {
	t.Helper()
	fyneApp := test.NewApp()
	app := New(fyneApp)
	w := app.OpenCollectionWindow(coll, "/tmp/curl-blindtest.yon")
	if w == nil {
		t.Fatal("OpenCollectionWindow returned nil")
	}
	return w
}

// A. A valid curl command parses into a populated Request. We assert only what
// the contract guarantees: Method, URL, and the JSON body — not incidental
// fields (Name, Auth, Headers) the contract does not pin down.
func TestNewRequestFromCurlText_Valid(t *testing.T) {
	const cmd = `curl -X POST https://api/x -H 'Content-Type: application/json' -d '{"a":1}'`

	req, err := newRequestFromCurlText(cmd)
	if err != nil {
		t.Fatalf("newRequestFromCurlText(valid) returned error: %v", err)
	}
	if req.Method != model.MethodPost {
		t.Errorf("Method = %q, want %q", req.Method, model.MethodPost)
	}
	if req.URL != "https://api/x" {
		t.Errorf("URL = %q, want %q", req.URL, "https://api/x")
	}
	if req.Body.Type != model.BodyJSON {
		t.Errorf("Body.Type = %q, want %q", req.Body.Type, model.BodyJSON)
	}
	if req.Body.Content != `{"a":1}` {
		t.Errorf("Body.Content = %q, want %q", req.Body.Content, `{"a":1}`)
	}
}

// B. Invalid input must surface a non-nil error rather than a silent zero
// Request. An empty string carries no command at all; a no-URL command names no
// target — both are invalid per the contract.
func TestNewRequestFromCurlText_Invalid(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"no url", "curl -X POST"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newRequestFromCurlText(tc.text); err == nil {
				t.Errorf("newRequestFromCurlText(%q) error = nil, want non-nil", tc.text)
			}
		})
	}
}

// C. appendAndOpenRequest appends the request to the Collection, grows the slice
// by exactly one with the new request last, marks the window dirty, and selects
// the appended request (selectedID == its flat index) — mirroring addRequest.
func TestAppendAndOpenRequest(t *testing.T) {
	coll := model.NewCollection("CurlTarget")
	// Seed one existing request so we verify the new one is APPENDED (not
	// replacing) and lands at the end.
	coll.Requests = append(coll.Requests, model.Request{
		Name:   "Existing",
		Method: model.MethodGet,
		URL:    "https://example.com/existing",
		Auth:   model.Auth{Kind: model.AuthInherit},
		Body:   model.Body{Type: model.BodyNone},
	})
	w := newCurlTestWindow(t, coll)

	before := len(w.coll.Requests)

	req := model.Request{
		Name:   "FromCurl",
		Method: model.MethodPost,
		URL:    "https://api/x",
		Auth:   model.Auth{Kind: model.AuthInherit},
		Body:   model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
	}
	w.appendAndOpenRequest(req)

	// Slice grew by exactly one.
	if got := len(w.coll.Requests); got != before+1 {
		t.Fatalf("len(Requests) = %d, want %d (grew by one)", got, before+1)
	}

	// The appended request is last and equals what we passed.
	// Request holds slices (Params/Headers) so it is not == comparable; compare
	// deeply.
	last := w.coll.Requests[len(w.coll.Requests)-1]
	if !reflect.DeepEqual(last, req) {
		t.Errorf("last request = %+v, want %+v", last, req)
	}

	// Appending an unsaved request marks the window dirty (mirrors addRequest).
	if !w.dirty {
		t.Error("window not marked dirty after appendAndOpenRequest")
	}

	// The new request is the selected one (selectedID is a FLAT request index).
	// The open of its editor tab is exercised by the verifier's smoke check.
	wantIdx := len(w.coll.Requests) - 1
	if w.selectedID != wantIdx {
		t.Errorf("selectedID = %d, want %d (the appended request)", w.selectedID, wantIdx)
	}
}
