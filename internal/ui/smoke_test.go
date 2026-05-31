package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// Smoke test: build the UI controller and its windows on the headless Fyne
// test driver and assert nothing panics. This does not drive real GUI
// interaction (hard to test) — it only checks the construction paths wire up.

func TestSmoke_NewApp(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)
	if app == nil {
		t.Fatal("New returned nil App")
	}
	if app.fyneApp == nil {
		t.Error("App has no underlying fyne.App")
	}
	if app.windows == nil {
		t.Error("App.windows map not initialised")
	}
}

func TestSmoke_NewCollectionWindowBuilds(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	w := app.NewCollectionWindow()
	if w == nil {
		t.Fatal("NewCollectionWindow returned nil")
	}
	if _, ok := app.windows[w]; !ok {
		t.Error("new window not tracked in app.windows")
	}
}

func TestSmoke_OpenCollectionWindow(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	coll := model.NewCollection("Smoke")
	coll.Requests = append(coll.Requests, model.Request{
		Name:   "Ping",
		Method: model.MethodGet,
		URL:    "https://example.com/ping",
		Body:   model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
	})

	w := app.OpenCollectionWindow(coll, "/tmp/smoke.yon")
	if w == nil {
		t.Fatal("OpenCollectionWindow returned nil")
	}
	if _, ok := app.windows[w]; !ok {
		t.Error("opened window not tracked in app.windows")
	}
}

// TestSmoke_OpenRequestTabBuilds asserts the Request-editor Tab builds without
// panicking under the headless Fyne test driver, for Requests with REAL content
// that drives every previously-crashing construction path.
//
// Four ordering bugs of the same class were fixed by the source owner; each was
// "SetSelected/SetChecked fired its OnChanged (the owner's commit) before the
// dependent widget/owner existed, dereferencing nil":
//   - requesteditor.go methodSel  — SetSelected before Param/Header/Auth/Body exist
//   - responseview.go prettyToggle — SetChecked before bodyGrid exists
//   - auth.go        kindSelect    — SetSelected before the authEditor is wired
//   - kvtable.go     row enabled   — seeding an ENABLED row fired commit before the
//     kvTable was assigned into its owner
//
// To catch the kvTable + authEditor cases the seeding Request must carry an
// ENABLED query Param, an ENABLED Header, and BASIC auth (so kindSelect lands on
// a non-default kind and the credential fields render). A second Request uses
// bearer + inherit auth so those kinds are constructed too. Opening the tabs
// must not panic, and the editors must read the seeded values back — proving the
// widgets were really built and wired, not silently skipped.
func TestSmoke_OpenRequestTabBuilds(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	coll := model.NewCollection("Smoke")
	coll.Auth = model.Auth{Kind: model.AuthBasic, Username: "colluser", Password: "collpass"}
	coll.Requests = append(coll.Requests,
		// Request 0: enabled param + enabled header + basic auth — exercises the
		// kvTable enabled-row seeding and the authEditor basic-credential path.
		model.Request{
			Name:   "WithContent",
			Method: model.MethodPost,
			URL:    "https://example.com/api",
			Params: []model.Param{
				{Key: "q", Value: "search", Enabled: true},
				{Key: "page", Value: "2", Enabled: false},
			},
			Headers: []model.Param{
				{Key: "X-Token", Value: "abc", Enabled: true},
			},
			Auth: model.Auth{Kind: model.AuthBasic, Username: "alice", Password: "s3cret"},
			Body: model.Body{Type: model.BodyJSON, Content: `{"hello":"world"}`},
		},
		// Request 1: bearer auth — exercises the authEditor bearer path.
		model.Request{
			Name:   "Bearer",
			Method: model.MethodGet,
			URL:    "https://example.com/me",
			Auth:   model.Auth{Kind: model.AuthBearer, Token: "tok-123"},
		},
		// Request 2: inherit auth — exercises the authEditor inherit default.
		model.Request{
			Name:   "Inherit",
			Method: model.MethodDelete,
			URL:    "https://example.com/x",
			Auth:   model.Auth{Kind: model.AuthInherit},
		},
	)

	w := app.OpenCollectionWindow(coll, "/tmp/smoke.yon")

	// Open every request tab — each is a fresh newRequestTab construction.
	for i := range coll.Requests {
		w.openRequestTab(i) // expected: builds without panic
	}

	// Assert the enabled-param / enabled-header / basic-auth tab actually wired
	// its widgets by reading the editor back. If construction had skipped a
	// widget (or the ordering fix regressed and we'd panicked above), these would
	// not round-trip.
	rt0, ok := w.openTabs[0]
	if !ok {
		t.Fatal("request tab 0 was not opened")
	}
	got0 := rt0.current()
	if got0.Auth.Kind != model.AuthBasic || got0.Auth.Username != "alice" || got0.Auth.Password != "s3cret" {
		t.Errorf("tab 0 auth = %+v, want basic alice/s3cret", got0.Auth)
	}
	// value() drops fully-empty rows but keeps disabled-with-content rows, so both
	// the enabled and the disabled param survive with their Enabled flags intact.
	if len(got0.Params) != 2 {
		t.Fatalf("tab 0 params = %+v, want 2 (one enabled, one disabled)", got0.Params)
	}
	if got0.Params[0].Key != "q" || got0.Params[0].Value != "search" || !got0.Params[0].Enabled {
		t.Errorf("tab 0 param[0] = %+v, want q/search enabled", got0.Params[0])
	}
	if got0.Params[1].Enabled {
		t.Errorf("tab 0 param[1] = %+v, want Enabled=false preserved", got0.Params[1])
	}
	if len(got0.Headers) != 1 || got0.Headers[0].Key != "X-Token" || !got0.Headers[0].Enabled {
		t.Errorf("tab 0 headers = %+v, want one enabled X-Token", got0.Headers)
	}

	// Bearer + inherit tabs must also have wired their kind selects.
	rt1 := w.openTabs[1]
	if got := rt1.current().Auth; got.Kind != model.AuthBearer || got.Token != "tok-123" {
		t.Errorf("tab 1 auth = %+v, want bearer tok-123", got)
	}
	rt2 := w.openTabs[2]
	if got := rt2.current().Auth.Kind; got != model.AuthInherit {
		t.Errorf("tab 2 auth kind = %q, want inherit", got)
	}
}
