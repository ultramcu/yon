package variables

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// TestBlindSecret_ValueFallback_NilSecrets covers SPEC A.(a): a Secret variable
// whose own Value holds the secret and Secrets is nil resolves to that Value.
func TestBlindSecret_ValueFallback_NilSecrets(t *testing.T) {
	sc := Scope{
		Collection: []model.Variable{
			{Key: "token", Value: "s3cret", Enabled: true, Secret: true},
		},
		Secrets: nil,
	}

	got, ok := sc.Lookup("token")
	if !ok {
		t.Fatalf("Lookup(token): expected found=true, got false")
	}
	if got != "s3cret" {
		t.Fatalf("Lookup(token) = %q, want %q", got, "s3cret")
	}

	if r := sc.Resolve("t={{token}}"); r != "t=s3cret" {
		t.Fatalf("Resolve = %q, want %q", r, "t=s3cret")
	}
}

// TestBlindSecret_MapWins covers SPEC A.(b): when the variable's Value is empty
// but Secrets has the key, the map value is returned.
func TestBlindSecret_MapWins(t *testing.T) {
	sc := Scope{
		Collection: []model.Variable{
			{Key: "token", Value: "", Enabled: true, Secret: true},
		},
		Secrets: map[string]string{"token": "s3cret"},
	}

	got, ok := sc.Lookup("token")
	if !ok {
		t.Fatalf("Lookup(token): expected found=true, got false")
	}
	if got != "s3cret" {
		t.Fatalf("Lookup(token) = %q, want %q (map should win)", got, "s3cret")
	}
}

// TestBlindSecret_NonSecretResolvesToValue covers SPEC A.(c): a normal (non
// secret) variable resolves to its own Value regardless of the Secrets map.
func TestBlindSecret_NonSecretResolvesToValue(t *testing.T) {
	sc := Scope{
		Collection: []model.Variable{
			{Key: "host", Value: "example.com", Enabled: true, Secret: false},
		},
		Secrets: map[string]string{"host": "should-not-be-used"},
	}

	got, ok := sc.Lookup("host")
	if !ok {
		t.Fatalf("Lookup(host): expected found=true, got false")
	}
	if got != "example.com" {
		t.Fatalf("Lookup(host) = %q, want %q", got, "example.com")
	}
}

// TestBlindSecret_EnvPrecedenceOverCollection covers SPEC A.(d): an enabled env
// variable wins over a collection variable with the same key.
func TestBlindSecret_EnvPrecedenceOverCollection(t *testing.T) {
	sc := Scope{
		Env: model.Environment{
			Name: "prod",
			Variables: []model.Variable{
				{Key: "base", Value: "from-env", Enabled: true},
			},
		},
		Collection: []model.Variable{
			{Key: "base", Value: "from-collection", Enabled: true},
		},
	}

	got, ok := sc.Lookup("base")
	if !ok {
		t.Fatalf("Lookup(base): expected found=true, got false")
	}
	if got != "from-env" {
		t.Fatalf("Lookup(base) = %q, want %q (env should win)", got, "from-env")
	}
}
