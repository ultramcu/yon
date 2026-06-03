package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/yonner"
)

// blindSyncWindow builds a Window holding a single Request and returns the
// requestTab editor opened on it. Independent of the existing sync tests.
func blindSyncWindow(t *testing.T, req model.Request) *requestTab {
	t.Helper()
	a := test.NewApp()
	coll := model.NewCollection("blind")
	coll.Requests = []model.Request{req}
	w := newWindow(New(a), coll, "")
	t.Cleanup(w.win.Close)
	return newRequestTab(w, 0)
}

// findParam returns the first param whose key matches, and whether it existed.
func findParam(ps []model.Param, key string) (model.Param, bool) {
	for _, p := range ps {
		if p.Key == key {
			return p, true
		}
	}
	return model.Param{}, false
}

// SPEC 1: URL -> Params. Setting the URL query produces two enabled rows in
// order: account=x then token=y.
func TestBlindSpec1_URLToParams(t *testing.T) {
	rt := blindSyncWindow(t, model.Request{Method: model.MethodGet})

	rt.urlEntry.SetText("https://api.com/usage?account=x&token=y")

	got := rt.paramsTable.value()
	if len(got) != 2 {
		t.Fatalf("want 2 param rows, got %d: %+v", len(got), got)
	}
	want := []model.Param{
		{Key: "account", Value: "x", Enabled: true},
		{Key: "token", Value: "y", Enabled: true},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d = %+v, want %+v", i, got[i], w)
		}
	}
}

// SPEC 2: Params -> URL. Editing the table rewrites the URL query while keeping
// the base path.
func TestBlindSpec2_ParamsToURL(t *testing.T) {
	rt := blindSyncWindow(t, model.Request{
		Method: model.MethodGet,
		URL:    "https://api.com/usage",
	})

	// Set two rows via the row widgets so the params->URL handler fires.
	rt.paramsTable.setValue([]model.Param{
		{Key: "account", Value: "x", Enabled: true},
		{Key: "token", Value: "y", Enabled: true},
	})
	// setValue doesn't fire onChange; trigger the sync the way a row edit would.
	rt.onParamsEdited()

	want := "https://api.com/usage?account=x&token=y"
	if rt.urlEntry.Text != want {
		t.Fatalf("URL = %q, want %q", rt.urlEntry.Text, want)
	}
}

// SPEC 3: Disabling a row removes it from URL query but keeps the row in table.
func TestBlindSpec3_DisableKeepsRowDropsFromURL(t *testing.T) {
	rt := blindSyncWindow(t, model.Request{Method: model.MethodGet})
	rt.urlEntry.SetText("https://api.com/usage?account=x&token=y")

	// Uncheck the "token" row via its widget (fires the params->URL sync).
	var tokenRow *kvRow
	for _, r := range rt.paramsTable.rows {
		if r.key.Text == "token" {
			tokenRow = r
		}
	}
	if tokenRow == nil {
		t.Fatalf("token row not found; rows=%+v", rt.paramsTable.value())
	}
	tokenRow.enabled.SetChecked(false)

	// Row still present in the table.
	p, ok := findParam(rt.paramsTable.value(), "token")
	if !ok {
		t.Errorf("token row should remain in the table after disabling")
	}
	if p.Enabled {
		t.Errorf("token row should be disabled, got Enabled=true")
	}
	// Removed from the URL query.
	if strings.Contains(rt.urlEntry.Text, "token") {
		t.Errorf("URL should drop disabled token, got %q", rt.urlEntry.Text)
	}
	if rt.urlEntry.Text != "https://api.com/usage?account=x" {
		t.Errorf("URL = %q, want %q", rt.urlEntry.Text, "https://api.com/usage?account=x")
	}
}

// SPEC 4: Percent-encoded pasted values decode for display; {{template}} stays
// literal (not %7B...).
func TestBlindSpec4_DecodeForDisplayTemplateLiteral(t *testing.T) {
	rt := blindSyncWindow(t, model.Request{Method: model.MethodGet})

	rt.urlEntry.SetText("https://api.com/x?enc=John%20Doe&tok={{token}}")

	enc, ok := findParam(rt.paramsTable.value(), "enc")
	if !ok {
		t.Fatalf("enc row missing; rows=%+v", rt.paramsTable.value())
	}
	if enc.Value != "John Doe" {
		t.Errorf("enc value = %q, want %q", enc.Value, "John Doe")
	}

	tok, ok := findParam(rt.paramsTable.value(), "tok")
	if !ok {
		t.Fatalf("tok row missing; rows=%+v", rt.paramsTable.value())
	}
	if tok.Value != "{{token}}" {
		t.Errorf("tok value = %q, want literal {{token}}", tok.Value)
	}
	if strings.Contains(tok.Value, "%7B") || strings.Contains(tok.Value, "%7b") {
		t.Errorf("tok value must not be percent-encoded, got %q", tok.Value)
	}
}

// SPEC 5: Sending must not duplicate the query. After the URL has a query, the
// built request carries each pair exactly once and templates are not %7B.
func TestBlindSpec5_NoDuplicateQueryOnSend(t *testing.T) {
	rt := blindSyncWindow(t, model.Request{Method: model.MethodGet})
	rt.urlEntry.SetText("https://api.com/usage?account=x&token={{token}}")

	// What the editor stores as the request to be sent.
	req := rt.current()

	// The stored URL must not still carry the query (else yonner appends it again).
	if strings.Contains(req.URL, "account") || strings.Contains(req.URL, "?") {
		t.Errorf("stored req.URL should be base-only, got %q", req.URL)
	}

	curl := yonner.ToCurl(req, rt.win.coll, yonner.DefaultOptions())

	if n := strings.Count(curl, "account=x"); n != 1 {
		t.Errorf("account=x appears %d times in curl, want 1:\n%s", n, curl)
	}
	if n := strings.Count(curl, "token="); n != 1 {
		t.Errorf("token= appears %d times in curl, want 1:\n%s", n, curl)
	}
	if strings.Contains(curl, "%7B") || strings.Contains(curl, "%7b") {
		t.Errorf("template braces must not be percent-encoded in curl:\n%s", curl)
	}
	if !strings.Contains(curl, "{{token}}") {
		t.Errorf("curl should carry literal {{token}}:\n%s", curl)
	}
}

// SPEC 6: Loading a request whose saved URL already has a query AND its own
// Params merges both, shown once in URL and table.
func TestBlindSpec6_LoadMergesSavedQueryAndParams(t *testing.T) {
	rt := blindSyncWindow(t, model.Request{
		Method: model.MethodGet,
		URL:    "https://api.com/usage?account=x",
		Params: []model.Param{{Key: "token", Value: "y", Enabled: true}},
	})

	// Table shows both, each once.
	ps := rt.paramsTable.value()
	if _, ok := findParam(ps, "account"); !ok {
		t.Errorf("account missing from merged table: %+v", ps)
	}
	if _, ok := findParam(ps, "token"); !ok {
		t.Errorf("token missing from merged table: %+v", ps)
	}
	if c := countKey(ps, "account"); c != 1 {
		t.Errorf("account appears %d times in table, want 1: %+v", c, ps)
	}
	if c := countKey(ps, "token"); c != 1 {
		t.Errorf("token appears %d times in table, want 1: %+v", c, ps)
	}

	// URL shows each pair once.
	if c := strings.Count(rt.urlEntry.Text, "account=x"); c != 1 {
		t.Errorf("account=x appears %d times in URL, want 1: %q", c, rt.urlEntry.Text)
	}
	if c := strings.Count(rt.urlEntry.Text, "token=y"); c != 1 {
		t.Errorf("token=y appears %d times in URL, want 1: %q", c, rt.urlEntry.Text)
	}

	// And the built request also carries each once.
	curl := yonner.ToCurl(rt.current(), rt.win.coll, yonner.DefaultOptions())
	if c := strings.Count(curl, "account=x"); c != 1 {
		t.Errorf("account=x appears %d times in curl, want 1:\n%s", c, curl)
	}
	if c := strings.Count(curl, "token=y"); c != 1 {
		t.Errorf("token=y appears %d times in curl, want 1:\n%s", c, curl)
	}
}

func countKey(ps []model.Param, key string) int {
	n := 0
	for _, p := range ps {
		if p.Key == key {
			n++
		}
	}
	return n
}

// SPEC 7: No infinite loop / hang when either side changes. Each mutation below
// returning (the test finishing) is the evidence.
func TestBlindSpec7_NoInfiniteLoop(t *testing.T) {
	rt := blindSyncWindow(t, model.Request{Method: model.MethodGet})

	// URL -> params -> (guard stops echo back to URL)
	rt.urlEntry.SetText("https://api.com/p?a=1&b=2")
	// params edit -> URL -> (guard stops echo back to params)
	if len(rt.paramsTable.rows) > 0 {
		rt.paramsTable.rows[0].value.SetText("99")
	}
	// toggle enabled
	if len(rt.paramsTable.rows) > 0 {
		rt.paramsTable.rows[0].enabled.SetChecked(false)
		rt.paramsTable.rows[0].enabled.SetChecked(true)
	}
	// change URL again
	rt.urlEntry.SetText("https://api.com/p?a=1&b=2&c=3")

	// If we got here without hanging, the guard works.
	if len(rt.paramsTable.value()) != 3 {
		t.Errorf("expected 3 rows after final URL set, got %+v", rt.paramsTable.value())
	}
}
