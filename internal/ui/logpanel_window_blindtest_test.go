package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// Blind tests for Dev B's Request-Log window integration (issue #30).
//
// Contract under test (package ui), mirroring the SIBLING Variables panel:
//   - Window gains `reqLog []logEntry`, `logPanel *logPanel`, `logVisible bool`,
//     `logToggle *canvas.Text`; consts maxLogEntries = 1000, prefKeyLogPanelOpen.
//   - (*Window).appendLog(e) appends, caps at maxLogEntries dropping the OLDEST,
//     and refreshes the panel if visible.
//   - (*Window).clearLog() empties reqLog and refreshes.
//   - (*Window).toggleLogPanel() flips logVisible, rebuilds the content (the
//     bottom dock shown/hidden), and persists the open/closed state.
//   - (*Window).refreshLogPanel() refreshes ONLY when visible.
//   - A status-bar "Log" toggle (a newTappable wrapping w.logToggle) AND a
//     View ▸ Request Log menu item, both calling toggleLogPanel.
//
// Dev A's pieces are already landed (logpanel.go): logEntry, logPanel,
// newLogPanel, formatLog, (*logPanel).refresh.
//
// These tests are BLIND to Dev B's implementation. They reuse the existing
// tree-walk helpers from the Variables-panel tests (walkObjects via
// contentContains, plus canvasTreeContains / findTappableFor) — they are NOT
// redefined here.

// --- helpers -----------------------------------------------------------------

// newLogTestWindow builds a Window on the headless test driver using the smoke
// pattern, with one simple request so the construction paths are exercised.
func newLogTestWindow(t *testing.T) *Window {
	t.Helper()
	fyneApp := test.NewApp()
	app := New(fyneApp)

	coll := model.NewCollection("Log")
	coll.Requests = append(coll.Requests, model.Request{
		Name:   "Ping",
		Method: model.MethodGet,
		URL:    "https://example.com/ping",
	})

	w := app.OpenCollectionWindow(coll, "/tmp/log.yon")
	if w == nil {
		t.Fatal("OpenCollectionWindow returned nil")
	}
	return w
}

// --- TestAppendLogCaps --------------------------------------------------------

// TestAppendLogCaps pins the bounded-log behaviour: appending more than
// maxLogEntries entries keeps exactly maxLogEntries, dropping the OLDEST. Each
// entry's Name carries its append index so we can prove the first surviving
// entry is index 5 (the oldest 5 were dropped) and the last is the newest.
func TestAppendLogCaps(t *testing.T) {
	w := newLogTestWindow(t)

	if len(w.reqLog) != 0 {
		t.Fatalf("precondition: reqLog should start empty, got %d entries", len(w.reqLog))
	}

	total := maxLogEntries + 5
	for i := 0; i < total; i++ {
		w.appendLog(logEntry{Name: itoa(i), Method: "GET", URL: "https://example.com/"})
	}

	if len(w.reqLog) != maxLogEntries {
		t.Fatalf("after appending %d entries, len(reqLog) = %d, want cap %d",
			total, len(w.reqLog), maxLogEntries)
	}

	// The oldest 5 (indices 0..4) were dropped, so the first surviving entry is
	// the one appended at index 5, and the last is the newest (total-1).
	if got, want := w.reqLog[0].Name, itoa(5); got != want {
		t.Errorf("oldest surviving entry Name = %q, want %q (oldest 5 should be dropped)", got, want)
	}
	if got, want := w.reqLog[len(w.reqLog)-1].Name, itoa(total-1); got != want {
		t.Errorf("newest entry Name = %q, want %q (newest kept last)", got, want)
	}
}

// --- TestToggleLogPanel -------------------------------------------------------

// TestToggleLogPanel pins the toggle behaviour, mirroring TestToggleVarsPanel:
// starts hidden with a built panel; showing it mounts the panel container into
// the window content; hiding it removes the container (restoring the no-dock
// layout).
func TestToggleLogPanel(t *testing.T) {
	w := newLogTestWindow(t)

	if w.logVisible {
		t.Errorf("logVisible = true at startup, want false (dock hidden by default)")
	}
	if w.logPanel == nil {
		t.Fatal("w.logPanel is nil; expected the panel to be constructed up front")
	}

	before := w.win.Content()
	if before == nil {
		t.Fatal("window content is nil before toggle")
	}
	// Sanity: hidden layout does not contain the log container.
	if contentContains(before, w.logPanel.container) {
		t.Fatal("log panel container present while dock is hidden at startup")
	}

	// Show the panel.
	w.toggleLogPanel()
	if !w.logVisible {
		t.Errorf("after first toggle logVisible = false, want true")
	}
	shown := w.win.Content()
	if shown == nil {
		t.Fatal("window content is nil after showing the panel")
	}
	if shown == before && !contentContains(shown, w.logPanel.container) {
		t.Errorf("showing the panel did not change the window content nor mount the panel container")
	}
	if !contentContains(shown, w.logPanel.container) {
		t.Errorf("after showing, the log panel container is not present in the window content tree")
	}

	// Hide the panel again — content returns to the no-dock layout.
	w.toggleLogPanel()
	if w.logVisible {
		t.Errorf("after second toggle logVisible = true, want false")
	}
	hidden := w.win.Content()
	if hidden == nil {
		t.Fatal("window content is nil after hiding the panel")
	}
	if contentContains(hidden, w.logPanel.container) {
		t.Errorf("after hiding, the log panel container is still present in the window content tree")
	}
}

// --- TestLogToggleAffordanceWired --------------------------------------------

// TestLogToggleAffordanceWired is the key test: a sibling feature shipped
// unreachable, so we assert BOTH on-screen handles to the dock are wired.
//
//  1. w.logToggle exists, lives in the status-bar (w.contentBottom) tree wrapped
//     by a tappable, and invoking that tappable's Tapped flips logVisible (and
//     a second tap flips it back).
//  2. buildMainMenu() has a "View" menu containing a "Request Log" item whose
//     Action flips logVisible.
func TestLogToggleAffordanceWiredBlind(t *testing.T) {
	w := newLogTestWindow(t)

	// (1) The footer toggle.
	if w.logToggle == nil {
		t.Fatal("w.logToggle is nil: buildStatusBar never created the footer Log toggle")
	}
	if !canvasTreeContains(w.contentBottom, w.logToggle) {
		t.Fatal("status bar does not contain w.logToggle: the toggle isn't shown in the footer")
	}
	tap := findTappableFor(w.contentBottom, w.logToggle)
	if tap == nil {
		t.Fatal("no tappable wraps w.logToggle: the footer label is not clickable")
	}

	before := w.logVisible
	tap.Tapped(&fyne.PointEvent{})
	if w.logVisible == before {
		t.Fatal("tapping the footer Log toggle did not flip logVisible")
	}
	tap.Tapped(&fyne.PointEvent{})
	if w.logVisible != before {
		t.Fatal("second tap did not flip logVisible back")
	}

	// (2) The View ▸ Request Log menu item.
	mm := w.buildMainMenu()
	var view *fyne.Menu
	for _, m := range mm.Items {
		if m.Label == "View" {
			view = m
			break
		}
	}
	if view == nil {
		t.Fatal("no View menu: View ▸ Request Log affordance is missing")
	}
	var item *fyne.MenuItem
	for _, it := range view.Items {
		if it.Label == "Request Log" {
			item = it
			break
		}
	}
	if item == nil {
		t.Fatal("View menu has no Request Log item")
	}
	if item.Action == nil {
		t.Fatal("View ▸ Request Log has a nil Action: clicking it would do nothing")
	}

	mbefore := w.logVisible
	item.Action()
	if w.logVisible == mbefore {
		t.Fatal("invoking View ▸ Request Log did not flip logVisible")
	}
}

// --- TestClearLog -------------------------------------------------------------

// TestClearLog pins that clearLog empties the log (the Clear button path).
func TestClearLog(t *testing.T) {
	w := newLogTestWindow(t)

	w.appendLog(logEntry{Name: "a", Method: "GET", URL: "https://example.com/a"})
	w.appendLog(logEntry{Name: "b", Method: "GET", URL: "https://example.com/b"})
	if len(w.reqLog) == 0 {
		t.Fatalf("precondition: expected entries after appends, got 0")
	}

	w.clearLog()
	if len(w.reqLog) != 0 {
		t.Fatalf("after clearLog, len(reqLog) = %d, want 0", len(w.reqLog))
	}
}

// --- TestRefreshLogPanelNoOpWhenHidden ---------------------------------------

// TestRefreshLogPanelNoOpWhenHidden asserts refreshLogPanel is safe while the
// dock is hidden (no panic) and never reveals the panel; once shown it stays
// safe and leaves the panel visible. Mirrors the Variables sibling's guard.
func TestRefreshLogPanelNoOpWhenHidden(t *testing.T) {
	w := newLogTestWindow(t)

	if w.logVisible {
		t.Fatalf("precondition: dock should start hidden")
	}

	w.refreshLogPanel() // hidden: must not panic, must not reveal
	if w.logVisible {
		t.Errorf("refreshLogPanel made the dock visible while hidden; want it left hidden")
	}

	w.toggleLogPanel()
	if !w.logVisible {
		t.Fatalf("toggleLogPanel did not show the dock")
	}
	w.refreshLogPanel() // shown: must not panic, must stay visible
	if !w.logVisible {
		t.Errorf("refreshLogPanel hid the dock while shown; want it left visible")
	}
}

// --- TestLogPrefPersists ------------------------------------------------------

// TestLogPrefPersists pins persistence: toggling the dock open sets the pref
// true, and a FRESH window built afterwards seeds open from that pref. Both
// windows share the same fyne.App so they share Preferences.
func TestLogPrefPersists(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	// First window: dock starts closed; toggling it open should persist true.
	w1 := app.OpenCollectionWindow(model.NewCollection("Log"), "/tmp/log1.yon")
	if w1.logVisible {
		t.Fatalf("first window opened with the dock already visible, want hidden default")
	}
	w1.toggleLogPanel()
	if !w1.logVisible {
		t.Fatalf("toggleLogPanel did not open the dock")
	}
	if !app.prefs().BoolWithFallback(prefKeyLogPanelOpen, false) {
		t.Fatalf("opening the dock did not persist %s = true", prefKeyLogPanelOpen)
	}

	// A new window (same app/prefs) seeds the dock open from the pref.
	w2 := app.OpenCollectionWindow(model.NewCollection("Log2"), "/tmp/log2.yon")
	if !w2.logVisible {
		t.Fatal("pref-seeded window did not open the Request Log dock")
	}
}

// --- TestLogComposesWithVarsPanel (optional) ---------------------------------

// TestLogComposesWithVarsPanel opens BOTH the right-side Variables dock and the
// bottom Request Log dock and asserts the content tree contains both panel
// containers at once (a vertical split wrapping the horizontal one), with no
// panic. This guards the two sibling docks composing rather than clobbering.
func TestLogComposesWithVarsPanelBlind(t *testing.T) {
	w := newLogTestWindow(t)

	if w.varsPanel == nil {
		t.Fatal("w.varsPanel is nil; expected the Variables panel constructed up front")
	}
	if w.logPanel == nil {
		t.Fatal("w.logPanel is nil; expected the Log panel constructed up front")
	}

	w.toggleVarsPanel()
	w.toggleLogPanel()
	if !w.varsVisible || !w.logVisible {
		t.Fatalf("expected both docks visible, got varsVisible=%v logVisible=%v",
			w.varsVisible, w.logVisible)
	}

	content := w.win.Content()
	if content == nil {
		t.Fatal("window content is nil with both docks open")
	}
	if !contentContains(content, w.varsPanel.container) {
		t.Error("with both docks open, the Variables panel container is missing from the content tree")
	}
	if !contentContains(content, w.logPanel.container) {
		t.Error("with both docks open, the Log panel container is missing from the content tree")
	}

	// Closing the log dock must leave the vars dock intact (composition is not
	// mutually exclusive).
	w.toggleLogPanel()
	if w.logVisible {
		t.Fatal("toggling the log dock did not close it")
	}
	content = w.win.Content()
	if contentContains(content, w.logPanel.container) {
		t.Error("after closing the log dock, its container is still in the content tree")
	}
	if !w.varsVisible || !contentContains(content, w.varsPanel.container) {
		t.Error("closing the log dock disturbed the still-open Variables dock")
	}
}
