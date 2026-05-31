// Package ui is Yon's Fyne front end: the only package besides main that
// imports Fyne (ADR-0002). It renders Collections as windows, Requests as
// DocTabs, and Responses in a read-only TextGrid (ADR-0001), translating
// between Fyne widgets and the pure model/yonner/store packages.
package ui

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// AppID is the Fyne application identifier; it scopes Preferences storage
// (session + settings) and must match the ID passed to app.NewWithID.
const AppID = "com.ultramcu.yon"

// App is the top-level UI controller. It owns the Fyne app, the live set of
// Collection windows, and the persisted Settings, and coordinates session
// save/restore.
type App struct {
	fyneApp fyne.App

	settings Settings

	// windows holds every open Collection window keyed by its *Window so we can
	// enumerate them for session save and remove them on close.
	windows map[*Window]struct{}
}

// New constructs the UI controller around an already-created fyne.App (created
// in main with app.NewWithID(AppID)). It loads persisted Settings immediately
// so the first send already honours the user's timeout / TLS choices.
func New(a fyne.App) *App {
	a.SetIcon(appIcon)
	app := &App{
		fyneApp:  a,
		settings: loadSettings(a.Preferences()),
		windows:  make(map[*Window]struct{}),
	}
	app.applyTheme() // apply the saved appearance theme before any window is built
	return app
}

// applyTheme sets the Fyne app theme from the current Settings (Dark Pro / Warm
// / System). Called at startup and whenever the user changes it in Settings.
func (a *App) applyTheme() {
	a.fyneApp.Settings().SetTheme(themeByID(a.settings.ThemeID))
}

// Run starts the Fyne event loop after deciding what to open. If one or more
// .yon file paths are given (e.g. from the command line), those Collections are
// opened — one window each — and the saved Session is NOT restored (the explicit
// files take precedence). With no file arguments, Run restores the previous
// Session, or opens one fresh untitled Collection if there is none. An OnStopped
// hook saves the Session on shutdown. Run blocks until the app quits.
func (a *App) Run(files ...string) {
	a.fyneApp.Lifecycle().SetOnStopped(func() {
		a.saveSession()
	})

	if len(files) > 0 {
		opened := 0
		for _, f := range files {
			if err := a.OpenPath(f); err != nil {
				fmt.Fprintf(os.Stderr, "yon: cannot open %q: %v\n", f, err)
				continue
			}
			opened++
		}
		if opened == 0 {
			a.NewCollectionWindow()
		}
	} else if !a.restoreSession() {
		a.NewCollectionWindow()
	}

	a.fyneApp.Run()
}

// OpenPath loads a Collection from a .yon file and opens it in its own window.
// It returns an error if the file cannot be read or is not a valid .yon file
// (callers decide whether to surface it via a dialog or stderr).
func (a *App) OpenPath(path string) error {
	coll, err := store.Load(path)
	if err != nil {
		return err
	}
	a.OpenCollectionWindow(coll, path)
	a.rememberRecent(path)
	return nil
}

// NewCollectionWindow opens a window for a fresh, untitled Collection and shows
// it. Returns the new Window so callers (e.g. session restore) can drive it.
func (a *App) NewCollectionWindow() *Window {
	coll := model.NewCollection("")
	w := newWindow(a, coll, "")
	a.windows[w] = struct{}{}
	w.win.Show()
	return w
}

// OpenCollectionWindow opens a window for a Collection already loaded from disk
// at path. Returns the Window.
func (a *App) OpenCollectionWindow(coll model.Collection, path string) *Window {
	w := newWindow(a, coll, path)
	a.windows[w] = struct{}{}
	w.win.Show()
	return w
}

// forgetWindow removes a closed window from the live set so it is not included
// in the next session save.
func (a *App) forgetWindow(w *Window) {
	delete(a.windows, w)
}

// prefs is a small accessor used across the ui package.
func (a *App) prefs() fyne.Preferences { return a.fyneApp.Preferences() }
