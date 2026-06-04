package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// Dev tests for the secret-VARIABLE collision bug (issue #24): two Environments
// that each declare a Secret with the SAME Variable.Key used to share ONE .env
// slot (keyed by the bare key), so the later Save clobbered the earlier env's
// value and both read the survivor on reload. The fix namespaces each Secret's
// value per environment (varSecretKey, mirroring the jump-host keys). Helpers
// are prefixed scd_ to avoid collisions with other test files in the package.

// scdCollPath returns a collection path inside a fresh temp dir. The collection
// file itself need not exist for environment Save/Load (they live in sibling
// dirs / a .env beside it).
func scdCollPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "api.yon")
}

// scdLoadByName loads environments and returns the one with the given name.
func scdLoadByName(t *testing.T, collPath, name string) model.Environment {
	t.Helper()
	envs, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}
	for _, e := range envs {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("environment %q not found after load (got %d envs)", name, len(envs))
	return model.Environment{}
}

// scdSecretValue returns the value of the named secret variable in env.
func scdSecretValue(t *testing.T, env model.Environment, key string) string {
	t.Helper()
	for _, v := range env.Variables {
		if v.Key == key {
			return v.Value
		}
	}
	t.Fatalf("secret %q not found in environment %q", key, env.Name)
	return ""
}

// TestDev_SecretVariable_SameNameAcrossEnvsKeepsOwnValue is the core fail-before
// case: two environments in one collection each declare a Secret named "token"
// with DIFFERENT values. Before the fix both stored under the bare "token" key,
// so the second Save overwrote the first and BOTH read the survivor on reload —
// silent data loss. After the fix each env keeps its own value.
func TestDev_SecretVariable_SameNameAcrossEnvsKeepsOwnValue(t *testing.T) {
	collPath := scdCollPath(t)
	const valA = "SCD-TOKEN-FOR-ENV-A"
	const valB = "SCD-TOKEN-FOR-ENV-B"

	envA := model.Environment{
		Name: "Alpha",
		Variables: []model.Variable{
			{Key: "token", Value: valA, Enabled: true, Secret: true},
		},
	}
	envB := model.Environment{
		Name: "Bravo",
		Variables: []model.Variable{
			{Key: "token", Value: valB, Enabled: true, Secret: true},
		},
	}

	if err := store.SaveEnvironment(collPath, envA); err != nil {
		t.Fatalf("SaveEnvironment(Alpha): %v", err)
	}
	if err := store.SaveEnvironment(collPath, envB); err != nil {
		t.Fatalf("SaveEnvironment(Bravo): %v", err)
	}

	gotA := scdSecretValue(t, scdLoadByName(t, collPath, "Alpha"), "token")
	gotB := scdSecretValue(t, scdLoadByName(t, collPath, "Bravo"), "token")

	// On the pre-fix (bare-key) code both of these read valB (Bravo saved last),
	// so the Alpha assertion FAILS — that is the captured data-loss bug.
	if gotA != valA {
		t.Errorf("Alpha's 'token' clobbered: got %q want %q (same-named secret shared one .env slot)", gotA, valA)
	}
	if gotB != valB {
		t.Errorf("Bravo's 'token' wrong: got %q want %q", gotB, valB)
	}
}

// TestDev_SecretVariable_LegacyBareKeyBackwardCompat verifies the backward-compat
// fallback: a .env written before this fix holds the secret under the BARE key
// (e.g. `token=legacy`). With no namespaced key present, Load must fall back to
// the bare key so existing collections still resolve their secrets.
func TestDev_SecretVariable_LegacyBareKeyBackwardCompat(t *testing.T) {
	collPath := scdCollPath(t)
	const legacyVal = "SCD-LEGACY-BARE-TOKEN"

	// Save normally so the store lays down a correctly-named, committed-style env
	// JSON (secret value blanked) for an env declaring secret "token".
	if err := store.SaveEnvironment(collPath, model.Environment{
		Name:      "Legacy",
		Variables: []model.Variable{{Key: "token", Value: "ignored-namespaced", Enabled: true, Secret: true}},
	}); err != nil {
		t.Fatalf("SaveEnvironment(Legacy): %v", err)
	}

	// Simulate a PRE-FIX .env: overwrite it so the secret lives ONLY under the
	// bare key, with no namespaced key present. Load must fall back to the bare
	// key (the two-value map read distinguishes absent-namespaced from empty).
	dotenvPath := filepath.Join(filepath.Dir(collPath), ".env")
	if err := os.WriteFile(dotenvPath, []byte("token="+legacyVal+"\n"), 0o644); err != nil {
		t.Fatalf("write legacy .env: %v", err)
	}

	got := scdSecretValue(t, scdLoadByName(t, collPath, "Legacy"), "token")
	if got != legacyVal {
		t.Errorf("legacy bare-key secret not loaded via fallback: got %q want %q", got, legacyVal)
	}
}

// TestDev_SecretVariable_MigratesToNamespacedOnSave checks that re-saving an
// environment loaded from a legacy bare-key .env migrates its secret to the
// namespaced key while leaving the (now-orphaned) bare key in place.
func TestDev_SecretVariable_MigratesToNamespacedOnSave(t *testing.T) {
	collPath := scdCollPath(t)
	const val = "SCD-MIGRATE-TOKEN"

	env := model.Environment{
		Name:      "Mig",
		Variables: []model.Variable{{Key: "token", Value: val, Enabled: true, Secret: true}},
	}
	if err := store.SaveEnvironment(collPath, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	dotenvPath := filepath.Join(filepath.Dir(collPath), ".env")
	data, err := os.ReadFile(dotenvPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(data), "__var.") {
		t.Errorf("Save should write the secret under a reserved '__var.' namespaced key.\n.env:\n%s", data)
	}
	if !strings.Contains(string(data), val) {
		t.Errorf(".env missing the secret value.\n.env:\n%s", data)
	}

	got := scdSecretValue(t, scdLoadByName(t, collPath, "Mig"), "token")
	if got != val {
		t.Errorf("namespaced secret not restored: got %q want %q", got, val)
	}
}

// TestDev_SecretVariable_DeletePrunesOwnKeyOnly verifies pruning: with two envs
// that share a same-named secret (each in its own namespaced slot), deleting one
// removes only that env's namespaced key; the survivor's secret is untouched.
func TestDev_SecretVariable_DeletePrunesOwnKeyOnly(t *testing.T) {
	collPath := scdCollPath(t)
	const valKeep = "SCD-KEEP-TOKEN"
	const valDrop = "SCD-DROP-TOKEN"

	keep := model.Environment{
		Name:      "Keep",
		Variables: []model.Variable{{Key: "token", Value: valKeep, Enabled: true, Secret: true}},
	}
	drop := model.Environment{
		Name:      "Drop",
		Variables: []model.Variable{{Key: "token", Value: valDrop, Enabled: true, Secret: true}},
	}
	if err := store.SaveEnvironment(collPath, keep); err != nil {
		t.Fatalf("SaveEnvironment(Keep): %v", err)
	}
	if err := store.SaveEnvironment(collPath, drop); err != nil {
		t.Fatalf("SaveEnvironment(Drop): %v", err)
	}

	if err := store.DeleteEnvironment(collPath, "Drop"); err != nil {
		t.Fatalf("DeleteEnvironment(Drop): %v", err)
	}

	// Survivor keeps its own value.
	got := scdSecretValue(t, scdLoadByName(t, collPath, "Keep"), "token")
	if got != valKeep {
		t.Errorf("survivor's secret lost after deleting same-named sibling: got %q want %q", got, valKeep)
	}

	// The deleted env's namespaced key is pruned: its value no longer appears in
	// the .env. (The survivor's value must still be there.)
	dotenvPath := filepath.Join(filepath.Dir(collPath), ".env")
	data, err := os.ReadFile(dotenvPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if strings.Contains(string(data), valDrop) {
		t.Errorf("deleted env's secret value should have been pruned from .env.\n.env:\n%s", data)
	}
	if !strings.Contains(string(data), valKeep) {
		t.Errorf("survivor's secret value missing from .env after sibling delete.\n.env:\n%s", data)
	}
}
