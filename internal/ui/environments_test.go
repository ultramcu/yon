package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// newScopeWindow builds a Window backed by coll, the same way other ui tests do
// (test.NewApp + New(a) + newWindow). t.Cleanup closes the underlying window.
func newScopeWindow(t *testing.T, coll model.Collection) *Window {
	t.Helper()
	a := test.NewApp()
	w := newWindow(New(a), coll, "")
	t.Cleanup(w.win.Close)
	return w
}

// TestVarScope_CollectionVariableResolves pins that, with no active environment,
// varScope() exposes the collection's enabled variables to the resolver: an
// enabled collection var {{host}} expands to its value.
func TestVarScope_CollectionVariableResolves(t *testing.T) {
	coll := model.NewCollection("T")
	coll.Variables = []model.Variable{{Key: "host", Value: "local", Enabled: true}}
	coll.ActiveEnvironment = ""

	w := newScopeWindow(t, coll)
	if got := w.varScope().Resolve("{{host}}"); got != "local" {
		t.Fatalf("Resolve({{host}}) = %q, want %q", got, "local")
	}
}

// TestVarScope_UnknownVariableLeftLiteral pins that an unknown reference is left
// literal through the scope (Postman behaviour), not blanked.
func TestVarScope_UnknownVariableLeftLiteral(t *testing.T) {
	coll := model.NewCollection("T") // no variables, no active environment
	w := newScopeWindow(t, coll)
	if got := w.varScope().Resolve("{{missing}}"); got != "{{missing}}" {
		t.Fatalf("Resolve({{missing}}) = %q, want %q", got, "{{missing}}")
	}
}

// TestVarScope_DisabledCollectionVariableNotApplied pins that a disabled
// collection variable is not applied: its reference is left literal.
func TestVarScope_DisabledCollectionVariableNotApplied(t *testing.T) {
	coll := model.NewCollection("T")
	coll.Variables = []model.Variable{{Key: "x", Value: "v", Enabled: false}}
	w := newScopeWindow(t, coll)
	if got := w.varScope().Resolve("{{x}}"); got != "{{x}}" {
		t.Fatalf("Resolve({{x}}) = %q, want %q", got, "{{x}}")
	}
}

// TestVarScope_EnabledWinsOverDisabledSameKey is the precedence case that can be
// constructed without setting loaded environments (see the test-2 limitation
// note below): when two collection variables share a key, the disabled one is
// skipped and the enabled one applies. This exercises the same enabled-only +
// first-match resolution path that env-over-collection precedence relies on.
func TestVarScope_EnabledWinsOverDisabledSameKey(t *testing.T) {
	coll := model.NewCollection("T")
	coll.Variables = []model.Variable{
		{Key: "host", Value: "disabled", Enabled: false},
		{Key: "host", Value: "enabled", Enabled: true},
	}
	w := newScopeWindow(t, coll)
	if got := w.varScope().Resolve("{{host}}"); got != "enabled" {
		t.Fatalf("Resolve({{host}}) = %q, want %q", got, "enabled")
	}
}
