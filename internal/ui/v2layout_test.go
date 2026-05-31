package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// The v2 "Dark Pro" sidebar verb chip must colour its tag via methodColor and
// abbreviate DELETE to "DEL". newVerbRow().set() is the binding used by the
// List's UpdateItem, so testing it covers the real recolour path.
func TestVerbRow_TagColourMatchesMethodColor(t *testing.T) {
	cases := []struct {
		method model.Method
		tag    string
	}{
		{model.MethodGet, "GET"},
		{model.MethodPost, "POST"},
		{model.MethodPut, "PUT"},
		{model.MethodDelete, "DEL"},
	}
	for _, tc := range cases {
		r := newVerbRow()
		r.set(model.Request{Method: tc.method, Name: "x"}, false)

		if r.tag.Text != tc.tag {
			t.Errorf("%s: tag text = %q, want %q", tc.method, r.tag.Text, tc.tag)
		}
		want := methodColor(tc.method)
		if !colorsEqual(r.tag.Color, want) {
			t.Errorf("%s: tag colour = %v, want methodColor() = %v", tc.method, r.tag.Color, want)
		}
	}
}

// An unknown method must fall back to the slate verb colour, not crash.
func TestVerbRow_UnknownMethodSlate(t *testing.T) {
	r := newVerbRow()
	r.set(model.Request{Method: model.Method("PATCH"), Name: "x"}, false)
	if !colorsEqual(r.tag.Color, methodColor(model.Method("PATCH"))) {
		t.Errorf("unknown method colour = %v, want slate %v", r.tag.Color, methodColorSlate)
	}
}

// The selected row paints its cyan left accent; an unselected row is transparent.
func TestVerbRow_SelectedAccent(t *testing.T) {
	r := newVerbRow()
	r.set(model.Request{Method: model.MethodGet}, true)
	if !colorsEqual(r.accent.FillColor, verbRowAccent) {
		t.Errorf("selected accent = %v, want cyan %v", r.accent.FillColor, verbRowAccent)
	}
	r.set(model.Request{Method: model.MethodGet}, false)
	if !colorsEqual(r.accent.FillColor, color.Transparent) {
		t.Errorf("unselected accent = %v, want transparent", r.accent.FillColor)
	}
}

// enabledParamCount counts only enabled rows that carry content; tabBadge then
// hides a zero count and shows "Base N" otherwise.
func TestTabBadge_CountsEnabledPresentRows(t *testing.T) {
	params := []model.Param{
		{Key: "page", Value: "1", Enabled: true},      // counted
		{Key: "q", Value: "hello", Enabled: true},     // counted
		{Key: "debug", Value: "true", Enabled: false}, // disabled → not counted
		{Key: "", Value: "", Enabled: true},           // empty → not counted
	}
	if got := enabledParamCount(params); got != 2 {
		t.Fatalf("enabledParamCount = %d, want 2", got)
	}
	if got := tabBadge("Params", enabledParamCount(params)); got != "Params 2" {
		t.Errorf("tabBadge = %q, want %q", got, "Params 2")
	}
	if got := tabBadge("Headers", 0); got != "Headers" {
		t.Errorf("tabBadge with 0 = %q, want plain %q", got, "Headers")
	}
}

// The request editor must seed the Params/Headers sub-tab badges from the loaded
// Request, and update them when rows change and commit() runs.
func TestRequestTab_BadgeInitialAndRefresh(t *testing.T) {
	app := New(test.NewApp())
	coll := model.Collection{
		Version: 1,
		Name:    "Badges",
		Requests: []model.Request{{
			Method: model.MethodGet,
			URL:    "https://example.com",
			Params: []model.Param{
				{Key: "a", Value: "1", Enabled: true},
				{Key: "b", Value: "2", Enabled: true},
				{Key: "c", Value: "3", Enabled: false},
			},
			Headers: []model.Param{
				{Key: "X-Trace", Value: "z", Enabled: true},
			},
		}},
	}
	w := app.OpenCollectionWindow(coll, "/tmp/badges.yon")
	w.openRequestTab(0)
	rt := w.openTabs[0]

	if rt.paramsTab.Text != "Params 2" {
		t.Errorf("initial Params badge = %q, want %q", rt.paramsTab.Text, "Params 2")
	}
	if rt.headerTab.Text != "Headers 1" {
		t.Errorf("initial Headers badge = %q, want %q", rt.headerTab.Text, "Headers 1")
	}

	// Disable a param row and commit: the badge must drop to "Params 1".
	rt.paramsTable.rows[0].enabled.SetChecked(false) // fires commit via OnChanged
	if rt.paramsTab.Text != "Params 1" {
		t.Errorf("after disabling a row, Params badge = %q, want %q", rt.paramsTab.Text, "Params 1")
	}
}

func colorsEqual(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}
