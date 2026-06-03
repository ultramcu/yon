package ui

import (
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// TestBlindStatusBar_ResolvesVarPath covers SPEC B: the bottom-right status bar
// shows "METHOD · path" with {{variables}} resolved. A collection variable
// server=https://167.99.78.232:8787 and a request URL {{server}}/usage must
// render as "GET · /usage" with no remaining "{{".
func TestBlindStatusBar_ResolvesVarPath(t *testing.T) {
	coll := model.NewCollection("T")
	coll.Variables = []model.Variable{
		{Key: "server", Value: "https://167.99.78.232:8787", Enabled: true},
	}
	coll.Requests = []model.Request{
		{Method: model.MethodGet, URL: "{{server}}/usage"},
	}

	w := newScopeWindow(t, coll)
	w.openRequestTab(0)
	w.updateStatusBar()

	got := w.sbReqInfo.Text
	if got != "GET · /usage" {
		t.Fatalf("sbReqInfo.Text = %q, want %q", got, "GET · /usage")
	}
	if strings.Contains(got, "{{") {
		t.Fatalf("sbReqInfo.Text = %q still contains unresolved %q", got, "{{")
	}
}
