package variables

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// TestLookupRuntimePrecedence verifies the precedence chain: a Runtime value
// beats an environment value, which beats a collection value.
func TestLookupRuntimePrecedence(t *testing.T) {
	sc := Scope{
		Env: model.Environment{Variables: []model.Variable{
			{Key: "token", Value: "env-token", Enabled: true},
			{Key: "host", Value: "env-host", Enabled: true},
		}},
		Collection: []model.Variable{
			{Key: "token", Value: "coll-token", Enabled: true},
			{Key: "host", Value: "coll-host", Enabled: true},
			{Key: "only", Value: "coll-only", Enabled: true},
		},
		Runtime: map[string]string{"token": "runtime-token"},
	}

	cases := []struct {
		key, want string
	}{
		{"token", "runtime-token"}, // runtime beats env beats collection
		{"host", "env-host"},       // env beats collection
		{"only", "coll-only"},      // collection-only
	}
	for _, c := range cases {
		got, ok := sc.Lookup(c.key)
		if !ok || got != c.want {
			t.Errorf("Lookup(%q) = %q,%v; want %q,true", c.key, got, ok, c.want)
		}
	}
}

// TestLookupNilRuntime confirms a nil Runtime map does not break existing
// env/collection lookup behavior.
func TestLookupNilRuntime(t *testing.T) {
	sc := Scope{
		Env:        model.Environment{Variables: []model.Variable{{Key: "a", Value: "env-a", Enabled: true}}},
		Collection: []model.Variable{{Key: "b", Value: "coll-b", Enabled: true}},
	}
	if v, ok := sc.Lookup("a"); !ok || v != "env-a" {
		t.Errorf("Lookup(a) = %q,%v; want env-a,true", v, ok)
	}
	if v, ok := sc.Lookup("b"); !ok || v != "coll-b" {
		t.Errorf("Lookup(b) = %q,%v; want coll-b,true", v, ok)
	}
	if _, ok := sc.Lookup("missing"); ok {
		t.Error("Lookup(missing) = true; want false")
	}
}

// TestResolveUsesRuntime confirms Resolve picks up a captured runtime value so a
// captured token wins inside a {{token}} reference.
func TestResolveUsesRuntime(t *testing.T) {
	sc := Scope{
		Env:     model.Environment{Variables: []model.Variable{{Key: "token", Value: "env", Enabled: true}}},
		Runtime: map[string]string{"token": "captured"},
	}
	if got := sc.Resolve("Bearer {{token}}"); got != "Bearer captured" {
		t.Errorf("Resolve = %q; want Bearer captured", got)
	}
}
