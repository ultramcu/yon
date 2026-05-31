package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// authEditor is a reusable Auth-editing widget: a Select for the kind plus
// conditional credential fields (username/password for basic, token for
// bearer). It calls onChange whenever any field changes so the owner can commit.
//
// includeInherit controls whether "inherit" is offered: Request Auth may
// inherit; Collection Auth may not (none/basic/bearer only).
type authEditor struct {
	container *fyne.Container

	kindSelect *widget.Select
	username   *widget.Entry
	password   *widget.Entry
	token      *widget.Entry

	// fieldsBox holds the conditional credential rows; it is rebuilt per kind.
	fieldsBox *fyne.Container

	onChange func()
}

// authKindOptions returns the Select options for an Auth editor.
func authKindOptions(includeInherit bool) []string {
	if includeInherit {
		return []string{"inherit", "none", "basic", "bearer"}
	}
	return []string{"none", "basic", "bearer"}
}

// newAuthEditor builds an Auth editor seeded from a. onChange is invoked on any
// edit (it may be nil).
func newAuthEditor(a model.Auth, includeInherit bool, onChange func()) *authEditor {
	ae := &authEditor{onChange: onChange}

	ae.username = widget.NewEntry()
	ae.username.SetText(a.Username)
	ae.username.OnChanged = func(string) { ae.fire() }

	ae.password = widget.NewPasswordEntry()
	ae.password.SetText(a.Password)
	ae.password.OnChanged = func(string) { ae.fire() }

	ae.token = widget.NewEntry()
	ae.token.SetText(a.Token)
	ae.token.OnChanged = func(string) { ae.fire() }

	ae.fieldsBox = container.NewVBox()

	// Build the Select with no handler, set its initial kind, then wire OnChanged —
	// otherwise SetSelected fires onChange (the owner's commit) before this
	// authEditor is assigned into its owner, dereferencing nil on construction.
	ae.kindSelect = widget.NewSelect(authKindOptions(includeInherit), nil)
	kind := string(a.Kind)
	if kind == "" {
		if includeInherit {
			kind = "inherit"
		} else {
			kind = "none"
		}
	}
	ae.kindSelect.SetSelected(kind)

	ae.container = container.NewVBox(
		widget.NewForm(widget.NewFormItem("Auth", ae.kindSelect)),
		ae.fieldsBox,
	)
	ae.rebuildFields()
	ae.kindSelect.OnChanged = func(string) {
		ae.rebuildFields()
		ae.fire()
	}
	return ae
}

// rebuildFields shows only the credential fields relevant to the current kind.
func (ae *authEditor) rebuildFields() {
	ae.fieldsBox.Objects = nil
	switch model.AuthKind(ae.kindSelect.Selected) {
	case model.AuthBasic:
		ae.fieldsBox.Add(widget.NewForm(
			widget.NewFormItem("Username", ae.username),
			widget.NewFormItem("Password", ae.password),
		))
	case model.AuthBearer:
		ae.fieldsBox.Add(widget.NewForm(
			widget.NewFormItem("Token", ae.token),
		))
	}
	ae.fieldsBox.Refresh()
}

// fire notifies the owner of a change.
func (ae *authEditor) fire() {
	if ae.onChange != nil {
		ae.onChange()
	}
}

// value reads the current Auth from the editor. Credential fields not relevant
// to the kind are still carried (kept, not cleared) so toggling kind back and
// forth doesn't lose typed values.
func (ae *authEditor) value() model.Auth {
	return model.Auth{
		Kind:     model.AuthKind(ae.kindSelect.Selected),
		Username: ae.username.Text,
		Password: ae.password.Text,
		Token:    ae.token.Text,
	}
}

// showCollectionAuth opens a dialog to edit the Collection-level default Auth
// (kind ∈ none/basic/bearer). Saving marks the window dirty.
func (w *Window) showCollectionAuth() {
	ae := newAuthEditor(w.coll.Auth, false, nil)
	dialog.ShowCustomConfirm("Collection Auth", "Save", "Cancel", ae.container,
		func(ok bool) {
			if !ok {
				return
			}
			w.coll.Auth = ae.value()
			w.markDirty()
		}, w.win)
}
