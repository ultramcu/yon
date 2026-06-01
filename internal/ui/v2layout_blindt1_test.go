package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// Blind Test author #1 — independent verification of the v2 "Dark Pro" layout
// (sidebar verb chips + collection-header count badge + Params/Headers sub-tab
// count badges). Expectations are derived from assets/design/mockup-v2.png and
// the spec, not from reading the implementation's control flow. These tests
// are written to be FAIL-able: each asserts an observable property of the built
// widgets.
//
// Note: a sibling file (v2layout_test.go) also tests the v2 layout; the helpers
// and function names here are deliberately distinct to avoid collisions.

// rgbaEq compares two colours by their RGBA() output (handles NRGBA vs color
// abstractions). Kept local to this file under a distinct name.
func rgbaEq(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// ---- Sidebar verb tag colour tracks the HTTP method ----

// In the v2 mockup the sidebar lists each request as a coloured UPPERCASE verb
// tag + name. The tag colour must be produced by methodColor(method): a GET row
// is GET-coloured, a POST row is POST-coloured. We assert both that each row
// matches its own methodColor AND that GET and POST are visibly different, so a
// regression that paints every tag the same colour fails.
func TestVerbTagColourTracksMethod(t *testing.T) {
	getRow := newVerbRow()
	getRow.set(model.Request{Method: model.MethodGet, Name: "query + header"}, false)

	postRow := newVerbRow()
	postRow.set(model.Request{Method: model.MethodPost, Name: "JSON body"}, false)

	if !rgbaEq(getRow.tag.Color, methodColor(model.MethodGet)) {
		t.Errorf("GET tag colour = %v, want methodColor(GET) = %v",
			getRow.tag.Color, methodColor(model.MethodGet))
	}
	if !rgbaEq(postRow.tag.Color, methodColor(model.MethodPost)) {
		t.Errorf("POST tag colour = %v, want methodColor(POST) = %v",
			postRow.tag.Color, methodColor(model.MethodPost))
	}
	// Mockup intent: different verbs are visually distinguishable.
	if rgbaEq(getRow.tag.Color, postRow.tag.Color) {
		t.Errorf("GET and POST tag colours are identical (%v); verb colours should differ",
			getRow.tag.Color)
	}
}

// The verb tag text is the UPPERCASE abbreviation shown in the mockup
// (GET/POST/PUT and the narrow "DEL" for DELETE).
func TestVerbTagTextUppercaseAbbrev(t *testing.T) {
	cases := map[model.Method]string{
		model.MethodGet:    "GET",
		model.MethodPost:   "POST",
		model.MethodPut:    "PUT",
		model.MethodDelete: "DEL",
	}
	for m, want := range cases {
		r := newVerbRow()
		r.set(model.Request{Method: m}, false)
		if r.tag.Text != want {
			t.Errorf("method %s: verb tag text = %q, want %q", m, r.tag.Text, want)
		}
	}
}

// Re-binding a row to a different method must recolour the tag (UpdateItem reuse
// path): a row first shown as POST, then re-set to GET, ends GET-coloured. This
// guards the virtualized-list recycle path the sidebar relies on.
func TestVerbTagRecolourOnReuse(t *testing.T) {
	r := newVerbRow()
	r.set(model.Request{Method: model.MethodPost}, false)
	r.set(model.Request{Method: model.MethodGet}, false)
	if !rgbaEq(r.tag.Color, methodColor(model.MethodGet)) {
		t.Errorf("after re-set to GET, tag colour = %v, want methodColor(GET) = %v",
			r.tag.Color, methodColor(model.MethodGet))
	}
	if r.tag.Text != "GET" {
		t.Errorf("after re-set to GET, tag text = %q, want %q", r.tag.Text, "GET")
	}
}

// ---- Collection-header request-count badge ----

// The collection header badge shows the number of Requests (mockup: "Yon Test
// Server  13"). It must equal len(coll.Requests) and update after addRequest.
func TestSidebarCountBadgeMatchesAndUpdates(t *testing.T) {
	app := New(test.NewApp())
	coll := model.Collection{
		Version: 1,
		Name:    "Yon Test Server",
		Requests: []model.Request{
			{Method: model.MethodGet, Name: "a"},
			{Method: model.MethodPost, Name: "b"},
			{Method: model.MethodPut, Name: "c"},
		},
	}
	w := app.OpenCollectionWindow(coll, "/tmp/count.yon")

	if got, want := w.sidebarCount.Text, "3"; got != want {
		t.Fatalf("initial count badge = %q, want %q (len=%d)", got, want, len(w.coll.Requests))
	}

	w.addRequest() // appends one Request and should refresh the badge
	if got, want := w.sidebarCount.Text, "4"; got != want {
		t.Errorf("after addRequest, count badge = %q, want %q (len=%d)", got, want, len(w.coll.Requests))
	}
	if len(w.coll.Requests) != 4 {
		t.Errorf("addRequest did not grow the collection: len=%d, want 4", len(w.coll.Requests))
	}
}

// An empty collection shows a "0" badge (header is built for any collection).
func TestSidebarCountBadgeEmptyCollection(t *testing.T) {
	app := New(test.NewApp())
	w := app.OpenCollectionWindow(model.Collection{Version: 1, Name: "Empty"}, "/tmp/empty.yon")
	if got := w.sidebarCount.Text; got != "0" {
		t.Errorf("empty-collection count badge = %q, want %q", got, "0")
	}
}

// ---- Params/Headers sub-tab count badges ----

// A Request with 2 enabled + 1 disabled params seeds the Params badge as
// "Params 2"; the single enabled header seeds "Headers 1". Mirrors the mockup's
// "Params 3" badge (which corresponds to its 3 enabled rows). We assert the
// enabled-only-and-present rule precisely.
func TestTabBadgeInitialSeed(t *testing.T) {
	app := New(test.NewApp())
	coll := model.Collection{
		Version: 1,
		Name:    "Seed",
		Requests: []model.Request{{
			Method: model.MethodGet,
			URL:    "http://localhost:7878/get",
			Params: []model.Param{
				{Key: "page", Value: "1", Enabled: true},
				{Key: "q", Value: "hello", Enabled: true},
				{Key: "debug", Value: "true", Enabled: false}, // disabled → excluded
			},
			Headers: []model.Param{
				{Key: "X-Trace", Value: "abc", Enabled: true},
			},
		}},
	}
	w := app.OpenCollectionWindow(coll, "/tmp/seed.yon")
	w.openRequestTab(0)
	rt := w.openTabs[0]

	if rt.paramsTab.Text != "Params 2" {
		t.Errorf("initial Params badge = %q, want %q", rt.paramsTab.Text, "Params 2")
	}
	if rt.headerTab.Text != "Headers 1" {
		t.Errorf("initial Headers badge = %q, want %q", rt.headerTab.Text, "Headers 1")
	}
}

// A Request with zero enabled+present rows shows the plain base label (no
// trailing count) for both Params and Headers.
func TestTabBadgeZeroPlainLabel(t *testing.T) {
	app := New(test.NewApp())
	coll := model.Collection{
		Version: 1,
		Name:    "Zero",
		Requests: []model.Request{{
			Method: model.MethodGet,
			URL:    "http://localhost/x",
			// All excluded: one disabled-with-content, one fully-empty.
			Params: []model.Param{
				{Key: "k", Value: "v", Enabled: false},
				{Key: "", Value: "", Enabled: true},
			},
		}},
	}
	w := app.OpenCollectionWindow(coll, "/tmp/zero.yon")
	w.openRequestTab(0)
	rt := w.openTabs[0]

	if rt.paramsTab.Text != "Params" {
		t.Errorf("zero-count Params badge = %q, want plain %q", rt.paramsTab.Text, "Params")
	}
	if rt.headerTab.Text != "Headers" {
		t.Errorf("zero-count Headers badge = %q, want plain %q", rt.headerTab.Text, "Headers")
	}
}

// Toggling a param row off must drop the Params badge via the commit/refresh
// path (the row's Enabled check fires OnChanged → commit → refreshTabBadges).
func TestTabBadgeUpdatesOnToggle(t *testing.T) {
	app := New(test.NewApp())
	coll := model.Collection{
		Version: 1,
		Name:    "Toggle",
		Requests: []model.Request{{
			Method: model.MethodGet,
			URL:    "http://localhost/x",
			Params: []model.Param{
				{Key: "a", Value: "1", Enabled: true},
				{Key: "b", Value: "2", Enabled: true},
			},
		}},
	}
	w := app.OpenCollectionWindow(coll, "/tmp/toggle.yon")
	w.openRequestTab(0)
	rt := w.openTabs[0]

	if rt.paramsTab.Text != "Params 2" {
		t.Fatalf("pre-toggle Params badge = %q, want %q", rt.paramsTab.Text, "Params 2")
	}
	rt.paramsTable.rows[0].enabled.SetChecked(false) // commit via OnChanged
	if rt.paramsTab.Text != "Params 1" {
		t.Errorf("after disabling one row, Params badge = %q, want %q", rt.paramsTab.Text, "Params 1")
	}
}

// Editing a header row's value into a previously-empty row must raise the
// Headers badge through the commit path (Entry.OnChanged → commit).
func TestTabBadgeUpdatesOnEdit(t *testing.T) {
	app := New(test.NewApp())
	coll := model.Collection{
		Version: 1,
		Name:    "Edit",
		Requests: []model.Request{{
			Method: model.MethodGet,
			URL:    "http://localhost/x",
			Headers: []model.Param{
				{Key: "Existing", Value: "1", Enabled: true},
				{Key: "", Value: "", Enabled: true}, // empty → not yet counted
			},
		}},
	}
	w := app.OpenCollectionWindow(coll, "/tmp/edit.yon")
	w.openRequestTab(0)
	rt := w.openTabs[0]

	if rt.headerTab.Text != "Headers 1" {
		t.Fatalf("pre-edit Headers badge = %q, want %q", rt.headerTab.Text, "Headers 1")
	}
	// Fill the empty row's key so it becomes enabled+present.
	rt.headerTable.rows[1].key.SetText("X-New")
	if rt.headerTab.Text != "Headers 2" {
		t.Errorf("after editing a row into existence, Headers badge = %q, want %q",
			rt.headerTab.Text, "Headers 2")
	}
}

// ---- No-panic full Window build with rich seeded content ----

// Building a Window over a collection with varied seeded Requests (a GET with
// params + headers + basic auth, a POST with a JSON body + bearer auth, a PUT,
// and a DELETE) and opening each as a tab must not panic. This exercises the
// real construction wiring (sidebar verb rows, header badge, sub-tab badges,
// auth editor, body pane) under the headless Fyne test driver.
func TestWindowOpenSeededTabsNoPanic(t *testing.T) {
	app := New(test.NewApp())
	coll := model.Collection{
		Version: 1,
		Name:    "Yon Test Server",
		Auth:    model.Auth{Kind: model.AuthNone},
		Requests: []model.Request{
			{
				Name:   "query + header",
				Method: model.MethodGet,
				URL:    "http://localhost:7878/get",
				Params: []model.Param{
					{Key: "page", Value: "1", Enabled: true},
					{Key: "q", Value: "hello", Enabled: true},
					{Key: "debug", Value: "true", Enabled: false},
				},
				Headers: []model.Param{
					{Key: "Accept", Value: "application/json", Enabled: true},
				},
				Auth: model.Auth{Kind: model.AuthBasic, Username: "u", Password: "p"},
				Body: model.Body{Type: model.BodyNone},
			},
			{
				Name:    "JSON body",
				Method:  model.MethodPost,
				URL:     "http://localhost:7878/post",
				Headers: []model.Param{{Key: "Content-Type", Value: "application/json", Enabled: true}},
				Auth:    model.Auth{Kind: model.AuthBearer, Token: "tok"},
				Body:    model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
			},
			{
				Name:   "JSON",
				Method: model.MethodPut,
				URL:    "http://localhost:7878/put",
				Auth:   model.Auth{Kind: model.AuthInherit},
				Body:   model.Body{Type: model.BodyText, Content: "hello"},
			},
			{
				Name:   "DELETE",
				Method: model.MethodDelete,
				URL:    "http://localhost:7878/delete",
				Auth:   model.Auth{Kind: model.AuthInherit},
				Body:   model.Body{Type: model.BodyNone},
			},
		},
	}

	w := app.OpenCollectionWindow(coll, "/tmp/seeded.yon")
	if w == nil {
		t.Fatal("OpenCollectionWindow returned nil")
	}
	for i := range coll.Requests {
		w.openRequestTab(i)
	}
	if got := len(w.openTabs); got != len(coll.Requests) {
		t.Errorf("opened tabs = %d, want %d", got, len(coll.Requests))
	}
	// Sanity: the count badge reflects all seeded requests.
	if w.sidebarCount.Text != "4" {
		t.Errorf("seeded count badge = %q, want %q", w.sidebarCount.Text, "4")
	}
}
