package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// newRequestTabForTest builds a Window around a one-request collection and opens
// its editor tab, the same way the app does, for the URL⇄Params sync tests.
func newRequestTabForTest(t *testing.T, req model.Request) *requestTab {
	t.Helper()
	a := test.NewApp()
	coll := model.NewCollection("T")
	coll.Requests = []model.Request{req}
	w := newWindow(New(a), coll, "")
	t.Cleanup(w.win.Close)
	return newRequestTab(w, 0)
}

// TestSync_URLQueryPopulatesParams pins the headline feature: typing/pasting a
// URL with a query string fills the Params table — no "+ Add" clicking.
func TestSync_URLQueryPopulatesParams(t *testing.T) {
	rt := newRequestTabForTest(t, model.Request{Method: model.MethodGet, URL: "https://api.com/usage"})

	rt.urlEntry.SetText("https://api.com/usage?account=x&token=y") // fires OnChanged

	got := rt.paramsTable.value()
	if len(got) != 2 ||
		got[0] != (model.Param{Key: "account", Value: "x", Enabled: true}) ||
		got[1] != (model.Param{Key: "token", Value: "y", Enabled: true}) {
		t.Fatalf("params after URL edit = %#v", got)
	}
}

// TestSync_ParamsRewriteURL pins the reverse direction: editing the Params table
// rewrites the URL field's query while keeping the base path.
func TestSync_ParamsRewriteURL(t *testing.T) {
	rt := newRequestTabForTest(t, model.Request{Method: model.MethodGet, URL: "https://api.com/usage"})

	// Seed two rows and fire the table's change handler via a real row edit.
	rt.paramsTable.setValue([]model.Param{
		{Key: "account", Value: "x", Enabled: true},
		{Key: "token", Value: "", Enabled: true},
	})
	rt.paramsTable.rows[1].value.SetText("y") // fires onParamsEdited

	if got, want := rt.urlEntry.Text, "https://api.com/usage?account=x&token=y"; got != want {
		t.Fatalf("URL after params edit = %q, want %q", got, want)
	}
}

// TestSync_DisabledRowExcludedFromURL pins that unchecking a row removes it from
// the URL query but keeps it in the table (kept-but-not-sent).
func TestSync_DisabledRowExcludedFromURL(t *testing.T) {
	rt := newRequestTabForTest(t, model.Request{Method: model.MethodGet, URL: "https://api.com/u?a=1&b=2"})

	rt.paramsTable.rows[1].enabled.SetChecked(false) // disable b; fires onParamsEdited

	if got, want := rt.urlEntry.Text, "https://api.com/u?a=1"; got != want {
		t.Fatalf("URL after disabling row = %q, want %q", got, want)
	}
	if len(rt.paramsTable.value()) != 2 {
		t.Fatalf("disabled row should be kept in the table: %#v", rt.paramsTable.value())
	}
}

// TestSync_NoDoubleQueryOnSend pins the correctness guard: current() stores only
// the base URL (the query lives in Params), so the built request/curl carry the
// query exactly once rather than twice.
func TestSync_NoDoubleQueryOnSend(t *testing.T) {
	rt := newRequestTabForTest(t, model.Request{Method: model.MethodGet, URL: "https://api.com/usage?account=x"})

	cur := rt.current()
	if cur.URL != "https://api.com/usage" {
		t.Fatalf("current().URL = %q, want base without query", cur.URL)
	}
	if len(cur.Params) != 1 || cur.Params[0].Key != "account" {
		t.Fatalf("current().Params = %#v, want the query carried once", cur.Params)
	}
}

// TestSync_SpecialCharsInValueRoundTrip pins that a param value containing query
// metacharacters (&, =, space) survives a Params→URL→Params round-trip instead of
// being split into bogus rows. Regression test for Auditor A Finding 1.
func TestSync_SpecialCharsInValueRoundTrip(t *testing.T) {
	rt := newRequestTabForTest(t, model.Request{Method: model.MethodGet, URL: "https://api.com/p"})

	rt.paramsTable.setValue([]model.Param{{Key: "q", Value: "a&b c", Enabled: true}})
	rt.onParamsEdited()              // Params → URL
	rt.onURLEdited(rt.urlEntry.Text) // URL → Params (reparse)

	got := rt.paramsTable.value()
	if len(got) != 1 || got[0].Key != "q" || got[0].Value != "a&b c" {
		t.Fatalf("value with special chars corrupted on round-trip: %#v (url=%q)", got, rt.urlEntry.Text)
	}
}

// TestSync_URLEditPreservesRowOrder pins that editing the URL keeps disabled rows
// in their original positions instead of shoving them to the end. Regression test
// for Auditor A Finding 2.
func TestSync_URLEditPreservesRowOrder(t *testing.T) {
	rt := newRequestTabForTest(t, model.Request{Method: model.MethodGet, URL: "https://api.com/p"})
	rt.paramsTable.setValue([]model.Param{
		{Key: "a", Value: "1", Enabled: true},
		{Key: "b", Value: "2", Enabled: false}, // disabled, sits in the middle
		{Key: "c", Value: "3", Enabled: true},
	})

	rt.urlEntry.SetText("https://api.com/p?a=1&c=3&d=4") // append d via the URL

	got := rt.paramsTable.value()
	keys := make([]string, len(got))
	for i, p := range got {
		keys[i] = p.Key
	}
	want := []string{"a", "b", "c", "d"}
	if len(keys) != len(want) {
		t.Fatalf("row count/order = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("row order = %v, want %v", keys, want)
		}
	}
}

// TestSync_LoadMergesSavedQueryAndParams pins that opening a Request whose saved
// URL has a query AND has its own Params shows them merged once in both views.
func TestSync_LoadMergesSavedQueryAndParams(t *testing.T) {
	rt := newRequestTabForTest(t, model.Request{
		Method: model.MethodGet,
		URL:    "https://api.com/u?a=1",
		Params: []model.Param{{Key: "b", Value: "2", Enabled: true}},
	})

	if got, want := rt.urlEntry.Text, "https://api.com/u?a=1&b=2"; got != want {
		t.Fatalf("loaded URL = %q, want %q", got, want)
	}
	if got := rt.paramsTable.value(); len(got) != 2 || got[0].Key != "a" || got[1].Key != "b" {
		t.Fatalf("loaded params = %#v", got)
	}
}
