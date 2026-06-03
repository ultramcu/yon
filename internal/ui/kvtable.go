package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// kvTable is an editable Key/Value/Enabled table backed by a VBox of rows
// (ADR-friendly: avoids widget.Table edit complexity). Each row is an Enabled
// check, a Key entry, a Value entry, and a delete button; an "Add" button
// appends a row. Any edit calls onChange so the owner can commit the Request.
type kvTable struct {
	container *fyne.Container
	rowsBox   *fyne.Container
	rows      []*kvRow

	onChange func()
}

// kvRow is one editable Param row.
type kvRow struct {
	enabled *widget.Check
	key     *widget.Entry
	value   *widget.Entry
	object  fyne.CanvasObject
}

// newKVTable builds a table seeded from params. onChange may be nil.
func newKVTable(params []model.Param, onChange func()) *kvTable {
	t := &kvTable{onChange: onChange}
	t.rowsBox = container.NewVBox()

	for _, p := range params {
		t.appendRow(p)
	}

	add := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		t.appendRow(model.Param{Enabled: true})
		t.fire()
	})
	add.Importance = widget.LowImportance

	header := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("On", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nil,
		container.NewGridWithColumns(2,
			widget.NewLabelWithStyle("Key", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Value", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
	)

	t.container = container.NewBorder(header, add, nil, nil,
		container.NewVScroll(t.rowsBox))
	return t
}

// appendRow adds a row widget for p (without firing onChange).
func (t *kvTable) appendRow(p model.Param) {
	r := &kvRow{}
	// nil handler → SetChecked → wire OnChanged after, so seeding an enabled row
	// during table construction doesn't fire onChange (commit) before the owning
	// table is assigned (nil deref).
	r.enabled = widget.NewCheck("", nil)
	r.enabled.SetChecked(p.Enabled)
	r.enabled.OnChanged = func(bool) { t.fire() }

	r.key = widget.NewEntry()
	r.key.SetPlaceHolder("Key")
	r.key.SetText(p.Key)
	r.key.OnChanged = func(string) { t.fire() }

	r.value = widget.NewEntry()
	r.value.SetPlaceHolder("Value")
	r.value.SetText(p.Value)
	r.value.OnChanged = func(string) { t.fire() }

	del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		t.removeRow(r)
		t.fire()
	})
	del.Importance = widget.LowImportance

	r.object = container.NewBorder(nil, nil, r.enabled, del,
		container.NewGridWithColumns(2, r.key, r.value),
	)

	t.rows = append(t.rows, r)
	t.rowsBox.Add(r.object)
	t.rowsBox.Refresh()
}

// setValue replaces every row with params, without firing onChange. Used by the
// URL⇄Params sync to rebuild the table from the URL's query string; the caller
// fires onChange itself once the rebuild is done.
func (t *kvTable) setValue(params []model.Param) {
	for _, r := range t.rows {
		t.rowsBox.Remove(r.object)
	}
	t.rows = nil
	for _, p := range params {
		t.appendRow(p)
	}
	t.rowsBox.Refresh()
}

// removeRow drops a row from the table.
func (t *kvTable) removeRow(target *kvRow) {
	for i, r := range t.rows {
		if r == target {
			t.rows = append(t.rows[:i], t.rows[i+1:]...)
			break
		}
	}
	t.rowsBox.Remove(target.object)
	t.rowsBox.Refresh()
}

// fire notifies the owner of a change.
func (t *kvTable) fire() {
	if t.onChange != nil {
		t.onChange()
	}
}

// value reads the table back into a []model.Param, preserving order. Fully
// empty rows (no key, no value) are skipped so an accidental blank row isn't
// persisted; a disabled row with content is kept (Enabled false), since
// disabling is "keep but don't send".
func (t *kvTable) value() []model.Param {
	var out []model.Param
	for _, r := range t.rows {
		if r.key.Text == "" && r.value.Text == "" {
			continue
		}
		out = append(out, model.Param{
			Key:     r.key.Text,
			Value:   r.value.Text,
			Enabled: r.enabled.Checked,
		})
	}
	return out
}
