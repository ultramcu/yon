package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// --- Capture source labels (UI label ↔ model.CaptureSource) ---

const (
	captureSourceJSONBodyLabel = "JSON body"
	captureSourceHeaderLabel   = "Header"
)

// captureSourceLabels lists the Capture Source selector values in display order.
var captureSourceLabels = []string{captureSourceJSONBodyLabel, captureSourceHeaderLabel}

// captureSourceFromLabel maps a Source selector label to its model.CaptureSource
// (defaulting to CaptureJSONBody for an empty/unknown label). Pure + Fyne-free so
// it can be unit-tested directly.
func captureSourceFromLabel(label string) model.CaptureSource {
	switch label {
	case captureSourceHeaderLabel:
		return model.CaptureHeader
	default:
		return model.CaptureJSONBody
	}
}

// captureSourceToLabel maps a model.CaptureSource back to its selector label.
func captureSourceToLabel(s model.CaptureSource) string {
	switch s {
	case model.CaptureHeader:
		return captureSourceHeaderLabel
	default:
		return captureSourceJSONBodyLabel
	}
}

// --- Assertion source labels (UI label ↔ model.AssertSource) ---

const (
	assertSourceStatusLabel   = "Status"
	assertSourceJSONBodyLabel = "JSON body"
	assertSourceHeaderLabel   = "Header"
	assertSourceRespTimeLabel = "Response time (ms)"
	assertSourceRawBodyLabel  = "Raw body"
)

// assertSourceLabels lists the Assertion Source selector values in display order.
var assertSourceLabels = []string{
	assertSourceStatusLabel,
	assertSourceJSONBodyLabel,
	assertSourceHeaderLabel,
	assertSourceRespTimeLabel,
	assertSourceRawBodyLabel,
}

// assertSourceFromLabel maps a Source selector label to its model.AssertSource
// (defaulting to AssertStatus). Pure + Fyne-free.
func assertSourceFromLabel(label string) model.AssertSource {
	switch label {
	case assertSourceJSONBodyLabel:
		return model.AssertJSONBody
	case assertSourceHeaderLabel:
		return model.AssertHeader
	case assertSourceRespTimeLabel:
		return model.AssertResponseTimeMs
	case assertSourceRawBodyLabel:
		return model.AssertRawBody
	default:
		return model.AssertStatus
	}
}

// assertSourceToLabel maps a model.AssertSource back to its selector label.
func assertSourceToLabel(s model.AssertSource) string {
	switch s {
	case model.AssertJSONBody:
		return assertSourceJSONBodyLabel
	case model.AssertHeader:
		return assertSourceHeaderLabel
	case model.AssertResponseTimeMs:
		return assertSourceRespTimeLabel
	case model.AssertRawBody:
		return assertSourceRawBodyLabel
	default:
		return assertSourceStatusLabel
	}
}

// --- Assertion operator labels (UI label ↔ model.AssertOp) ---

const (
	assertOpEqualsLabel      = "equals"
	assertOpNotEqualsLabel   = "not equals"
	assertOpContainsLabel    = "contains"
	assertOpNotContainsLabel = "not contains"
	assertOpExistsLabel      = "exists"
	assertOpNotExistsLabel   = "not exists"
	assertOpLessThanLabel    = "less than"
	assertOpGreaterThanLabel = "greater than"
	assertOpMatchesLabel     = "matches"
)

// assertOpLabels lists the Op selector values in display order.
var assertOpLabels = []string{
	assertOpEqualsLabel,
	assertOpNotEqualsLabel,
	assertOpContainsLabel,
	assertOpNotContainsLabel,
	assertOpExistsLabel,
	assertOpNotExistsLabel,
	assertOpLessThanLabel,
	assertOpGreaterThanLabel,
	assertOpMatchesLabel,
}

// assertOpFromLabel maps an Op selector label to its model.AssertOp (defaulting
// to OpEquals). Pure + Fyne-free.
func assertOpFromLabel(label string) model.AssertOp {
	switch label {
	case assertOpNotEqualsLabel:
		return model.OpNotEquals
	case assertOpContainsLabel:
		return model.OpContains
	case assertOpNotContainsLabel:
		return model.OpNotContains
	case assertOpExistsLabel:
		return model.OpExists
	case assertOpNotExistsLabel:
		return model.OpNotExists
	case assertOpLessThanLabel:
		return model.OpLessThan
	case assertOpGreaterThanLabel:
		return model.OpGreaterThan
	case assertOpMatchesLabel:
		return model.OpMatches
	default:
		return model.OpEquals
	}
}

// assertOpToLabel maps a model.AssertOp back to its selector label.
func assertOpToLabel(op model.AssertOp) string {
	switch op {
	case model.OpNotEquals:
		return assertOpNotEqualsLabel
	case model.OpContains:
		return assertOpContainsLabel
	case model.OpNotContains:
		return assertOpNotContainsLabel
	case model.OpExists:
		return assertOpExistsLabel
	case model.OpNotExists:
		return assertOpNotExistsLabel
	case model.OpLessThan:
		return assertOpLessThanLabel
	case model.OpGreaterThan:
		return assertOpGreaterThanLabel
	case model.OpMatches:
		return assertOpMatchesLabel
	default:
		return assertOpEqualsLabel
	}
}

// --- Pure row→model mapping helpers (Fyne-free, unit-testable) ---

// captureFromRow builds a model.Capture from a row's raw string field values.
// variable/expr are taken verbatim; sourceLabel is mapped through
// captureSourceFromLabel; enabled is the row's checkbox state. Keeping this pure
// lets the blind tester drive it without constructing widgets.
func captureFromRow(variable, sourceLabel, expr string, enabled bool) model.Capture {
	return model.Capture{
		Variable: variable,
		Source:   captureSourceFromLabel(sourceLabel),
		Expr:     expr,
		Enabled:  enabled,
	}
}

// assertionFromRow builds a model.Assertion from a row's raw string field values.
// sourceLabel/opLabel are mapped through their label→value helpers; expr and
// expected are taken verbatim; enabled is the row's checkbox state.
func assertionFromRow(sourceLabel, expr, opLabel, expected string, enabled bool) model.Assertion {
	return model.Assertion{
		Source:   assertSourceFromLabel(sourceLabel),
		Expr:     expr,
		Op:       assertOpFromLabel(opLabel),
		Expected: expected,
		Enabled:  enabled,
	}
}

// captureRowEmpty reports whether a Capture row carries no content (no variable
// and no expr), so a wholly-blank row isn't persisted (mirrors kvTable.value).
func captureRowEmpty(variable, expr string) bool {
	return variable == "" && expr == ""
}

// assertionRowEmpty reports whether an Assertion row carries no content. Source
// and Op always default-select, so a freshly "Add"ed, untouched row has no expr
// and no expected value; treat it as empty and drop it (mirrors
// captureRowEmpty) so an accidental Add never persists a noise assertion.
func assertionRowEmpty(expr, expected string) bool {
	return expr == "" && expected == ""
}

// --- testsTab UI ---

// captureRow is one editable model.Capture row in the Captures table.
type captureRow struct {
	enabled  *widget.Check
	variable *widget.Entry
	source   *widget.Select
	expr     *widget.Entry
	object   fyne.CanvasObject
}

// assertionRow is one editable model.Assertion row in the Assertions table.
type assertionRow struct {
	enabled  *widget.Check
	source   *widget.Select
	expr     *widget.Entry
	op       *widget.Select
	expected *widget.Entry
	object   fyne.CanvasObject
}

// testsTab is the request editor's "Tests" sub-tab: a Captures table stacked
// above an Assertions table. Both are editable VBox-of-rows tables (the kvTable
// idiom) and every edit calls onChange (rt.commit) — but only AFTER seeding, so
// opening a request with saved captures/assertions does not dirty a fresh tab.
type testsTab struct {
	container fyne.CanvasObject

	capRowsBox *fyne.Container
	capRows    []*captureRow

	assertRowsBox *fyne.Container
	assertRows    []*assertionRow

	onChange func()
}

// newTestsTab builds the Tests tab seeded from seed.Captures / seed.Assertions,
// wiring every edit to rt.commit(). Controls are seeded BEFORE their OnChanged
// handlers are wired, so seeding never fires commit on a freshly opened tab.
func newTestsTab(rt *requestTab, seed model.Request) *testsTab {
	t := &testsTab{onChange: func() { rt.commit() }}

	t.capRowsBox = container.NewVBox()
	for _, c := range seed.Captures {
		t.appendCaptureRow(c)
	}
	addCap := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		t.appendCaptureRow(model.Capture{Source: model.CaptureJSONBody, Enabled: true})
		t.fire()
	})
	addCap.Importance = widget.LowImportance

	// [On][Variable | Expr][Source][del] header.
	capHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("On", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Source", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2,
			widget.NewLabelWithStyle("Variable", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Expr (JSON path / header)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
	)
	capturesPane := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Captures", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			capHeader,
		),
		addCap, nil, nil, t.capRowsBox,
	)

	t.assertRowsBox = container.NewVBox()
	for _, a := range seed.Assertions {
		t.appendAssertionRow(a)
	}
	addAssert := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		t.appendAssertionRow(model.Assertion{Source: model.AssertStatus, Op: model.OpEquals, Enabled: true})
		t.fire()
	})
	addAssert.Importance = widget.LowImportance

	// [On][Source][Expr | Op | Expected][del] header.
	assertHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("On", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nil,
		container.NewGridWithColumns(4,
			widget.NewLabelWithStyle("Source", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Expr", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Op", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Expected", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
	)
	assertionsPane := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Assertions", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			assertHeader,
		),
		addAssert, nil, nil, t.assertRowsBox,
	)

	// Captures on top, Assertions below, the whole thing scrollable so long lists
	// stay reachable inside the request editor's split pane.
	body := container.NewVBox(
		capturesPane,
		widget.NewSeparator(),
		assertionsPane,
	)
	t.container = container.NewVScroll(body)
	return t
}

// fire notifies the owner (rt.commit) of an edit.
func (t *testsTab) fire() {
	if t.onChange != nil {
		t.onChange()
	}
}

// appendCaptureRow adds a row widget for c WITHOUT firing onChange (so seeding a
// fresh tab can't dirty it). The Add button and edit handlers fire explicitly.
func (t *testsTab) appendCaptureRow(c model.Capture) {
	r := &captureRow{}

	r.enabled = widget.NewCheck("", nil)
	r.enabled.SetChecked(c.Enabled)
	r.enabled.OnChanged = func(bool) { t.fire() }

	r.variable = widget.NewEntry()
	r.variable.SetPlaceHolder("token")
	r.variable.SetText(c.Variable)
	r.variable.OnChanged = func(string) { t.fire() }

	r.source = widget.NewSelect(captureSourceLabels, nil)
	r.source.SetSelected(captureSourceToLabel(c.Source))
	r.source.OnChanged = func(string) { t.fire() }

	r.expr = widget.NewEntry()
	r.expr.SetPlaceHolder("data.token / X-Token")
	r.expr.SetText(c.Expr)
	r.expr.OnChanged = func(string) { t.fire() }

	del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		t.removeCaptureRow(r)
		t.fire()
	})
	del.Importance = widget.LowImportance

	r.object = container.NewBorder(nil, nil, r.enabled,
		container.NewHBox(r.source, del),
		container.NewGridWithColumns(2, r.variable, r.expr),
	)

	t.capRows = append(t.capRows, r)
	t.capRowsBox.Add(r.object)
	t.capRowsBox.Refresh()
}

// removeCaptureRow drops a Capture row from the table.
func (t *testsTab) removeCaptureRow(target *captureRow) {
	for i, r := range t.capRows {
		if r == target {
			t.capRows = append(t.capRows[:i], t.capRows[i+1:]...)
			break
		}
	}
	t.capRowsBox.Remove(target.object)
	t.capRowsBox.Refresh()
}

// appendAssertionRow adds a row widget for a WITHOUT firing onChange.
func (t *testsTab) appendAssertionRow(a model.Assertion) {
	r := &assertionRow{}

	r.enabled = widget.NewCheck("", nil)
	r.enabled.SetChecked(a.Enabled)
	r.enabled.OnChanged = func(bool) { t.fire() }

	r.source = widget.NewSelect(assertSourceLabels, nil)
	r.source.SetSelected(assertSourceToLabel(a.Source))
	r.source.OnChanged = func(string) { t.fire() }

	r.expr = widget.NewEntry()
	r.expr.SetPlaceHolder("data.id / X-Token")
	r.expr.SetText(a.Expr)
	r.expr.OnChanged = func(string) { t.fire() }

	r.op = widget.NewSelect(assertOpLabels, nil)
	r.op.SetSelected(assertOpToLabel(a.Op))
	r.op.OnChanged = func(string) { t.fire() }

	r.expected = widget.NewEntry()
	r.expected.SetPlaceHolder("200")
	r.expected.SetText(a.Expected)
	r.expected.OnChanged = func(string) { t.fire() }

	del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		t.removeAssertionRow(r)
		t.fire()
	})
	del.Importance = widget.LowImportance

	r.object = container.NewBorder(nil, nil, r.enabled, del,
		container.NewGridWithColumns(4, r.source, r.expr, r.op, r.expected),
	)

	t.assertRows = append(t.assertRows, r)
	t.assertRowsBox.Add(r.object)
	t.assertRowsBox.Refresh()
}

// removeAssertionRow drops an Assertion row from the table.
func (t *testsTab) removeAssertionRow(target *assertionRow) {
	for i, r := range t.assertRows {
		if r == target {
			t.assertRows = append(t.assertRows[:i], t.assertRows[i+1:]...)
			break
		}
	}
	t.assertRowsBox.Remove(target.object)
	t.assertRowsBox.Refresh()
}

// captures reads the Captures table back into a []model.Capture, preserving
// order. Wholly-empty rows (no variable, no expr) are skipped; a disabled row
// with content is kept ("keep but don't run"). Returns nil when none, so the
// Request's omitempty drops the JSON key.
func (t *testsTab) captures() []model.Capture {
	var out []model.Capture
	for _, r := range t.capRows {
		if captureRowEmpty(r.variable.Text, r.expr.Text) {
			continue
		}
		out = append(out, captureFromRow(
			r.variable.Text, r.source.Selected, r.expr.Text, r.enabled.Checked))
	}
	return out
}

// assertions reads the Assertions table back into a []model.Assertion,
// preserving order. A row is kept whenever any of its content fields carry text
// (Status/exists checks need no expr/expected, so an empty-expr row is still
// meaningful); only a fully-default blank row is skipped. Returns nil when none.
func (t *testsTab) assertions() []model.Assertion {
	var out []model.Assertion
	for _, r := range t.assertRows {
		if assertionRowEmpty(r.expr.Text, r.expected.Text) {
			continue
		}
		out = append(out, assertionFromRow(
			r.source.Selected, r.expr.Text, r.op.Selected, r.expected.Text, r.enabled.Checked))
	}
	return out
}
