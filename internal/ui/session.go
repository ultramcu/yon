package ui

import (
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/ultramcu/yon/internal/store"
)

// prefKeySession is the Preferences key holding the serialized Session.
const prefKeySession = "session.v1"

// sessionWindow is the persisted state of one open Collection window: its file
// path, the Collection-request indices of its open Tabs (in tab order), and
// which open Tab was active.
type sessionWindow struct {
	Path        string `json:"path"`
	OpenTabs    []int  `json:"openTabs"`
	ActiveIndex int    `json:"activeIndex"` // index into OpenTabs, -1 if none
}

// session is the whole persisted snapshot: every open, saved Collection window.
// Untitled (never-saved) windows are not remembered — there is no file to
// reopen.
type session struct {
	Windows []sessionWindow `json:"windows"`
}

// saveSession captures the current windows and stores the Session in
// Preferences. Only windows with a backing .yon path are remembered. It does
// NOT persist unsaved in-memory edits (the file on disk is the source of truth
// for restore); on-close save prompting handles unsaved work.
func (a *App) saveSession() {
	var s session
	for w := range a.windows {
		if w.path == "" {
			continue // untitled, nothing to reopen
		}
		s.Windows = append(s.Windows, w.sessionState())
	}

	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	a.prefs().SetString(prefKeySession, string(data))
}

// sessionState snapshots one window: its open tabs in tab order and the active
// one. The tab order is the DocTabs order; we map each Tab back to its
// Collection-request index.
func (w *Window) sessionState() sessionWindow {
	sw := sessionWindow{Path: w.path, ActiveIndex: -1}

	// Map each *requestTab to its position in the DocTabs Items slice so the
	// persisted order matches the on-screen order.
	tabToIdx := make(map[*requestTab]int, len(w.openTabs))
	for idx, rt := range w.openTabs {
		tabToIdx[rt] = idx
	}

	selected := w.tabs.Selected()
	for _, item := range w.tabs.Items {
		// Find the requestTab whose tab == item.
		for rt, idx := range tabToIdx {
			if rt.tab == item {
				if item == selected {
					sw.ActiveIndex = len(sw.OpenTabs)
				}
				sw.OpenTabs = append(sw.OpenTabs, idx)
				break
			}
		}
	}
	return sw
}

// restoreSession reopens the Collections and Tabs from the last Session. It
// returns true if at least one window was restored. Files that no longer exist
// (or fail to load) are skipped with a non-blocking notice rather than aborting
// startup (CONTEXT: "skipped with a notice rather than blocking startup").
func (a *App) restoreSession() bool {
	raw := a.prefs().String(prefKeySession)
	if raw == "" {
		return false
	}
	var s session
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return false
	}

	var skipped []string
	restored := false
	for _, sw := range s.Windows {
		if sw.Path == "" {
			continue
		}
		coll, err := store.Load(sw.Path)
		if err != nil {
			skipped = append(skipped, sw.Path)
			continue
		}
		w := a.OpenCollectionWindow(coll, sw.Path)
		w.restoreTabs(sw, len(coll.Requests))
		restored = true
	}

	if len(skipped) > 0 {
		notifySkipped(a, skipped)
	}
	return restored
}

// restoreTabs reopens the Tabs recorded for this window and selects the active
// one. Indices that are out of range for the current Collection (the file may
// have changed since the Session was saved) are skipped.
func (w *Window) restoreTabs(sw sessionWindow, requestCount int) {
	for _, idx := range sw.OpenTabs {
		if idx < 0 || idx >= requestCount {
			continue
		}
		w.openRequestTab(idx)
	}
	// Select the recorded active tab; if its request index is now out of range
	// (the file changed since save), fall back to the first tab that did open so
	// a tab is always focused rather than leaving a mismatched default.
	selected := false
	if sw.ActiveIndex >= 0 && sw.ActiveIndex < len(sw.OpenTabs) {
		if rt, ok := w.openTabs[sw.OpenTabs[sw.ActiveIndex]]; ok {
			w.tabs.Select(rt.tab)
			selected = true
		}
	}
	if !selected && len(w.tabs.Items) > 0 {
		w.tabs.SelectIndex(0)
	}
}

// notifySkipped tells the user which remembered files could not be reopened,
// without blocking startup. Shown on the first restored window if any, else as
// a standalone information dialog on a throwaway window.
func notifySkipped(a *App, paths []string) {
	msg := "Some remembered Collections could not be reopened:\n"
	for _, p := range paths {
		msg += "\n• " + p
	}
	// Attach to any live window; if none, create a tiny one to host the notice.
	var parent fyne.Window
	for w := range a.windows {
		parent = w.win
		break
	}
	if parent == nil {
		parent = a.fyneApp.NewWindow("Yon")
		parent.Resize(fyne.NewSize(480, 200))
		parent.Show()
	}
	dialog.ShowInformation("Session restore", msg, parent)
}
