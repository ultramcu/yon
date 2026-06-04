package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// collPath returns a usable collection path inside a fresh temp dir.
func collPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "api.yon")
}

// readEnvJSON returns the raw committed JSON for the named environment by
// reading the single file under the collection's environments dir. (The tests
// here save one environment per collection, so there is exactly one file.)
func readEnvJSON(t *testing.T, cp, name string) string {
	t.Helper()
	dir := store.EnvironmentsDir(cp)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read env JSON: %v", err)
		}
		return string(data)
	}
	t.Fatalf("no environment file under %s", dir)
	return ""
}

// TestJumpHost_KeyAuthRoundTrip: a key-auth jump host with a passphrase
// round-trips through Save/Load — non-secret fields are in the JSON, the
// passphrase is NOT in the committed JSON but is restored from .env.
func TestJumpHost_KeyAuthRoundTrip(t *testing.T) {
	cp := collPath(t)
	env := model.Environment{
		Name: "Staging",
		JumpHost: &model.JumpHost{
			Host:       "jump.example.com",
			Port:       2222,
			User:       "deploy",
			Auth:       model.JumpAuthKey,
			KeyPath:    "/home/me/id_ed25519",
			Insecure:   true,
			Passphrase: "p4ss",
		},
	}
	if err := store.SaveEnvironment(cp, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	json := readEnvJSON(t, cp, "Staging")
	// Non-secret fields present.
	for _, want := range []string{`"host": "jump.example.com"`, `"port": 2222`,
		`"user": "deploy"`, `"auth": "key"`, `"keyPath": "/home/me/id_ed25519"`,
		`"insecure": true`} {
		if !strings.Contains(json, want) {
			t.Errorf("committed JSON missing %q\n%s", want, json)
		}
	}
	// Secret NOT in JSON.
	if strings.Contains(json, "p4ss") {
		t.Errorf("passphrase leaked into committed JSON:\n%s", json)
	}

	// Restored from .env on load.
	envs, err := store.LoadEnvironments(cp)
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}
	got, ok := findEnv(envs, "Staging")
	if !ok || got.JumpHost == nil {
		t.Fatalf("Staging jump host not loaded: %+v", envs)
	}
	if got.JumpHost.Passphrase != "p4ss" {
		t.Errorf("passphrase = %q; want restored %q", got.JumpHost.Passphrase, "p4ss")
	}
	if got.JumpHost.Host != "jump.example.com" || got.JumpHost.Port != 2222 ||
		got.JumpHost.User != "deploy" || got.JumpHost.Auth != model.JumpAuthKey ||
		got.JumpHost.KeyPath != "/home/me/id_ed25519" || !got.JumpHost.Insecure {
		t.Errorf("non-secret fields not round-tripped: %+v", got.JumpHost)
	}
}

// TestJumpHost_PasswordAuthRoundTrip: a password-auth jump host keeps the
// password out of the committed JSON and restores it from .env.
func TestJumpHost_PasswordAuthRoundTrip(t *testing.T) {
	cp := collPath(t)
	env := model.Environment{
		Name: "Prod",
		JumpHost: &model.JumpHost{
			Host:     "bastion.internal",
			User:     "ops",
			Auth:     model.JumpAuthPassword,
			Password: "hunter2",
		},
	}
	if err := store.SaveEnvironment(cp, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}
	if json := readEnvJSON(t, cp, "Prod"); strings.Contains(json, "hunter2") {
		t.Errorf("password leaked into committed JSON:\n%s", json)
	}
	envs, _ := store.LoadEnvironments(cp)
	got, ok := findEnv(envs, "Prod")
	if !ok || got.JumpHost == nil || got.JumpHost.Password != "hunter2" {
		t.Fatalf("password not restored: %+v", got.JumpHost)
	}
}

// TestJumpHost_BackwardCompatByteIdentical: an environment with NO jump host
// serializes byte-for-byte the same as it did before the feature, and the
// reserved keys never appear in .env.
func TestJumpHost_BackwardCompatByteIdentical(t *testing.T) {
	cp := collPath(t)
	env := model.Environment{
		Name: "Local",
		Variables: []model.Variable{
			{Key: "baseUrl", Value: "http://localhost", Enabled: true},
		},
	}
	if err := store.SaveEnvironment(cp, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}
	json := readEnvJSON(t, cp, "Local")
	// The jumpHost field is omitempty, so it must not appear at all.
	if strings.Contains(json, "jumpHost") {
		t.Errorf("jumpHost key present for an env without a jump host:\n%s", json)
	}
	// No reserved keys, so no .env file should exist at all here.
	if _, err := os.Stat(filepath.Join(filepath.Dir(cp), ".env")); !os.IsNotExist(err) {
		t.Errorf(".env exists for an env with no secrets/jump host (err=%v)", err)
	}
}

// TestJumpHost_ReservedKeyDoesNotClobberUserVariable: a user Variable named
// "password" (secret) and a jump host with a password coexist in .env without
// either overwriting the other.
func TestJumpHost_ReservedKeyDoesNotClobberUserVariable(t *testing.T) {
	cp := collPath(t)
	env := model.Environment{
		Name: "Staging",
		Variables: []model.Variable{
			{Key: "password", Value: "uservalue", Enabled: true, Secret: true},
		},
		JumpHost: &model.JumpHost{
			Host:     "h",
			User:     "u",
			Auth:     model.JumpAuthPassword,
			Password: "jumpvalue",
		},
	}
	if err := store.SaveEnvironment(cp, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}
	envs, err := store.LoadEnvironments(cp)
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}
	got, _ := findEnv(envs, "Staging")
	uv, ok := findVar(got, "password")
	if !ok || uv.Value != "uservalue" {
		t.Errorf("user Variable 'password' = %q; want %q", uv.Value, "uservalue")
	}
	if got.JumpHost == nil || got.JumpHost.Password != "jumpvalue" {
		t.Errorf("jump host password = %v; want %q", got.JumpHost, "jumpvalue")
	}
}

// TestJumpHost_DeletePrunesReservedKeys: deleting an environment removes its
// reserved jump-host keys from .env when no other environment uses them.
func TestJumpHost_DeletePrunesReservedKeys(t *testing.T) {
	cp := collPath(t)
	env := model.Environment{
		Name:     "Staging",
		JumpHost: &model.JumpHost{Host: "h", User: "u", Auth: model.JumpAuthPassword, Password: "p"},
	}
	if err := store.SaveEnvironment(cp, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}
	if err := store.DeleteEnvironment(cp, "Staging"); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	// .env should now be gone (it held only the reserved key).
	if _, err := os.Stat(filepath.Join(filepath.Dir(cp), ".env")); !os.IsNotExist(err) {
		data, _ := os.ReadFile(filepath.Join(filepath.Dir(cp), ".env"))
		t.Errorf(".env not pruned after delete: %q (err=%v)", data, err)
	}
}
