package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// Blind tests for Dev B's Variables-panel window integration (issue #29).
//
// Contract under test (package ui):
//   - Window gains fields `varsPanel *varsPanel` and `varsVisible bool`.
//   - (*Window).toggleVarsPanel() flips varsVisible and rebuilds the window
//     content: the right-side dock is shown when visible, hidden otherwise (the
//     hidden state looks like today's no-dock layout).
//   - (*Window).refreshVarsPanel() refreshes the panel ONLY when visible.
//
// Dev A's pieces are already landed (varspanel.go): newVarsPanel, (*varsPanel).
// refresh, collectVariableView, renderVarLine, secretMask, varsPanel.container.
//
// These tests are BLIND to Dev B's implementation. They pin the visible flag and
// the content change/restore for the toggle, the no-op-when-hidden guard for
// refresh, and — the valuable ones — that a runtime capture renders end-to-end
// and that a Secret variable is masked end-to-end (its clear value never leaks
// into the rendered panel).

// --- helpers -----------------------------------------------------------------

// newVarsTestWindow builds a Window on the headless test driver using the smoke
// pattern, with one simple request so the construction paths are exercised.
func newVarsTestWindow(t *testing.T) *Window {
	t.Helper()
	fyneApp := test.NewApp()
	app := New(fyneApp)

	coll := model.NewCollection("Vars")
	coll.Requests = append(coll.Requests, model.Request{
		Name:   "Ping",
		Method: model.MethodGet,
		URL:    "https://example.com/ping",
	})

	w := app.OpenCollectionWindow(coll, "/tmp/vars.yon")
	if w == nil {
		t.Fatal("OpenCollectionWindow returned nil")
	}
	return w
}

// collectPanelText walks p.container collecting the text of every *widget.Label
// and *canvas.Text it renders, joined with newlines. This is the rendered text
// the user sees in the panel. It reuses the package's walkObjects test helper,
// which descends *fyne.Container, *container.Scroll, and widget renderers.
func collectPanelText(p *varsPanel) string {
	var b strings.Builder
	walkObjects(fyne.CurrentApp(), p.container, func(o fyne.CanvasObject) {
		switch t := o.(type) {
		case *widget.Label:
			b.WriteString(t.Text)
			b.WriteString("\n")
		case *canvas.Text:
			b.WriteString(t.Text)
			b.WriteString("\n")
		}
	})
	return b.String()
}

// --- TestToggleVarsPanel ------------------------------------------------------

// TestToggleVarsPanel pins the toggle behaviour: starts hidden with a built
// panel, showing it changes the window content (the dock was added), hiding it
// again restores the no-dock content.
func TestToggleVarsPanel(t *testing.T) {
	w := newVarsTestWindow(t)

	// Starts hidden, but the panel object already exists (Dev A's newVarsPanel
	// is wired in at construction so refresh paths are always safe).
	if w.varsVisible {
		t.Errorf("varsVisible = true at startup, want false (dock hidden by default)")
	}
	if w.varsPanel == nil {
		t.Fatal("w.varsPanel is nil; expected the panel to be constructed up front")
	}

	before := w.win.Content()
	if before == nil {
		t.Fatal("window content is nil before toggle")
	}

	// Show the panel.
	w.toggleVarsPanel()
	if !w.varsVisible {
		t.Errorf("after first toggle varsVisible = false, want true")
	}
	shown := w.win.Content()
	if shown == nil {
		t.Fatal("window content is nil after showing the panel")
	}
	// The dock was added: either the content object identity changed or the panel
	// container now appears in the content tree. Assert at least one holds, so we
	// don't over-pin the exact widget tree.
	if shown == before && !contentContains(shown, w.varsPanel.container) {
		t.Errorf("showing the panel did not change the window content nor mount the panel container")
	}
	if !contentContains(shown, w.varsPanel.container) {
		t.Errorf("after showing, the panel container is not present in the window content tree")
	}

	// Hide the panel again.
	w.toggleVarsPanel()
	if w.varsVisible {
		t.Errorf("after second toggle varsVisible = true, want false")
	}
	hidden := w.win.Content()
	if hidden == nil {
		t.Fatal("window content is nil after hiding the panel")
	}
	if contentContains(hidden, w.varsPanel.container) {
		t.Errorf("after hiding, the panel container is still present in the window content tree")
	}
}

// contentContains reports whether target appears anywhere in root's rendered
// object tree (by pointer identity), using the package walkObjects helper.
func contentContains(root, target fyne.CanvasObject) bool {
	found := false
	walkObjects(fyne.CurrentApp(), root, func(o fyne.CanvasObject) {
		if o == target {
			found = true
		}
	})
	return found
}

// --- TestRefreshVarsPanelNoOpWhenHidden --------------------------------------

// TestRefreshVarsPanelNoOpWhenHidden asserts refreshVarsPanel is safe while the
// panel is hidden (no panic) and does not flip the panel visible; once shown,
// refresh is also safe to call.
func TestRefreshVarsPanelNoOpWhenHidden(t *testing.T) {
	w := newVarsTestWindow(t)

	if w.varsVisible {
		t.Fatalf("precondition: panel should start hidden")
	}

	// Hidden: refresh must not panic and must not reveal the panel.
	w.refreshVarsPanel()
	if w.varsVisible {
		t.Errorf("refreshVarsPanel made the panel visible while hidden; want it left hidden")
	}

	// Shown: refresh must be safe to call.
	w.toggleVarsPanel()
	if !w.varsVisible {
		t.Fatalf("toggleVarsPanel did not show the panel")
	}
	w.refreshVarsPanel() // expected: no panic
	if !w.varsVisible {
		t.Errorf("refreshVarsPanel hid the panel while shown; want it left visible")
	}
}

// --- TestRuntimeVarShownAfterRefresh -----------------------------------------

// TestRuntimeVarShownAfterRefresh is the end-to-end happy path: a session
// runtime capture appears in the panel's rendered text after a refresh while the
// panel is visible.
func TestRuntimeVarShownAfterRefresh(t *testing.T) {
	w := newVarsTestWindow(t)

	// Show the panel, then set a runtime capture and refresh.
	w.toggleVarsPanel()
	if !w.varsVisible {
		t.Fatalf("toggleVarsPanel did not show the panel")
	}
	w.runtimeVars = map[string]string{"userId": "42"}
	w.refreshVarsPanel()

	got := collectPanelText(w.varsPanel)
	if !strings.Contains(got, "userId") {
		t.Errorf("panel text does not contain captured key %q\npanel text:\n%s", "userId", got)
	}
	if !strings.Contains(got, "42") {
		t.Errorf("panel text does not contain captured value %q\npanel text:\n%s", "42", got)
	}

	// Cross-check against the pure render path so the assertion is anchored to the
	// shared source of truth even if the tree walk misses a widget kind: the
	// runtime row renders exactly "userId = 42" (runtime rows are never secret).
	line := renderVarLine(varView{Key: "userId", Value: "42", Scope: scopeRuntime})
	if line != "userId = 42" {
		t.Errorf("renderVarLine(runtime userId=42) = %q, want %q", line, "userId = 42")
	}
}

// --- TestSecretMaskedInPanel -------------------------------------------------

// TestSecretMaskedInPanel is the privacy end-to-end: with an active environment
// holding a Secret variable apiKey=topsecret, the rendered panel shows the key
// and the mask but NEVER the clear secret value.
func TestSecretMaskedInPanel(t *testing.T) {
	w := newVarsTestWindow(t)

	// Install an active environment with a Secret variable. activeEnv() matches
	// w.envs against w.coll.ActiveEnvironment by Name, so set both.
	w.envs = []model.Environment{{
		Name: "Prod",
		Variables: []model.Variable{
			{Key: "apiKey", Value: "topsecret", Enabled: true, Secret: true},
		},
	}}
	w.coll.ActiveEnvironment = "Prod"

	// Sanity: the active env really resolves (so the panel has something to show).
	if env, ok := w.activeEnv(); !ok || env.Name != "Prod" {
		t.Fatalf("activeEnv() = (%+v, %v), want the Prod env active", env, ok)
	}

	w.toggleVarsPanel()
	if !w.varsVisible {
		t.Fatalf("toggleVarsPanel did not show the panel")
	}
	w.refreshVarsPanel()

	got := collectPanelText(w.varsPanel)
	if !strings.Contains(got, "apiKey") {
		t.Errorf("panel text does not contain secret key %q\npanel text:\n%s", "apiKey", got)
	}
	if !strings.Contains(got, secretMask) {
		t.Errorf("panel text does not contain the mask %q\npanel text:\n%s", secretMask, got)
	}
	if strings.Contains(got, "topsecret") {
		t.Fatalf("SECRET LEAK: panel text contains the clear secret value %q\npanel text:\n%s", "topsecret", got)
	}

	// Anchor to the shared render path: a Secret row renders masked, not clear.
	line := renderVarLine(varView{Key: "apiKey", Value: "topsecret", Secret: true, Scope: scopeEnv})
	if strings.Contains(line, "topsecret") {
		t.Errorf("renderVarLine leaked the secret value: %q", line)
	}
	if !strings.Contains(line, secretMask) {
		t.Errorf("renderVarLine did not mask the secret: %q", line)
	}
}
