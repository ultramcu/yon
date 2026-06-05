package ui

// Blind tests for issue #27 (capture/assertions Tests tab), authored against
// the CONTRACT only — not Dev B's implementation. The single most valuable
// behaviour these pin is the END-TO-END runtime-variable wiring: the Window
// must carry a runtimeVars map, and varScope() must fold it into the resolver
// as variables.Scope.Runtime (HIGHEST precedence) so a value captured from one
// response resolves as {{name}} in the next send and overrides any env or
// collection variable of the same name.
//
// The runtime-resolution tests are deliberately BLIND to Dev B's helper names:
// they drive only the package surface the contract guarantees (w.runtimeVars,
// w.varScope().Resolve). teststab.go has since landed, so the model-row mapping
// round-trip tests below (TestCaptureRowMapping / TestAssertionRowMapping) call
// Dev B's pure, Fyne-free helpers (captureFromRow, assertionFromRow,
// captureRowEmpty, and the Source/Op label↔value maps) directly.
//
// FAIL-BEFORE: as of authoring, Window has no runtimeVars field and varScope()
// does not set Scope.Runtime. This file therefore does NOT COMPILE against the
// current tree (undefined: w.runtimeVars) — that compile failure IS the
// fail-before evidence. After Dev B lands the field + wiring, it compiles and
// passes.

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// newRuntimeTestWindow builds a real Window on the headless Fyne test driver via
// the same path as smoke_test.go, so varScope() exercises the genuine wiring
// (active environment + collection variables + runtime), not a hand-rolled Scope.
func newRuntimeTestWindow(t *testing.T, coll model.Collection) *Window {
	t.Helper()
	fyneApp := test.NewApp()
	app := New(fyneApp)
	w := app.OpenCollectionWindow(coll, "/tmp/teststab_blindtest.yon")
	if w == nil {
		t.Fatal("OpenCollectionWindow returned nil")
	}
	return w
}

// TestRuntimeVarsResolveHighestPrecedence pins the contract's most valuable wire:
// runtime-captured variables resolve through the REAL Window.varScope() and beat
// both env and collection variables of the same name, a nil runtime map is safe
// (falls back to env/collection), and a runtime-only var resolves on its own.
func TestRuntimeVarsResolveHighestPrecedence(t *testing.T) {
	// A collection that defines `token` (collection scope) and an environment
	// that also defines `token` (env scope, normally higher than collection).
	// Runtime must beat BOTH.
	coll := model.NewCollection("RuntimePrecedence")
	coll.Variables = []model.Variable{
		{Key: "token", Value: "COLL", Enabled: true},
		{Key: "collOnly", Value: "C_ONLY", Enabled: true},
	}
	coll.ActiveEnvironment = "dev"

	w := newRuntimeTestWindow(t, coll)

	// Inject an active environment named "dev" that binds `token` = "ENV" so we
	// can prove runtime beats env (the strongest of the static scopes), not just
	// collection. We set it directly on the loaded list the same way the manager
	// would, then point ActiveEnvironment at it.
	w.envs = []model.Environment{
		{
			Name: "dev",
			Variables: []model.Variable{
				{Key: "token", Value: "ENV", Enabled: true},
			},
		},
	}

	// Sanity: with NO runtime values, resolution falls back to env/collection.
	// `token` should resolve to the ENV value (env beats collection), proving the
	// baseline precedence is intact before runtime is layered on.
	if w.runtimeVars != nil {
		t.Fatalf("runtimeVars should start nil/empty on a fresh window, got %v", w.runtimeVars)
	}
	if got := w.varScope().Resolve("{{token}}"); got != "ENV" {
		t.Fatalf("baseline (nil runtime): {{token}} = %q, want env value %q", got, "ENV")
	}
	// A nil runtime map must be SAFE (no panic) even for an unknown key; an
	// unknown {{x}} is left literal per the resolver contract.
	if got := w.varScope().Resolve("{{nope}}"); got != "{{nope}}" {
		t.Fatalf("nil runtime, unknown key: {{nope}} = %q, want literal %q", got, "{{nope}}")
	}

	// Now layer a runtime capture: token -> "RT". This is the field Dev B adds
	// (w.runtimeVars) and the value a Capture would write after a send.
	w.runtimeVars = map[string]string{"token": "RT"}

	if got := w.varScope().Resolve("{{token}}"); got != "RT" {
		t.Errorf("runtime override: {{token}} = %q, want runtime value %q (must beat env %q and collection %q)",
			got, "RT", "ENV", "COLL")
	}

	// A variable present ONLY in runtime (no env/collection entry) must resolve.
	w.runtimeVars["sessionId"] = "S123"
	if got := w.varScope().Resolve("{{sessionId}}"); got != "S123" {
		t.Errorf("runtime-only var: {{sessionId}} = %q, want %q", got, "S123")
	}

	// A variable NOT in runtime must still fall through to the static scopes,
	// i.e. runtime is additive (highest precedence) and does not shadow others.
	if got := w.varScope().Resolve("{{collOnly}}"); got != "C_ONLY" {
		t.Errorf("fall-through past runtime: {{collOnly}} = %q, want collection value %q", got, "C_ONLY")
	}

	// Mixed template: runtime + collection in one string resolves each from its
	// winning scope in a single Resolve call.
	if got := w.varScope().Resolve("{{token}}/{{collOnly}}"); got != "RT/C_ONLY" {
		t.Errorf("mixed template = %q, want %q", got, "RT/C_ONLY")
	}
}

// TestRuntimeVarsEmptyMapFallsBack pins that an explicitly-empty (non-nil)
// runtime map behaves identically to nil: resolution falls through to the
// env/collection scopes with no panic.
func TestRuntimeVarsEmptyMapFallsBack(t *testing.T) {
	coll := model.NewCollection("RuntimeEmpty")
	coll.Variables = []model.Variable{
		{Key: "host", Value: "api.example.com", Enabled: true},
	}
	w := newRuntimeTestWindow(t, coll)

	w.runtimeVars = map[string]string{} // empty, non-nil

	if got := w.varScope().Resolve("{{host}}"); got != "api.example.com" {
		t.Errorf("empty runtime map: {{host}} = %q, want collection value %q", got, "api.example.com")
	}
}

// TestCaptureRowMapping pins Dev B's pure capture-row → model.Capture mapping:
// the Source label maps to the right model.CaptureSource (with an unknown/empty
// label defaulting to jsonBody), variable/expr pass through verbatim, the row's
// checkbox drives Enabled, and a wholly-empty row is reported empty (so captures()
// drops it).
func TestCaptureRowMapping(t *testing.T) {
	tests := []struct {
		name        string
		variable    string
		sourceLabel string
		expr        string
		enabled     bool
		want        model.Capture
	}{
		{
			name:        "json body, enabled",
			variable:    "token",
			sourceLabel: captureSourceJSONBodyLabel,
			expr:        "data.token",
			enabled:     true,
			want:        model.Capture{Variable: "token", Source: model.CaptureJSONBody, Expr: "data.token", Enabled: true},
		},
		{
			name:        "header, disabled (kept but won't run)",
			variable:    "rid",
			sourceLabel: captureSourceHeaderLabel,
			expr:        "X-Request-Id",
			enabled:     false,
			want:        model.Capture{Variable: "rid", Source: model.CaptureHeader, Expr: "X-Request-Id", Enabled: false},
		},
		{
			name:        "unknown/empty source label defaults to jsonBody",
			variable:    "v",
			sourceLabel: "",
			expr:        "a.b",
			enabled:     true,
			want:        model.Capture{Variable: "v", Source: model.CaptureJSONBody, Expr: "a.b", Enabled: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := captureFromRow(tc.variable, tc.sourceLabel, tc.expr, tc.enabled)
			if got != tc.want {
				t.Errorf("captureFromRow(%q,%q,%q,%v) = %+v, want %+v",
					tc.variable, tc.sourceLabel, tc.expr, tc.enabled, got, tc.want)
			}
		})
	}

	// Source label round-trips through to-label and back.
	for _, s := range []model.CaptureSource{model.CaptureJSONBody, model.CaptureHeader} {
		if got := captureSourceFromLabel(captureSourceToLabel(s)); got != s {
			t.Errorf("capture source round-trip for %q = %q", s, got)
		}
	}

	// A wholly-blank row (no variable, no expr) is empty → excluded by captures().
	if !captureRowEmpty("", "") {
		t.Error("captureRowEmpty(\"\",\"\") = false, want true (blank row must be dropped)")
	}
	// Any content keeps the row.
	if captureRowEmpty("token", "") {
		t.Error("captureRowEmpty(\"token\",\"\") = true, want false (variable-only row is kept)")
	}
	if captureRowEmpty("", "data.x") {
		t.Error("captureRowEmpty(\"\",\"data.x\") = true, want false (expr-only row is kept)")
	}
}

// TestAssertionRowMapping pins Dev B's pure assertion-row → model.Assertion
// mapping: Source and Op labels map to the right model values (each defaulting
// sensibly on an unknown label — Source→status, Op→equals), expr/expected pass
// through verbatim, and the checkbox drives Enabled.
func TestAssertionRowMapping(t *testing.T) {
	tests := []struct {
		name        string
		sourceLabel string
		expr        string
		opLabel     string
		expected    string
		enabled     bool
		want        model.Assertion
	}{
		{
			name:        "status equals 200, enabled",
			sourceLabel: assertSourceStatusLabel,
			expr:        "",
			opLabel:     assertOpEqualsLabel,
			expected:    "200",
			enabled:     true,
			want:        model.Assertion{Source: model.AssertStatus, Expr: "", Op: model.OpEquals, Expected: "200", Enabled: true},
		},
		{
			name:        "json body contains, enabled",
			sourceLabel: assertSourceJSONBodyLabel,
			expr:        "data.name",
			opLabel:     assertOpContainsLabel,
			expected:    "alice",
			enabled:     true,
			want:        model.Assertion{Source: model.AssertJSONBody, Expr: "data.name", Op: model.OpContains, Expected: "alice", Enabled: true},
		},
		{
			name:        "header exists, disabled",
			sourceLabel: assertSourceHeaderLabel,
			expr:        "X-Token",
			opLabel:     assertOpExistsLabel,
			expected:    "",
			enabled:     false,
			want:        model.Assertion{Source: model.AssertHeader, Expr: "X-Token", Op: model.OpExists, Expected: "", Enabled: false},
		},
		{
			name:        "response time less than, enabled",
			sourceLabel: assertSourceRespTimeLabel,
			expr:        "",
			opLabel:     assertOpLessThanLabel,
			expected:    "500",
			enabled:     true,
			want:        model.Assertion{Source: model.AssertResponseTimeMs, Expr: "", Op: model.OpLessThan, Expected: "500", Enabled: true},
		},
		{
			name:        "raw body matches, enabled",
			sourceLabel: assertSourceRawBodyLabel,
			expr:        "",
			opLabel:     assertOpMatchesLabel,
			expected:    "^ok$",
			enabled:     true,
			want:        model.Assertion{Source: model.AssertRawBody, Expr: "", Op: model.OpMatches, Expected: "^ok$", Enabled: true},
		},
		{
			name:        "unknown labels default to status/equals",
			sourceLabel: "???",
			expr:        "",
			opLabel:     "???",
			expected:    "x",
			enabled:     true,
			want:        model.Assertion{Source: model.AssertStatus, Expr: "", Op: model.OpEquals, Expected: "x", Enabled: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := assertionFromRow(tc.sourceLabel, tc.expr, tc.opLabel, tc.expected, tc.enabled)
			if got != tc.want {
				t.Errorf("assertionFromRow(%q,%q,%q,%q,%v) = %+v, want %+v",
					tc.sourceLabel, tc.expr, tc.opLabel, tc.expected, tc.enabled, got, tc.want)
			}
		})
	}

	// Every Source and Op label round-trips through to-label and back, covering
	// the full set Dev B exposes in the selectors.
	for _, s := range []model.AssertSource{
		model.AssertStatus, model.AssertJSONBody, model.AssertHeader,
		model.AssertResponseTimeMs, model.AssertRawBody,
	} {
		if got := assertSourceFromLabel(assertSourceToLabel(s)); got != s {
			t.Errorf("assert source round-trip for %q = %q", s, got)
		}
	}
	for _, op := range []model.AssertOp{
		model.OpEquals, model.OpNotEquals, model.OpContains, model.OpNotContains,
		model.OpExists, model.OpNotExists, model.OpLessThan, model.OpGreaterThan, model.OpMatches,
	} {
		if got := assertOpFromLabel(assertOpToLabel(op)); got != op {
			t.Errorf("assert op round-trip for %q = %q", op, got)
		}
	}
}
