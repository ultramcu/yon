package ui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
	"github.com/ultramcu/yon/internal/variables"
)

// noEnvironmentLabel is the selector entry meaning "resolve with collection
// variables only" (no active environment). It is not a real environment name.
const noEnvironmentLabel = "No Environment"

// manageEnvironmentsLabel is the selector entry that opens the Manage
// Environments dialog instead of selecting an environment.
const manageEnvironmentsLabel = "Manage Environments…"

// loadEnvironments populates w.envs from the sibling environment files of the
// backing collection. For an unsaved collection (path == "") there are no
// sibling files, so the list is left empty. Errors are reported to the user but
// are non-fatal: the window still opens, just without environments.
func (w *Window) loadEnvironments() {
	if w.path == "" {
		w.envs = nil
		return
	}
	envs, err := store.LoadEnvironments(w.path)
	if err != nil {
		dialog.ShowError(err, w.win)
		w.envs = nil
		return
	}
	w.envs = envs
}

// activeEnv returns the loaded environment whose Name matches the collection's
// ActiveEnvironment, plus whether one was found. An empty ActiveEnvironment (or
// a name no longer present in the loaded list) yields ok == false.
func (w *Window) activeEnv() (model.Environment, bool) {
	if w.coll.ActiveEnvironment == "" {
		return model.Environment{}, false
	}
	for _, e := range w.envs {
		if e.Name == w.coll.ActiveEnvironment {
			return e, true
		}
	}
	return model.Environment{}, false
}

// varScope builds the variables.Scope used to resolve {{templates}} on a send.
// It combines the active environment (env-scoped, highest precedence) with the
// collection-scoped variables (lower precedence). Secret values are already
// merged into the loaded environments by store.LoadEnvironments, so each
// Variable carries its effective Value directly; the Scope.Secrets map is left
// empty (a secret env/collection variable resolves to its own Value, which the
// store has populated). When no environment is active, only collection
// variables apply.
func (w *Window) varScope() variables.Scope {
	env, _ := w.activeEnv() // zero Environment when none active → env vars empty
	return variables.Scope{
		Env:        env,
		Collection: w.coll.Variables,
		Secrets:    nil,
	}
}

// environmentNames returns the list of loaded environment names, in their
// loaded order, used to populate the selector and the manager list.
func (w *Window) environmentNames() []string {
	names := make([]string, 0, len(w.envs))
	for _, e := range w.envs {
		names = append(names, e.Name)
	}
	return names
}

// ---- Environment selector ----

// buildEnvSelector builds the compact environment picker shown in the sidebar
// header: "No Environment" + each environment name + a "Manage Environments…"
// action. Selecting an environment sets it active (marking the collection
// dirty); selecting the Manage entry opens the manager and re-selects the
// previously active environment so the picker keeps showing it.
func (w *Window) buildEnvSelector() fyne.CanvasObject {
	options := append([]string{noEnvironmentLabel}, w.environmentNames()...)
	options = append(options, manageEnvironmentsLabel)

	sel := widget.NewSelect(options, nil)
	w.envSelect = sel

	// Seed the selection from the collection's active environment.
	if _, ok := w.activeEnv(); ok {
		sel.Selected = w.coll.ActiveEnvironment
	} else {
		sel.Selected = noEnvironmentLabel
	}

	sel.OnChanged = func(choice string) {
		switch choice {
		case manageEnvironmentsLabel:
			// Re-show the active selection, then open the manager.
			w.syncEnvSelector()
			w.showEnvironmentManager()
		case noEnvironmentLabel:
			if w.coll.ActiveEnvironment != "" {
				w.coll.ActiveEnvironment = ""
				w.markDirty()
			}
			// Clearing the active environment drops any jump-host binding.
			w.app.rebindTunnel(w)
			w.updateTunnelIndicator()
		default:
			if w.coll.ActiveEnvironment != choice {
				w.coll.ActiveEnvironment = choice
				w.markDirty()
			}
			// Follow the new active environment's jump host (if any).
			w.app.rebindTunnel(w)
			w.updateTunnelIndicator()
		}
	}
	return sel
}

// syncEnvSelector rebuilds the selector's options from the current environment
// list and restores its selection to the active environment (or "No
// Environment"). Called after the environment list or active selection changes.
func (w *Window) syncEnvSelector() {
	if w.envSelect == nil {
		return
	}
	options := append([]string{noEnvironmentLabel}, w.environmentNames()...)
	options = append(options, manageEnvironmentsLabel)
	w.envSelect.Options = options

	if _, ok := w.activeEnv(); ok {
		w.envSelect.SetSelected(w.coll.ActiveEnvironment)
	} else {
		w.envSelect.SetSelected(noEnvironmentLabel)
	}
	w.envSelect.Refresh()
}

// ---- Variable table (Key / Value / Enabled / Secret) ----

// varTable is an editable variable table: each row has an Enabled check, Key
// and Value entries, an optional Secret toggle, and a delete button, plus an
// "Add" button. It mirrors kvTable but carries the per-variable Secret flag that
// environment variables need.
//
// secrets controls whether the Secret column is shown. Collection variables do
// NOT support secrets (secrets belong to environments and live only in the
// gitignored .env), so the manager builds the collection-variables table with
// secrets == false: the Secret checkbox is hidden and value() never marks a
// collection variable Secret, so a secret value can never leak into the .yon.
type varTable struct {
	container *fyne.Container
	rowsBox   *fyne.Container
	rows      []*varRow
	secrets   bool
}

// varRow is one editable model.Variable row.
type varRow struct {
	enabled *widget.Check
	key     *widget.Entry
	value   *widget.Entry
	secret  *widget.Check
	object  fyne.CanvasObject
}

// newVarTable builds a table seeded from vars. When secrets is true a per-row
// Secret toggle (and its header) are shown; when false the Secret column is
// omitted entirely, which is how collection variables are edited (they cannot be
// secret).
func newVarTable(vars []model.Variable, secrets bool) *varTable {
	t := &varTable{secrets: secrets}
	t.rowsBox = container.NewVBox()

	for _, v := range vars {
		t.appendRow(v)
	}

	add := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		t.appendRow(model.Variable{Enabled: true})
	})
	add.Importance = widget.LowImportance

	var secretHeader fyne.CanvasObject
	if secrets {
		secretHeader = widget.NewLabelWithStyle("Secret", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	}
	header := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("On", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		secretHeader,
		container.NewGridWithColumns(2,
			widget.NewLabelWithStyle("Key", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Value", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
	)

	t.container = container.NewBorder(header, add, nil, nil,
		container.NewVScroll(t.rowsBox))
	return t
}

// appendRow adds a row widget for v.
func (t *varTable) appendRow(v model.Variable) {
	r := &varRow{}

	r.enabled = widget.NewCheck("", nil)
	r.enabled.SetChecked(v.Enabled)

	r.key = widget.NewEntry()
	r.key.SetPlaceHolder("Key")
	r.key.SetText(v.Key)

	r.value = widget.NewEntry()
	r.value.SetPlaceHolder("Value")
	r.value.SetText(v.Value)

	del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		t.removeRow(r)
	})
	del.Importance = widget.LowImportance

	// The Secret toggle is only present for environment variables; collection
	// variables (secrets == false) get no Secret checkbox so they can never be
	// marked secret and leak a value into the committed .yon.
	var trailing *fyne.Container
	if t.secrets {
		r.secret = widget.NewCheck("", nil)
		r.secret.SetChecked(v.Secret)
		trailing = container.NewHBox(r.secret, del)
	} else {
		trailing = container.NewHBox(del)
	}

	// [On][Key | Value][Secret?][del]
	r.object = container.NewBorder(nil, nil, r.enabled,
		trailing,
		container.NewGridWithColumns(2, r.key, r.value),
	)

	t.rows = append(t.rows, r)
	t.rowsBox.Add(r.object)
	t.rowsBox.Refresh()
}

// removeRow drops a row from the table.
func (t *varTable) removeRow(target *varRow) {
	for i, r := range t.rows {
		if r == target {
			t.rows = append(t.rows[:i], t.rows[i+1:]...)
			break
		}
	}
	t.rowsBox.Remove(target.object)
	t.rowsBox.Refresh()
}

// value reads the table back into a []model.Variable, preserving order. Fully
// empty rows (no key, no value) are skipped; a disabled row with content is
// kept ("keep but don't apply").
func (t *varTable) value() []model.Variable {
	var out []model.Variable
	for _, r := range t.rows {
		if r.key.Text == "" && r.value.Text == "" {
			continue
		}
		// r.secret is nil for a collection-variables table (no Secret column),
		// so such variables are always non-secret.
		secret := r.secret != nil && r.secret.Checked
		out = append(out, model.Variable{
			Key:     r.key.Text,
			Value:   r.value.Text,
			Enabled: r.enabled.Checked,
			Secret:  secret,
		})
	}
	return out
}

// ---- SSH jump-host editor ----

// jumpAuthKeyLabel and jumpAuthPasswordLabel are the auth-selector entries in
// the jump-host form; they map to model.JumpAuthKey / model.JumpAuthPassword.
const (
	jumpAuthKeyLabel      = "Private key"
	jumpAuthPasswordLabel = "Password"
)

// jumpHostFromForm builds the *model.JumpHost an environment should carry from
// the jump-host form's raw fields. It returns nil when the jump host is turned
// off (!use) or no Host is given, so an environment without a usable jump host
// serializes exactly as before (nil JumpHost). Port is parsed from text; a
// blank or non-numeric value yields 0, which the tunnel manager treats as the
// default (22). auth selects password vs key auth: it accepts either the UI
// selector label (jumpAuthPasswordLabel) or the model constant
// (model.JumpAuthPassword); anything else maps to key auth. This is a pure
// helper (no Fyne) so it can be unit-tested directly.
func jumpHostFromForm(use bool, host, port, user, auth, keyPath, passphrase, password string, insecure bool) *model.JumpHost {
	if !use || host == "" {
		return nil
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 0 {
		p = 0 // blank/invalid → 0, i.e. the manager's default port
	}
	authKind := model.JumpAuthKey
	if auth == jumpAuthPasswordLabel || auth == model.JumpAuthPassword {
		authKind = model.JumpAuthPassword
	}
	jh := &model.JumpHost{
		Host:     host,
		Port:     p,
		User:     user,
		Auth:     authKind,
		Insecure: insecure,
	}
	// Carry only the fields the chosen auth uses; the unused secret/path stays
	// blank so a password jump host never persists a stale key path and vice versa.
	if authKind == model.JumpAuthPassword {
		jh.Password = password
	} else {
		jh.KeyPath = keyPath
		jh.Passphrase = passphrase
	}
	return jh
}

// jumpHostForm is the editable SSH jump-host section shown below the variable
// table for a real environment. It edits a single environment's optional
// *JumpHost; value() reads it back via jumpHostFromForm.
type jumpHostForm struct {
	container *fyne.Container

	use        *widget.Check
	host       *widget.Entry
	port       *widget.Entry
	user       *widget.Entry
	auth       *widget.Select
	keyPath    *widget.Entry
	passphrase *widget.Entry
	password   *widget.Entry
	insecure   *widget.Check

	// fields is the reveal target toggled by use; keyRow/passwordRow swap with the
	// auth selector.
	fields      *fyne.Container
	keyRow      fyne.CanvasObject
	passwordRow fyne.CanvasObject
}

// newJumpHostForm builds the jump-host form seeded from jh (nil means "no jump
// host": the Use checkbox is off and the fields are hidden).
func newJumpHostForm(jh *model.JumpHost) *jumpHostForm {
	f := &jumpHostForm{}

	f.host = widget.NewEntry()
	f.host.SetPlaceHolder("bastion.example.com")
	f.port = widget.NewEntry()
	f.port.SetPlaceHolder("22")
	f.user = widget.NewEntry()
	f.user.SetPlaceHolder("user")
	f.auth = widget.NewSelect([]string{jumpAuthKeyLabel, jumpAuthPasswordLabel}, nil)
	f.keyPath = widget.NewEntry()
	f.keyPath.SetPlaceHolder("~/.ssh/id_ed25519")
	f.passphrase = widget.NewPasswordEntry()
	f.passphrase.SetPlaceHolder("Key passphrase (optional)")
	f.password = widget.NewPasswordEntry()
	f.password.SetPlaceHolder("Password")
	f.insecure = widget.NewCheck("Skip host key check (insecure)", nil)

	// Seed from the existing config.
	f.auth.SetSelected(jumpAuthKeyLabel)
	if jh != nil {
		f.host.SetText(jh.Host)
		if jh.Port != 0 {
			f.port.SetText(strconv.Itoa(jh.Port))
		}
		f.user.SetText(jh.User)
		if jh.Auth == model.JumpAuthPassword {
			f.auth.SetSelected(jumpAuthPasswordLabel)
		}
		f.keyPath.SetText(jh.KeyPath)
		f.passphrase.SetText(jh.Passphrase)
		f.password.SetText(jh.Password)
		f.insecure.SetChecked(jh.Insecure)
	}

	f.keyRow = container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Key path", f.keyPath),
			widget.NewFormItem("Passphrase", f.passphrase),
		),
	)
	f.passwordRow = widget.NewForm(widget.NewFormItem("Password", f.password))

	authFields := container.NewStack(f.keyRow, f.passwordRow)
	f.auth.OnChanged = func(string) { f.refreshAuth(authFields) }

	f.fields = container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Host", f.host),
			widget.NewFormItem("Port", f.port),
			widget.NewFormItem("User", f.user),
			widget.NewFormItem("Auth", f.auth),
		),
		authFields,
		f.insecure,
	)

	f.use = widget.NewCheck("Use SSH jump host", func(bool) { f.refreshVisibility() })
	f.use.SetChecked(jh != nil)

	f.container = container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("SSH jump host", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		f.use,
		f.fields,
	)

	f.refreshVisibility()
	f.refreshAuth(authFields)
	return f
}

// refreshVisibility shows the jump-host fields only when "Use SSH jump host" is
// checked.
func (f *jumpHostForm) refreshVisibility() {
	if f.use.Checked {
		f.fields.Show()
	} else {
		f.fields.Hide()
	}
}

// refreshAuth reveals the key rows or the password row to match the auth
// selector.
func (f *jumpHostForm) refreshAuth(authFields *fyne.Container) {
	if f.auth.Selected == jumpAuthPasswordLabel {
		f.keyRow.Hide()
		f.passwordRow.Show()
	} else {
		f.keyRow.Show()
		f.passwordRow.Hide()
	}
	authFields.Refresh()
}

// value reads the form back into a *model.JumpHost (nil when unchecked or Host
// is empty) via the pure jumpHostFromForm.
func (f *jumpHostForm) value() *model.JumpHost {
	return jumpHostFromForm(
		f.use.Checked,
		f.host.Text, f.port.Text, f.user.Text,
		f.auth.Selected,
		f.keyPath.Text, f.passphrase.Text, f.password.Text,
		f.insecure.Checked,
	)
}

// ---- Manage Environments dialog ----

// showEnvironmentManager opens the environment manager: a left list of
// environments (plus a "Collection Variables" pseudo-entry) and, on the right,
// an editable variable table for the selected entry with Add / Rename / Delete
// actions. Real environments persist via store.SaveEnvironment /
// store.DeleteEnvironment, which require a saved collection; collection
// variables persist with the normal collection Save. If the collection is
// unsaved, the user is prompted to save it first.
func (w *Window) showEnvironmentManager() {
	if w.path == "" {
		w.promptSaveBeforeEnvironments()
		return
	}

	// Reset any deletes/renames queued by a previous manager session that was
	// closed without saving. pendingEnvDeletes must only ever contain entries
	// queued in THIS dialog session, and is cleared again on Save (in
	// persistEnvironments) or on the Close/cancel path below; otherwise a stale
	// queued delete could fire on a later unrelated collection Save.
	w.pendingEnvDeletes = nil

	// collectionEntry is the pseudo-row that edits w.coll.Variables in the same
	// list as the real environments.
	const collectionEntry = "Collection Variables"

	// Work on a copy of the environments so edits only land on Save.
	working := make([]model.Environment, len(w.envs))
	copy(working, w.envs)
	collVars := append([]model.Variable(nil), w.coll.Variables...)

	// entries is the list backing model: collectionEntry first, then env names.
	entries := func() []string {
		out := []string{collectionEntry}
		for _, e := range working {
			out = append(out, e.Name)
		}
		return out
	}

	var selected int               // index into entries()
	var table *varTable            // editor for the currently selected entry
	var jhForm *jumpHostForm       // SSH jump-host editor (real environments only)
	detail := container.NewStack() // holds the table (+ jump-host form)
	var list *widget.List
	var rebuildDetail func()
	var commitTable func() // flush the visible table back into working/collVars

	// commitTable captures the current table edits into the working model for the
	// entry that was being edited, so switching rows / saving doesn't lose them.
	editing := 0
	commitTable = func() {
		if table == nil {
			return
		}
		if editing == 0 {
			collVars = table.value()
			return
		}
		envIdx := editing - 1
		if envIdx >= 0 && envIdx < len(working) {
			working[envIdx].Variables = table.value()
			// Real environments also carry the optional SSH jump host.
			if jhForm != nil {
				working[envIdx].JumpHost = jhForm.value()
			}
		}
	}

	rebuildDetail = func() {
		commitTable()
		editing = selected
		var vars []model.Variable
		var jh *model.JumpHost
		// The collection pseudo-entry (selected == 0) does not support secrets or
		// a jump host; real environments do.
		secrets := selected != 0
		if selected == 0 {
			vars = collVars
		} else if envIdx := selected - 1; envIdx >= 0 && envIdx < len(working) {
			vars = working[envIdx].Variables
			jh = working[envIdx].JumpHost
		}
		table = newVarTable(vars, secrets)
		if secrets {
			// Real environment: show the variable table with the SSH jump-host
			// editor below it.
			jhForm = newJumpHostForm(jh)
			content := container.NewBorder(nil, jhForm.container, nil, nil, table.container)
			detail.Objects = []fyne.CanvasObject{content}
		} else {
			jhForm = nil
			detail.Objects = []fyne.CanvasObject{table.container}
		}
		detail.Refresh()
	}

	list = widget.NewList(
		func() int { return len(entries()) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			es := entries()
			if id < 0 || id >= len(es) {
				return
			}
			o.(*widget.Label).SetText(es[id])
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		selected = id
		rebuildDetail()
	}

	// Add: prompt for a new environment name, append it (unsaved until Save).
	addBtn := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder("Environment name")
		dialog.ShowForm("New Environment", "Add", "Cancel",
			[]*widget.FormItem{widget.NewFormItem("Name", nameEntry)},
			func(ok bool) {
				name := nameEntry.Text
				if !ok || name == "" {
					return
				}
				for _, e := range working {
					if e.Name == name {
						dialog.ShowInformation("Duplicate", "An environment named "+name+" already exists.", w.win)
						return
					}
				}
				commitTable()
				working = append(working, model.Environment{Name: name})
				selected = len(working) // new entry index in entries()
				list.Refresh()
				list.Select(selected)
			}, w.win)
	})
	addBtn.Importance = widget.LowImportance

	// Rename: only valid for a real environment (not the Collection pseudo-entry).
	renameBtn := widget.NewButton("Rename", func() {
		if selected == 0 {
			dialog.ShowInformation("Rename", "Collection Variables cannot be renamed.", w.win)
			return
		}
		envIdx := selected - 1
		if envIdx < 0 || envIdx >= len(working) {
			return
		}
		old := working[envIdx].Name
		nameEntry := widget.NewEntry()
		nameEntry.SetText(old)
		dialog.ShowForm("Rename Environment", "Rename", "Cancel",
			[]*widget.FormItem{widget.NewFormItem("Name", nameEntry)},
			func(ok bool) {
				name := nameEntry.Text
				if !ok || name == "" || name == old {
					return
				}
				commitTable()
				working[envIdx].Name = name
				// Delete the old sibling file on Save; track the rename by removing
				// the old then writing the new (handled in the Save action below).
				w.pendingEnvDeletes = append(w.pendingEnvDeletes, old)
				if w.coll.ActiveEnvironment == old {
					w.coll.ActiveEnvironment = name
					w.markDirty()
				}
				list.Refresh()
			}, w.win)
	})
	renameBtn.Importance = widget.LowImportance

	// Delete: removes the selected environment (deferred to Save). The Collection
	// pseudo-entry cannot be deleted.
	deleteBtn := widget.NewButton("Delete", func() {
		if selected == 0 {
			dialog.ShowInformation("Delete", "Collection Variables cannot be deleted.", w.win)
			return
		}
		envIdx := selected - 1
		if envIdx < 0 || envIdx >= len(working) {
			return
		}
		name := working[envIdx].Name
		dialog.ShowConfirm("Delete Environment",
			"Delete environment "+name+"? This removes its sibling file on Save.",
			func(yes bool) {
				if !yes {
					return
				}
				w.pendingEnvDeletes = append(w.pendingEnvDeletes, name)
				working = append(working[:envIdx], working[envIdx+1:]...)
				if w.coll.ActiveEnvironment == name {
					w.coll.ActiveEnvironment = ""
					w.markDirty()
				}
				selected = 0
				table = nil
				jhForm = nil
				editing = 0
				list.Refresh()
				list.Select(0)
			}, w.win)
	})
	deleteBtn.Importance = widget.LowImportance

	actions := container.NewHBox(addBtn, renameBtn, deleteBtn)
	left := container.NewBorder(nil, actions, nil, nil, list)
	body := container.NewHSplit(left, container.NewPadded(detail))
	body.SetOffset(0.32)

	// Seed selection on the Collection entry.
	list.Select(0)

	d := dialog.NewCustomConfirm("Environments", "Save", "Close", body,
		func(ok bool) {
			if !ok {
				// Dialog closed/cancelled without saving: discard any
				// deletes/renames queued this session so they never execute on
				// a later unrelated Save.
				w.pendingEnvDeletes = nil
				return
			}
			commitTable()
			w.persistEnvironments(working, collVars)
		}, w.win)
	d.Resize(fyne.NewSize(720, 480))
	d.Show()
}

// persistEnvironments writes the manager's edits to disk: deletes the
// environments queued in pendingEnvDeletes, saves every working environment,
// applies the edited collection variables (marking the collection dirty so a
// normal Save persists them), reloads the in-memory list, and refreshes the
// selector. Errors are reported but processing continues for the rest.
func (w *Window) persistEnvironments(working []model.Environment, collVars []model.Variable) {
	if w.path == "" {
		w.promptSaveBeforeEnvironments()
		return
	}

	for _, name := range w.pendingEnvDeletes {
		if err := store.DeleteEnvironment(w.path, name); err != nil {
			dialog.ShowError(err, w.win)
		}
	}
	w.pendingEnvDeletes = nil

	for _, env := range working {
		if err := store.SaveEnvironment(w.path, env); err != nil {
			dialog.ShowError(err, w.win)
		}
	}

	// Collection variables ride along with the normal collection Save.
	w.coll.Variables = collVars
	w.markDirty()

	w.loadEnvironments()
	w.syncEnvSelector()
}

// promptSaveBeforeEnvironments tells the user that environments need a saved
// collection and offers to Save now; on success it re-opens the manager.
func (w *Window) promptSaveBeforeEnvironments() {
	dialog.ShowConfirm("Save Collection First",
		"Environments are stored in files beside the collection, so the collection must be saved first.\n\nSave it now?",
		func(save bool) {
			if !save {
				return
			}
			w.save(func(ok bool) {
				if ok && w.path != "" {
					w.loadEnvironments()
					w.syncEnvSelector()
					w.showEnvironmentManager()
				}
			})
		}, w.win)
}
