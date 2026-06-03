package ui

import (
	"fmt"
	"image/color"
	"net/url"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
	"github.com/ultramcu/yon/internal/updater"
)

// Window hosts exactly one model.Collection: a sidebar listing its Requests and
// a DocTabs area showing the open Requests as Tabs. It owns the Collection's
// in-memory state, its on-disk path (empty when untitled), and its dirty flag.
type Window struct {
	app  *App
	win  fyne.Window
	coll model.Collection

	// path is the .yon file backing this Collection, or "" if never saved.
	path string

	// dirty is true when the in-memory Collection differs from disk.
	dirty bool

	sidebar *widget.List
	tabs    *tabStrip

	// Bottom status bar (mockup-v9): the active tab's last response status/time/
	// size on the left, its method · path on the right.
	sbStatus  *canvas.Text
	sbMeta    *canvas.Text
	sbReqInfo *canvas.Text

	// sidebarCount is the "N requests" badge in the collection header; kept so
	// add/delete can refresh the number without rebuilding the header.
	sidebarCount *widget.Label

	// sidebarTitle is the collection-name label in the header; kept so
	// updateTitle can refresh it (e.g. after Save As changes the file/name)
	// without rebuilding the header.
	sidebarTitle *widget.Label

	// selectedID is the currently-selected sidebar row (-1 = none); a row paints
	// its cyan left-accent bar when its id matches, on top of the List's own
	// selection tint.
	selectedID widget.ListItemID

	// openTabs maps a Collection request index to its open editor Tab, so we
	// don't open the same Request twice and can find a Tab to refresh/select.
	openTabs map[int]*requestTab

	// envs holds the environments loaded from the collection's sibling files
	// (empty for an unsaved collection). envSelect is the sidebar-header picker
	// that chooses the active environment; pendingEnvDeletes queues environment
	// files to remove on the next manager Save (set by Rename/Delete).
	envs              []model.Environment
	envSelect         *widget.Select
	pendingEnvDeletes []string

	// updateBanner is a dismissible notice shown at the top of the window when a
	// newer release is found; updateLabel is its text, and pendingRel/pendingAsset
	// hold the release whose asset the Download button fetches.
	updateBanner *fyne.Container
	updateLabel  *widget.Label
	pendingRel   updater.Release
	pendingAsset updater.Asset
}

// newWindow builds (but does not Show) a Window for coll. path is "" for an
// untitled Collection.
func newWindow(app *App, coll model.Collection, path string) *Window {
	w := &Window{
		app:        app,
		coll:       coll,
		path:       path,
		openTabs:   make(map[int]*requestTab),
		selectedID: -1,
	}
	w.win = app.fyneApp.NewWindow("")
	w.win.SetIcon(appIcon)

	w.loadEnvironments()
	w.buildSidebar()
	w.buildTabs()
	w.buildMenu()

	split := container.NewHSplit(
		container.NewBorder(w.buildSidebarHeader(), nil, nil, nil, w.sidebar),
		w.tabs.object(),
	)
	split.SetOffset(0.22)
	w.win.SetContent(container.NewBorder(w.buildUpdateBanner(), w.buildStatusBar(), nil, nil, split))
	w.updateStatusBar()

	// Cmd/Ctrl+F opens find on the active response (also in Edit ▸ Find…); Esc
	// closes it. These fire when focus is on the response area; the Edit menu and
	// the find field's own handlers cover the cases where an Entry holds focus.
	w.win.Canvas().AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault},
		func(fyne.Shortcut) {
			if rt := w.activeTab(); rt != nil {
				rt.response.openFind()
			}
		},
	)
	w.win.Canvas().AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyEscape},
		func(fyne.Shortcut) {
			if rt := w.activeTab(); rt != nil {
				rt.response.closeFind()
			}
		},
	)
	// Cmd/Ctrl+W closes the active request tab (the standard close-tab shortcut;
	// not Cmd+Q, which macOS reserves for Quit). Fires when focus is on the tab /
	// response area; clicking a tab's × always works regardless of focus.
	w.win.Canvas().AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyW, Modifier: fyne.KeyModifierShortcutDefault},
		func(fyne.Shortcut) {
			if rt := w.activeTab(); rt != nil {
				w.closeTab(rt.tab)
			}
		},
	)
	w.win.Resize(fyne.NewSize(1100, 720))

	w.win.SetCloseIntercept(w.onCloseRequested)
	w.updateTitle()
	return w
}

// buildSidebarHeader is the collection header above the request list: a folder
// icon + collection name, a count badge of how many Requests it holds, and a
// compact "Add" button. Matches the v2 mockup's "Yon Test Server  [13]" row.
func (w *Window) buildSidebarHeader() fyne.CanvasObject {
	w.sidebarTitle = widget.NewLabelWithStyle(
		collectionDisplayName(w.coll.Name, w.path),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title := w.sidebarTitle

	w.sidebarCount = widget.NewLabel("")
	w.refreshSidebarCount()

	add := widget.NewButtonWithIcon("", theme.ContentAddIcon(), w.addRequest)
	add.Importance = widget.LowImportance

	folder := widget.NewIcon(theme.FolderIcon())
	header := container.NewBorder(
		nil, nil,
		container.NewHBox(folder, title),
		container.NewHBox(w.sidebarCount, add),
		nil,
	)

	// Save / Save As toolbar row under the collection title.
	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() { w.save(nil) })
	saveBtn.Importance = widget.LowImportance
	saveAsBtn := widget.NewButton("Save As…", func() { w.saveAs(nil) })
	saveAsBtn.Importance = widget.LowImportance
	toolbar := container.NewHBox(saveBtn, saveAsBtn)

	// Environment picker row: a label + the compact selector ("No Environment" +
	// each environment + "Manage Environments…").
	envRow := container.NewBorder(nil, nil,
		widget.NewLabel("Env"), nil,
		w.buildEnvSelector(),
	)

	return container.NewVBox(header, toolbar, envRow, widget.NewSeparator())
}

// refreshSidebarCount updates the request-count badge in the collection header.
func (w *Window) refreshSidebarCount() {
	if w.sidebarCount == nil {
		return
	}
	w.sidebarCount.SetText(fmt.Sprintf("%d", len(w.coll.Requests)))
}

// buildSidebar creates the request list. Each row is a coloured UPPERCASE verb
// tag (per methodColor) plus the Request's DisplayName, with a thin cyan accent
// bar marking the selected row. Selecting a row opens that Request as a Tab
// (single-click open). The List stays virtualized: the row is built once in
// CreateItem and recoloured/relabelled per id in UpdateItem.
func (w *Window) buildSidebar() {
	w.sidebar = widget.NewList(
		func() int { return len(w.coll.Requests) },
		func() fyne.CanvasObject { return newVerbRow() },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id < 0 || id >= len(w.coll.Requests) {
				return
			}
			o.(*verbRow).set(w.coll.Requests[id], id == w.selectedID)
		},
	)
	w.sidebar.OnSelected = func(id widget.ListItemID) {
		w.selectedID = id
		w.sidebar.Refresh() // repaint accent bars
		w.openRequestTab(id)
	}
	w.sidebar.OnUnselected = func(widget.ListItemID) {
		w.selectedID = -1
		w.sidebar.Refresh()
	}
}

// buildTabs creates the custom browser-card tab strip holding open Request
// editors. Closing a card detaches its editor; the underlying Request stays in
// the Collection. Selecting a card refreshes the status bar.
func (w *Window) buildTabs() {
	w.tabs = newTabStrip()
	w.tabs.OnSelected = func(*tabCard) { w.updateStatusBar() }
	w.tabs.OnClose = w.closeTab
}

// openRequestTab opens (or re-selects) the Request at Collection index idx as a
// Tab. Idempotent: re-selecting an already-open Request just focuses its Tab.
func (w *Window) openRequestTab(idx int) {
	if idx < 0 || idx >= len(w.coll.Requests) {
		return
	}
	if rt, ok := w.openTabs[idx]; ok {
		w.tabs.Select(rt.tab)
		return
	}
	rt := newRequestTab(w, idx)
	w.openTabs[idx] = rt
	w.tabs.Append(rt.tab)
	w.tabs.Select(rt.tab)
}

// closeTab removes a card and forgets its editor. Called from the strip's
// OnClose when a card's "×" is tapped.
func (w *Window) closeTab(card *tabCard) {
	closedIdx := -1
	for idx, rt := range w.openTabs {
		if rt.tab == card {
			rt.cancelInFlight()
			delete(w.openTabs, idx)
			closedIdx = idx
			break
		}
	}
	w.tabs.Remove(card)
	// Drop the sidebar selection if it pointed at the closed tab, so its row can
	// be clicked again to reopen it — widget.List.Select no-ops on the already-
	// selected row, which would otherwise leave the request un-reopenable.
	if closedIdx >= 0 && closedIdx == w.selectedID {
		w.selectedID = -1
		w.sidebar.Unselect(closedIdx)
	}
	w.updateStatusBar()
}

// addRequest appends a new empty Request to the Collection and opens it.
func (w *Window) addRequest() {
	req := model.Request{
		Method: model.MethodGet,
		Auth:   model.Auth{Kind: model.AuthInherit},
		Body:   model.Body{Type: model.BodyNone},
	}
	w.coll.Requests = append(w.coll.Requests, req)
	w.markDirty()
	w.sidebar.Refresh()
	w.refreshSidebarCount()
	// Select via the sidebar (not openRequestTab directly) so the new row's
	// selection + cyan accent stay in sync with the opened tab.
	w.sidebar.Select(len(w.coll.Requests) - 1)
}

// commitRequest writes an edited Request back into the Collection at idx and
// refreshes the sidebar label + Tab title. Called by the editor on every change.
func (w *Window) commitRequest(idx int, req model.Request) {
	if idx < 0 || idx >= len(w.coll.Requests) {
		return
	}
	w.coll.Requests[idx] = req
	w.markDirty()
	w.sidebar.Refresh()
	if rt, ok := w.openTabs[idx]; ok {
		rt.tab.setRequest(req.Method, req.DisplayName(), rt.dirty)
	}
}

// markDirty flags unsaved changes and updates the title bar.
func (w *Window) markDirty() {
	if !w.dirty {
		w.dirty = true
		w.updateTitle()
	}
}

// collectionDisplayName is the human-readable name shown for a Collection in
// the window title and the sidebar header: the Collection's own Name, else the
// file's base name when it was loaded/saved to disk, else "Untitled".
func collectionDisplayName(name, path string) string {
	switch {
	case name != "":
		return name
	case path != "":
		return filepath.Base(path)
	default:
		return "Untitled"
	}
}

// updateTitle reflects the Collection name, file, and dirty marker in the window
// title and keeps the sidebar header label in sync (e.g. after Save As changes
// the file the empty-name fallback resolves to).
func (w *Window) updateTitle() {
	name := collectionDisplayName(w.coll.Name, w.path)
	if w.sidebarTitle != nil {
		w.sidebarTitle.SetText(name)
	}
	marker := ""
	if w.dirty {
		marker = " •"
	}
	w.win.SetTitle(fmt.Sprintf("Yon — %s%s", name, marker))
}

// onCloseRequested handles the window close button: if dirty, prompt to save.
func (w *Window) onCloseRequested() {
	if !w.dirty {
		w.finishClose()
		return
	}
	dialog.ShowConfirm("Unsaved changes",
		"Save changes to this Collection before closing?",
		func(save bool) {
			if save {
				w.save(func(ok bool) {
					if ok {
						w.finishClose()
					}
					// if save was cancelled/failed, keep the window open.
				})
				return
			}
			w.finishClose()
		}, w.win)
}

// finishClose tears down the window: cancels in-flight sends and drops it from
// the app's live set so it leaves the next session save.
func (w *Window) finishClose() {
	for _, rt := range w.openTabs {
		rt.cancelInFlight()
	}
	w.app.forgetWindow(w)
	w.win.Close()
}

// ---- Menu + file actions ----

// saveShortcut / saveAsShortcut are the Save (Cmd/Ctrl+S) and Save As
// (Cmd/Ctrl+Shift+S) accelerators. Unlike Find / Copy / Paste (see buildMainMenu),
// Save can safely be a menu accelerator: nothing in a focused Entry or pop-out
// window binds Cmd+S, so there is no shortcut to hijack — and a menu accelerator
// (unlike a canvas shortcut) still fires while a text field is focused, which is
// exactly when the user reaches for Save.
var (
	saveShortcut   = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault}
	saveAsShortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}
)

func (w *Window) buildMenu() {
	w.win.SetMainMenu(w.buildMainMenu())
}

// buildMainMenu assembles the window's menu bar. Split from buildMenu (which
// installs it) so the menu — and its Save/Save As accelerators — is unit-testable.
func (w *Window) buildMainMenu() *fyne.MainMenu {
	saveItem := fyne.NewMenuItem("Save", func() { w.save(nil) })
	saveItem.Shortcut = saveShortcut
	saveAsItem := fyne.NewMenuItem("Save As…", func() { w.saveAs(nil) })
	saveAsItem.Shortcut = saveAsShortcut
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("New", func() { w.app.NewCollectionWindow() }),
		fyne.NewMenuItem("Open…", w.open),
		fyne.NewMenuItem("Import Collection (JSON)…", w.importCollection),
		w.recentMenuItem(),
		fyne.NewMenuItemSeparator(),
		saveItem,
		saveAsItem,
	)
	// No accelerators on Find / Copy / Paste: on macOS a main-menu accelerator is
	// registered app-globally and bound to the menu's window, so it would hijack
	// Cmd+F / Cmd+C / Cmd+V from a focused pop-out window or Entry and act on the
	// wrong target. Those are handled per-window instead — Cmd+F via each window's
	// canvas/find-field, Cmd+C/V by the focused Entry itself — so these menu items
	// provide discoverable, mouse-driven access. (Save is exempt; see saveShortcut.)
	editMenu := fyne.NewMenu("Edit",
		fyne.NewMenuItem("Copy", w.editCopy),
		fyne.NewMenuItem("Paste", w.editPaste),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Find…", w.openFindActive),
	)
	collMenu := fyne.NewMenu("Collection",
		fyne.NewMenuItem("Collection Auth…", w.showCollectionAuth),
		fyne.NewMenuItem("Environments…", w.showEnvironmentManager),
	)
	// macOS: Fyne moves items labelled exactly "About" and "Settings…" into the
	// system application menu (the one named after the app) — and "About" replaces
	// the default About entry — so they appear where macOS users expect, with no
	// duplicate. "Check for Updates…" is not a recognised special label, so Fyne
	// leaves it here in the Help menu. On Windows/Linux there is no application
	// menu, so all three items simply show under Help.
	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("About", func() { w.app.showAboutDialog(w.win) }),
		fyne.NewMenuItem("Settings…", func() { w.app.showSettingsDialog(w.win) }),
		fyne.NewMenuItem("Check for Updates…", func() { w.checkForUpdates(true) }),
	)
	return fyne.NewMainMenu(fileMenu, editMenu, collMenu, helpMenu)
}

// openFindActive opens find on the active request's response (Edit ▸ Find…, Cmd/Ctrl+F).
func (w *Window) openFindActive() {
	if rt := w.activeTab(); rt != nil {
		rt.response.openFind()
	}
}

// editCopy / editPaste forward the clipboard action to the focused widget so the
// Edit menu items act on whatever entry (URL, params, body, find…) has focus.
func (w *Window) editCopy() {
	w.dispatchShortcut(&fyne.ShortcutCopy{Clipboard: fyne.CurrentApp().Clipboard()})
}

func (w *Window) editPaste() {
	w.dispatchShortcut(&fyne.ShortcutPaste{Clipboard: fyne.CurrentApp().Clipboard()})
}

func (w *Window) dispatchShortcut(s fyne.Shortcut) {
	if f, ok := w.win.Canvas().Focused().(fyne.Shortcutable); ok {
		f.TypedShortcut(s)
	}
}

// open shows a native (OS) file picker filtered to *.yon and loads the chosen
// Collection into a new window. Falls back to Fyne's in-app dialog when the
// native backend is unavailable (e.g. Linux without zenity).
func (w *Window) open() {
	go func() {
		path, ok, err := nativeOpenYon("Open Collection")
		fyne.Do(func() {
			switch {
			case err != nil:
				w.openFyne() // native unavailable → in-app dialog
			case !ok:
				// cancelled — nothing to do
			default:
				if e := w.app.OpenPath(path); e != nil {
					dialog.ShowError(e, w.win)
					return
				}
				w.buildMenu() // refresh Open Recent
			}
		})
	}()
}

// openFyne is the Fyne in-app fallback for open().
func (w *Window) openFyne() {
	d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil || rc == nil {
			return
		}
		path := rc.URI().Path()
		_ = rc.Close()
		if err := w.app.OpenPath(path); err != nil {
			dialog.ShowError(err, w.win)
			return
		}
		w.buildMenu()
	}, w.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".yon"}))
	d.Show()
}

// recentMenuItem builds the "Open Recent" submenu from the app's recent-files
// list. Each entry opens that Collection; a missing file is reported and pruned.
func (w *Window) recentMenuItem() *fyne.MenuItem {
	item := fyne.NewMenuItem("Open Recent", nil)
	recents := w.app.recentFiles()
	if len(recents) == 0 {
		none := fyne.NewMenuItem("(none)", nil)
		none.Disabled = true
		item.ChildMenu = fyne.NewMenu("", none)
		return item
	}
	items := make([]*fyne.MenuItem, 0, len(recents)+2)
	for _, p := range recents {
		p := p
		mi := fyne.NewMenuItem(filepath.Base(p), func() { w.openRecent(p) })
		items = append(items, mi)
	}
	items = append(items,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Clear Menu", func() {
			w.app.clearRecent()
			w.buildMenu()
		}),
	)
	item.ChildMenu = fyne.NewMenu("", items...)
	return item
}

// openRecent opens a remembered path; if it can no longer be loaded (moved or
// deleted) it reports the error and removes it from the recent list.
func (w *Window) openRecent(path string) {
	if err := w.app.OpenPath(path); err != nil {
		dialog.ShowError(err, w.win)
		w.app.removeRecent(path)
	}
	w.buildMenu()
}

// save writes the Collection to its existing path, or falls back to Save As when
// untitled. done (may be nil) is called with whether the save succeeded.
func (w *Window) save(done func(bool)) {
	if w.path == "" {
		w.saveAs(done)
		return
	}
	if err := store.Save(w.path, w.coll); err != nil {
		dialog.ShowError(err, w.win)
		if done != nil {
			done(false)
		}
		return
	}
	w.dirty = false
	w.clearTabsDirty()
	w.updateTitle()
	if done != nil {
		done(true)
	}
}

// saveAs prompts for a path (native dialog, Fyne fallback) and saves there,
// adopting that path as the Collection's file.
func (w *Window) saveAs(done func(bool)) {
	go func() {
		path, ok, err := nativeSaveYon("Save Collection As", "collection.yon")
		fyne.Do(func() {
			switch {
			case err != nil:
				w.saveAsFyne(done) // native unavailable → in-app dialog
			case !ok:
				if done != nil {
					done(false)
				}
			default:
				w.saveToPath(store.EnsureExt(path), done)
			}
		})
	}()
}

// saveAsFyne is the Fyne in-app fallback for saveAs().
func (w *Window) saveAsFyne(done func(bool)) {
	d := dialog.NewFileSave(func(wc fyne.URIWriteCloser, err error) {
		if err != nil || wc == nil {
			if done != nil {
				done(false)
			}
			return
		}
		path := store.EnsureExt(wc.URI().Path())
		_ = wc.Close() // store.Save reopens by path; avoid double-writer.
		w.saveToPath(path, done)
	}, w.win)
	d.SetFileName("collection.yon")
	d.SetFilter(storage.NewExtensionFileFilter([]string{".yon"}))
	d.Show()
}

// saveToPath writes the Collection to path and adopts it: clears dirty, updates
// the title, remembers the file as recent, and rebuilds the menu.
func (w *Window) saveToPath(path string, done func(bool)) {
	if err := store.Save(path, w.coll); err != nil {
		dialog.ShowError(err, w.win)
		if done != nil {
			done(false)
		}
		return
	}
	w.path = path
	w.dirty = false
	w.clearTabsDirty()
	w.updateTitle()
	w.app.rememberRecent(path)
	w.buildMenu()
	if done != nil {
		done(true)
	}
}

// ---- Sidebar verb-chip row ----

// methodAbbrev returns the compact UPPERCASE verb tag shown in the sidebar and
// the request-bar pill (DELETE → "DEL" to stay narrow; others as-is).
func methodAbbrev(m model.Method) string {
	switch m {
	case model.MethodDelete:
		return "DEL"
	default:
		s := string(m)
		if s == "" {
			return "GET"
		}
		return s
	}
}

// verbRowAccent is the cyan left-accent bar colour for the selected row.
var verbRowAccent = color.NRGBA{R: 0x18, G: 0xC5, B: 0xE8, A: 0xff}

// verbRow is one sidebar entry: a thin left accent bar (cyan when selected), a
// fixed-width coloured monospace verb tag, and the Request's display name. It is
// a custom widget so widget.List can use it directly as a list item.
type verbRow struct {
	widget.BaseWidget
	accent *canvas.Rectangle
	tag    *canvas.Text
	name   *widget.Label
	object fyne.CanvasObject
}

// newVerbRow builds an empty row; set() fills it per Request in UpdateItem.
func newVerbRow() *verbRow {
	r := &verbRow{}

	r.accent = canvas.NewRectangle(color.Transparent)
	r.accent.SetMinSize(fyne.NewSize(3, 1))

	r.tag = canvas.NewText("", methodColorSlate)
	r.tag.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	r.tag.TextSize = theme.CaptionTextSize()
	// Fixed-ish width so names line up regardless of verb length (GET/POST/DEL…).
	tagBox := container.NewGridWrap(fyne.NewSize(40, theme.IconInlineSize()), r.tag)

	r.name = widget.NewLabel("")
	r.name.Truncation = fyne.TextTruncateEllipsis

	// [accent][verb tag][name........] — the accent sits in the Border's Left
	// slot so it stretches to the full row height (an HBox would leave a sliver).
	content := container.NewBorder(nil, nil, tagBox, nil, r.name)
	r.object = container.NewBorder(nil, nil, r.accent, nil, content)
	r.ExtendBaseWidget(r)
	return r
}

// CreateRenderer renders the row's pre-built object.
func (r *verbRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.object)
}

// set re-binds the row to req, recolouring the verb tag via methodColor and
// showing the cyan accent bar when this row is the selected one.
func (r *verbRow) set(req model.Request, selected bool) {
	r.tag.Text = methodAbbrev(req.Method)
	r.tag.Color = methodColor(req.Method)
	r.tag.Refresh()

	r.name.SetText(req.DisplayName())

	if selected {
		r.accent.FillColor = verbRowAccent
	} else {
		r.accent.FillColor = color.Transparent
	}
	r.accent.Refresh()
}

// clearTabsDirty drops the unsaved marker from every open tab (called on save).
func (w *Window) clearTabsDirty() {
	for idx, rt := range w.openTabs {
		rt.dirty = false
		if idx >= 0 && idx < len(w.coll.Requests) {
			req := w.coll.Requests[idx]
			rt.tab.setRequest(req.Method, req.DisplayName(), false)
		}
	}
}

// ---- Bottom status bar ----

// buildStatusBar is the thin bar docked at the bottom of the window: the active
// tab's last response status / time / size on the left, its method · path on the
// right, plus the app version (mockup-v9 `.statusbar`).
func (w *Window) buildStatusBar() fyne.CanvasObject {
	muted := theme.Color(theme.ColorNamePlaceHolder)
	mk := func(s string) *canvas.Text {
		t := canvas.NewText(s, muted)
		t.TextSize = theme.CaptionTextSize()
		return t
	}
	w.sbStatus = mk("Ready")
	w.sbMeta = mk("")
	w.sbReqInfo = mk("")

	// Left: status · time · size. Right: the active request's method · path.
	left := container.NewHBox(w.sbStatus, w.sbMeta)
	bar := container.NewBorder(nil, nil, left, w.sbReqInfo)

	bg := canvas.NewRectangle(theme.Color(theme.ColorNameMenuBackground))
	return container.NewStack(bg, container.NewPadded(bar))
}

// activeTab returns the requestTab whose card is currently selected (nil if
// none is open).
func (w *Window) activeTab() *requestTab {
	sel := w.tabs.Selected()
	if sel == nil {
		return nil
	}
	for _, rt := range w.openTabs {
		if rt.tab == sel {
			return rt
		}
	}
	return nil
}

// updateStatusBar refreshes the bottom bar from the active tab's request +
// last response. Safe to call before the bar is built (no-op).
func (w *Window) updateStatusBar() {
	if w.sbStatus == nil {
		return
	}
	muted := theme.Color(theme.ColorNamePlaceHolder)
	rt := w.activeTab()
	if rt == nil || rt.idx < 0 || rt.idx >= len(w.coll.Requests) {
		w.sbStatus.Text, w.sbStatus.Color = "Ready", muted
		w.sbMeta.Text = ""
		w.sbReqInfo.Text = ""
	} else {
		req := w.coll.Requests[rt.idx]
		// Resolve {{variables}} so the status bar shows the real path (the
		// environment/collection value) rather than the literal {{key}} template.
		resolvedURL := w.varScope().Resolve(req.URL)
		w.sbReqInfo.Text = fmt.Sprintf("%s · %s", req.Method, urlPathOf(resolvedURL))
		if r := rt.lastResp; r != nil {
			w.sbStatus.Text = fmt.Sprintf("● %d %s", r.Status, r.StatusText)
			w.sbStatus.Color = statusColor(r.Status)
			w.sbMeta.Text = fmt.Sprintf("   %s · %s", formatDuration(r.Duration), formatSize(r.Size))
		} else {
			w.sbStatus.Text, w.sbStatus.Color = "Ready", muted
			w.sbMeta.Text = ""
		}
	}
	w.sbStatus.Refresh()
	w.sbMeta.Refresh()
	w.sbReqInfo.Refresh()
}

// urlPathOf returns the path component of a request URL for the status bar,
// falling back to the raw string when it can't be parsed.
func urlPathOf(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		return u.Path
	}
	return raw
}
