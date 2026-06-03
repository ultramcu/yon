package ui

import (
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// TestStatusBar_ResolvesTemplatesInPath pins that the bottom-right status bar
// shows the real path with {{variables}} replaced by their values, not the
// literal {{key}} template.
func TestStatusBar_ResolvesTemplatesInPath(t *testing.T) {
	coll := model.NewCollection("T")
	coll.Variables = []model.Variable{{Key: "server", Value: "https://167.99.78.232:8787", Enabled: true}}
	coll.Requests = []model.Request{{Method: model.MethodGet, URL: "{{server}}/usage"}}

	w := newScopeWindow(t, coll)
	w.openRequestTab(0) // selecting the tab refreshes the status bar
	w.updateStatusBar()

	got := w.sbReqInfo.Text
	if strings.Contains(got, "{{") {
		t.Fatalf("status bar still shows a template: %q", got)
	}
	if got != "GET · /usage" {
		t.Fatalf("status bar = %q, want %q", got, "GET · /usage")
	}
}
