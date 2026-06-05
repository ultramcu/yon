package variables

// Blind test for issue #27 — Scope.Runtime precedence. Written from the
// contract: Runtime is a map[string]string with the highest precedence
// (Runtime > Env > Collection). Only contract symbols are used.

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// TestScopeRuntimePrecedence pins that Runtime wins over Env and Collection,
// that lower tiers are used when Runtime lacks the key, that a runtime-only
// variable resolves, and that a nil Runtime map does not break resolution.
func TestScopeRuntimePrecedence(t *testing.T) {
	t.Parallel()

	envWith := func(val string) model.Environment {
		return model.Environment{
			Name:      "env",
			Variables: []model.Variable{{Key: "token", Value: val, Enabled: true}},
		}
	}
	collWith := func(val string) []model.Variable {
		return []model.Variable{{Key: "token", Value: val, Enabled: true}}
	}

	t.Run("runtime beats env and collection", func(t *testing.T) {
		t.Parallel()
		sc := Scope{
			Runtime:    map[string]string{"token": "RT"},
			Env:        envWith("ENV"),
			Collection: collWith("COLL"),
		}
		if got := sc.Resolve("{{token}}"); got != "RT" {
			t.Fatalf("Resolve = %q, want %q", got, "RT")
		}
	})

	t.Run("env used when runtime lacks key", func(t *testing.T) {
		t.Parallel()
		sc := Scope{
			Runtime:    map[string]string{},
			Env:        envWith("ENV"),
			Collection: collWith("COLL"),
		}
		if got := sc.Resolve("{{token}}"); got != "ENV" {
			t.Fatalf("Resolve = %q, want %q", got, "ENV")
		}
	})

	t.Run("collection used when only collection has it", func(t *testing.T) {
		t.Parallel()
		sc := Scope{
			Collection: collWith("COLL"),
		}
		if got := sc.Resolve("{{token}}"); got != "COLL" {
			t.Fatalf("Resolve = %q, want %q", got, "COLL")
		}
	})

	t.Run("runtime-only variable resolves", func(t *testing.T) {
		t.Parallel()
		sc := Scope{Runtime: map[string]string{"only": "X"}}
		if got, ok := sc.Lookup("only"); !ok || got != "X" {
			t.Fatalf("Lookup(only) = %q,%v, want \"X\",true", got, ok)
		}
		if got := sc.Resolve("v={{only}}"); got != "v=X" {
			t.Fatalf("Resolve = %q, want %q", got, "v=X")
		}
	})

	t.Run("nil runtime does not break resolution", func(t *testing.T) {
		t.Parallel()
		sc := Scope{
			Runtime:    nil,
			Collection: collWith("COLL"),
		}
		if _, ok := sc.Lookup("missing"); ok {
			t.Fatalf("Lookup(missing) ok=true, want false")
		}
		if got := sc.Resolve("{{token}}"); got != "COLL" {
			t.Fatalf("Resolve = %q, want %q", got, "COLL")
		}
	})
}
