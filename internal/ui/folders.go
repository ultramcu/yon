package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// ---- Grouped sidebar: folder header row + unified list item ----
//
// The sidebar List renders a heterogeneous sequence (see Window.sidebarRows):
// folder-header rows interleaved with request rows. widget.List uses a SINGLE
// item template, so each list item is a sidebarItem that holds BOTH a folderRow
// and a verbRow and shows exactly one of them per visible row ("row morphing").
// UpdateItem reads w.rows[id] and calls setFolder or setRequest accordingly.

// folderRow is one sidebar folder-header entry: a collapse chevron (▶ collapsed /
// ▼ expanded), the folder name, and a request-count badge. It is a custom widget
// so it can live as part of a List item AND be primary/secondary tappable.
//
// Like verbRow, folderRow MUST implement fyne.Tappable: the glfw tap router (see
// sidebar_tap_routing_test) resolves a click to the deepest object implementing
// ANY tappable interface. The folder header is the deepest match over its row, so
// it needs a primary Tapped (to toggle collapse) — otherwise the click would be
// swallowed exactly like the #13 verbRow regression.
type folderRow struct {
	widget.BaseWidget
	chevron *widget.Icon
	name    *widget.Label
	count   *widget.Label
	object  fyne.CanvasObject

	// id is the folder id this row currently shows; set() rebinds it per row in
	// UpdateItem. The callbacks (wired in buildSidebar) are invoked with id.
	id       string
	onToggle func(id string)
	onRename func(id string)
	onDelete func(id string)
}

// newFolderRow builds an empty folder header; set() fills it per Folder.
func newFolderRow() *folderRow {
	r := &folderRow{}
	r.chevron = widget.NewIcon(theme.MenuDropDownIcon())
	r.name = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	r.name.Truncation = fyne.TextTruncateEllipsis
	r.count = widget.NewLabel("")

	// [chevron][name........][count] — chevron left, count right, name fills.
	r.object = container.NewBorder(nil, nil, r.chevron, r.count, r.name)
	r.ExtendBaseWidget(r)
	return r
}

// CreateRenderer renders the row's pre-built object.
func (r *folderRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.object)
}

// set re-binds the row to folder f, pointing the chevron down when expanded and
// right when collapsed, and showing the folder's request count.
func (r *folderRow) set(f model.Folder, reqCount int) {
	r.id = f.ID
	if f.Collapsed {
		r.chevron.SetResource(theme.MenuExpandIcon()) // ▶ collapsed (points right)
	} else {
		r.chevron.SetResource(theme.MenuDropDownIcon()) // ▼ expanded
	}
	r.name.SetText(f.Name)
	r.count.SetText(fmt.Sprintf("%d", reqCount))
}

// Tapped toggles the folder's collapsed state on a primary (left) click of the
// header or chevron. folderRow MUST be fyne.Tappable (see the type doc) so the
// driver dispatches this and the click isn't swallowed.
func (r *folderRow) Tapped(*fyne.PointEvent) {
	if r.onToggle != nil {
		r.onToggle(r.id)
	}
}

// TappedSecondary shows the folder's right-click menu: "Rename Folder…" then
// "Delete Folder", each calling its wired callback with this row's folder id.
func (r *folderRow) TappedSecondary(e *fyne.PointEvent) {
	if r.onRename == nil && r.onDelete == nil {
		return
	}
	id := r.id
	var items []*fyne.MenuItem
	if r.onRename != nil {
		items = append(items, fyne.NewMenuItem("Rename Folder…", func() { r.onRename(id) }))
	}
	if r.onDelete != nil {
		items = append(items, fyne.NewMenuItem("Delete Folder", func() { r.onDelete(id) }))
	}
	menu := fyne.NewMenu("", items...)
	if c := fyne.CurrentApp().Driver().CanvasForObject(r); c != nil {
		widget.ShowPopUpMenuAtPosition(menu, c, e.AbsolutePosition)
	}
}

// sidebarItem is the unified List item template that renders EITHER a folder
// header OR a request row. It stacks a folderRow and a verbRow and hides one of
// them per row ("row morphing"): setFolder shows the header and hides the request
// widget; setRequest does the inverse. Only ever one is visible, so taps route to
// the correct child (the hidden child is not laid out / not hit-tested).
type sidebarItem struct {
	widget.BaseWidget
	folder *folderRow
	req    *verbRow
	object *fyne.Container
}

// newSidebarItem builds the morphing item with both child widgets present; the
// request widget starts shown (UpdateItem morphs it as needed).
func newSidebarItem() *sidebarItem {
	it := &sidebarItem{
		folder: newFolderRow(),
		req:    newVerbRow(),
	}
	it.object = container.NewStack(it.folder, it.req)
	it.folder.Hide()
	it.ExtendBaseWidget(it)
	return it
}

// CreateRenderer renders the stacked children (exactly one is visible at a time).
func (it *sidebarItem) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(it.object)
}

// setFolder morphs this item into a folder header: show the folderRow, hide the
// verbRow, and bind the header to f with its request count.
func (it *sidebarItem) setFolder(f model.Folder, reqCount int) {
	it.req.Hide()
	it.folder.Show()
	it.folder.set(f, reqCount)
}

// setRequest morphs this item into a request row: show the verbRow, hide the
// folderRow, and bind the row to the Request at flat index id (selected paints
// the cyan accent).
func (it *sidebarItem) setRequest(id int, req model.Request, selected bool) {
	it.folder.Hide()
	it.req.Show()
	it.req.set(id, req, selected)
}

// ---- Folder UI actions (wired from buildSidebar / the sidebar header) ----

// toggleFolder flips the folder's collapsed flag and re-renders the grouped rows
// (a collapse hides that folder's request rows; an expand reveals them).
func (w *Window) toggleFolder(id string) {
	w.toggleFolderCollapsed(id)
	w.refreshSidebar()
}

// showNewFolder prompts for a name and creates an empty, expanded folder, then
// refreshes so its header appears. Wired to the sidebar header's "New Folder"
// button. An all-whitespace name is ignored.
func (w *Window) showNewFolder() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Folder name")
	dialog.ShowForm("New Folder", "Create", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Name", entry)},
		func(ok bool) {
			if !ok {
				return
			}
			w.addFolder(entry.Text)
			w.refreshSidebar()
		}, w.win)
}

// showRenameFolder opens a dialog prefilled with the folder's current name and
// renames it on confirm, then refreshes so the header label updates.
func (w *Window) showRenameFolder(id string) {
	f, ok := w.folderByID(id)
	if !ok {
		return
	}
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Folder name")
	entry.SetText(f.Name)
	dialog.ShowForm("Rename Folder", "Rename", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Name", entry)},
		func(ok bool) {
			if !ok {
				return
			}
			w.renameFolder(id, entry.Text)
			w.refreshSidebar()
		}, w.win)
}

// confirmDeleteFolder confirms deleting a folder, reminding the user that its
// requests are NOT deleted but move to top level. On confirm it deletes the
// folder (reparenting its requests) and refreshes.
func (w *Window) confirmDeleteFolder(id string) {
	f, ok := w.folderByID(id)
	if !ok {
		return
	}
	name := f.Name
	dialog.NewConfirm("Delete folder",
		fmt.Sprintf("Delete folder %q?\nIts requests are kept and moved to the top level.", name),
		func(ok bool) {
			if !ok {
				return
			}
			w.deleteFolder(id)
			w.refreshSidebar()
		}, w.win).Show()
}

// moveToFolderMenu builds the "Move to folder…" submenu for the request at
// reqIdx: one item per folder plus "Top level (no folder)". The request's CURRENT
// folder item is disabled (moving there is a no-op). Each item calls
// moveRequestToFolder then refreshes so the row reappears under its new folder.
func (w *Window) moveToFolderMenu(reqIdx int) *fyne.Menu {
	current := ""
	if reqIdx >= 0 && reqIdx < len(w.coll.Requests) {
		current = w.coll.Requests[reqIdx].FolderID
	}

	move := func(folderID string) {
		w.moveRequestToFolder(reqIdx, folderID)
		w.refreshSidebar()
		// Keep the selection/accent pinned to the moved request at its (unchanged)
		// flat index, now under the new folder.
		if reqIdx == w.selectedID {
			w.selectByReqIdx(reqIdx)
		}
	}

	items := make([]*fyne.MenuItem, 0, len(w.coll.Folders)+1)
	top := fyne.NewMenuItem("Top level (no folder)", func() { move("") })
	top.Disabled = current == ""
	items = append(items, top)
	for i := range w.coll.Folders {
		f := w.coll.Folders[i]
		mi := fyne.NewMenuItem(f.Name, func() { move(f.ID) })
		mi.Disabled = current == f.ID
		items = append(items, mi)
	}
	return fyne.NewMenu("", items...)
}
