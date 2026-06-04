package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// Blind tests for issue #24: secret VARIABLE values were stored in the
// collection's shared .env keyed by the bare variable name, so two Environments
// with a same-named Secret shared one slot and the later save clobbered the
// earlier. These tests exercise only the PUBLIC store API + model types (plus
// the public store.EnvironmentsDir helper). Helpers are prefixed scol_ to avoid
// colliding with other test files in package store_test.

// scolCollPath returns a collection path inside a fresh temp dir. The .yon file
// itself need not exist; Save/Load only need the sibling .environments dir and
// the .env beside the collection.
func scolCollPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "api.yon")
}

// scolFindEnv returns the loaded environment with the given Name, failing if it
// is absent.
func scolFindEnv(t *testing.T, envs []model.Environment, name string) model.Environment {
	t.Helper()
	for _, e := range envs {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("environment %q not found among %d loaded envs", name, len(envs))
	return model.Environment{}
}

// scolVarValue returns the Value of the variable with the given Key, failing if
// the variable is absent.
func scolVarValue(t *testing.T, env model.Environment, key string) string {
	t.Helper()
	for _, v := range env.Variables {
		if v.Key == key {
			return v.Value
		}
	}
	t.Fatalf("variable %q not found in environment %q: %+v", key, env.Name, env.Variables)
	return ""
}

// secretEnv builds an Environment with a single secret variable.
func secretEnv(name, key, value string) model.Environment {
	return model.Environment{
		Name: name,
		Variables: []model.Variable{
			{Key: key, Value: value, Enabled: true, Secret: true},
		},
	}
}

// A. The bug itself. Two environments each declare a secret variable named
// "token" with DIFFERENT values. After a save/load round-trip each environment
// must read back its OWN value.
//
// Pre-fix the secret was keyed in the shared .env by the bare variable name
// "token", so the second save overwrote the first and BOTH environments loaded
// back the survivor ("BBB"). This test fails on that code (Prod's token == "BBB"
// instead of "AAA"). Post-fix each environment's secret key is namespaced so the
// two values coexist.
func TestSecretsIndependentAcrossEnvs(t *testing.T) {
	collPath := scolCollPath(t)

	if err := store.SaveEnvironment(collPath, secretEnv("Prod", "token", "AAA")); err != nil {
		t.Fatalf("SaveEnvironment(Prod): %v", err)
	}
	if err := store.SaveEnvironment(collPath, secretEnv("Staging", "token", "BBB")); err != nil {
		t.Fatalf("SaveEnvironment(Staging): %v", err)
	}

	envs, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}

	prod := scolFindEnv(t, envs, "Prod")
	staging := scolFindEnv(t, envs, "Staging")

	if got := scolVarValue(t, prod, "token"); got != "AAA" {
		t.Errorf("Prod token = %q, want %q (same-named secret clobbered across envs)", got, "AAA")
	}
	if got := scolVarValue(t, staging, "token"); got != "BBB" {
		t.Errorf("Staging token = %q, want %q (same-named secret clobbered across envs)", got, "BBB")
	}

	// The secret values must live in the .env, never in the committed env JSON.
	envDir := store.EnvironmentsDir(collPath)
	entries, err := os.ReadDir(envDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", envDir, err)
	}
	var combinedJSON string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(envDir, e.Name()))
		if err != nil {
			t.Fatalf("read env JSON %s: %v", e.Name(), err)
		}
		combinedJSON += string(b)
	}
	for _, secret := range []string{"AAA", "BBB"} {
		if strings.Contains(combinedJSON, secret) {
			t.Errorf("SECURITY: secret value %q leaked into committed env JSON:\n%s", secret, combinedJSON)
		}
	}
}

// B. Backward compatibility: an old (pre-fix) .env keyed the secret by the bare
// variable name. After the fix, LoadEnvironments must still restore that legacy
// value via a bare-key fallback.
//
// Setup: save an env "Old" with a secret token using the CURRENT code path, then
// rewrite the .env to the legacy bare form (token=legacy), removing any
// namespaced key the current writer may have produced. A reload must surface the
// legacy value through the env's "token" variable.
func TestSecretBackwardCompatLegacyBareKey(t *testing.T) {
	collPath := scolCollPath(t)

	if err := store.SaveEnvironment(collPath, secretEnv("Old", "token", "placeholder")); err != nil {
		t.Fatalf("SaveEnvironment(Old): %v", err)
	}

	// Emulate a pre-fix .env: a single bare `token=legacy` assignment, with no
	// namespaced keys present.
	dotenvPath := filepath.Join(filepath.Dir(collPath), ".env")
	if err := os.WriteFile(dotenvPath, []byte("token=legacy\n"), 0o644); err != nil {
		t.Fatalf("rewrite legacy .env: %v", err)
	}

	envs, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}
	old := scolFindEnv(t, envs, "Old")
	if got := scolVarValue(t, old, "token"); got != "legacy" {
		t.Errorf("legacy bare-key secret not restored: token = %q, want %q", got, "legacy")
	}
}

// C. Deleting one environment must not prune a same-named secret still used by
// another environment. Save Prod(token=AAA) and Staging(token=BBB), delete Prod,
// then Staging must still load its own token "BBB".
//
// Pre-fix both shared the bare "token" slot in .env, so this also guards against
// a delete that drops the shared key out from under the survivor; post-fix the
// keys are namespaced and Staging's slot is independent.
func TestDeleteOneSameNamedSecretEnvKeepsOther(t *testing.T) {
	collPath := scolCollPath(t)

	if err := store.SaveEnvironment(collPath, secretEnv("Prod", "token", "AAA")); err != nil {
		t.Fatalf("SaveEnvironment(Prod): %v", err)
	}
	if err := store.SaveEnvironment(collPath, secretEnv("Staging", "token", "BBB")); err != nil {
		t.Fatalf("SaveEnvironment(Staging): %v", err)
	}

	if err := store.DeleteEnvironment(collPath, "Prod"); err != nil {
		t.Fatalf("DeleteEnvironment(Prod): %v", err)
	}

	envs, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}

	// Prod is gone.
	for _, e := range envs {
		if e.Name == "Prod" {
			t.Fatalf("Prod still present after delete: %+v", envs)
		}
	}

	staging := scolFindEnv(t, envs, "Staging")
	if got := scolVarValue(t, staging, "token"); got != "BBB" {
		t.Errorf("Staging token after deleting Prod = %q, want %q (delete pruned survivor's secret)", got, "BBB")
	}
}
