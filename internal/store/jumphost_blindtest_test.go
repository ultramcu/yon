package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// Blind tests for the SSH jump-host DATA LAYER store round-trip (SPEC 1-4).
// Written independently from the spec. Helpers are prefixed bjhs_ to avoid
// collisions with other test files in the package.

// bjhsCollPath returns a collection path inside a fresh temp dir. The collection
// file itself need not exist for environment Save/Load (they live in sibling
// dirs / a .env beside it).
func bjhsCollPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "api.yon")
}

// bjhsReadEnvJSON locates and reads the single environment JSON file under the
// collection's .environments dir. Fails if not exactly one is found.
func bjhsReadEnvJSON(t *testing.T, collPath string) string {
	t.Helper()
	dir := store.EnvironmentsDir(collPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read environments dir %q: %v", dir, err)
	}
	var found string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			if found != "" {
				t.Fatalf("expected exactly one env JSON file, found multiple in %q", dir)
			}
			found = filepath.Join(dir, e.Name())
		}
	}
	if found == "" {
		t.Fatalf("no env JSON file found in %q", dir)
	}
	data, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("read env JSON %q: %v", found, err)
	}
	return string(data)
}

// bjhsReadDotenv reads the .env beside the collection, returning ("", false) if
// it does not exist.
func bjhsReadDotenv(t *testing.T, collPath string) (string, bool) {
	t.Helper()
	p := filepath.Join(filepath.Dir(collPath), ".env")
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read .env %q: %v", p, err)
	}
	return string(data), true
}

// bjhsLoadByName loads environments and returns the one with the given name.
func bjhsLoadByName(t *testing.T, collPath, name string) model.Environment {
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

// SPEC 1: round-trip with key auth. All fields restored on load; committed JSON
// has Host/Port/User/Auth/KeyPath but NOT the Passphrase value; the Passphrase
// is present in .env.
func TestBlind_JumpHostStore_KeyAuthRoundTrip(t *testing.T) {
	collPath := bjhsCollPath(t)
	const passphrase = "BJHS-KEY-PASSPHRASE-SENTINEL"
	env := model.Environment{
		Name: "Staging",
		JumpHost: &model.JumpHost{
			Host:       "bastion.example.com",
			Port:       2222,
			User:       "deploy",
			Auth:       model.JumpAuthKey,
			KeyPath:    "/home/deploy/id_ed25519",
			Insecure:   true,
			Passphrase: passphrase,
		},
	}

	if err := store.SaveEnvironment(collPath, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	jsonStr := bjhsReadEnvJSON(t, collPath)
	// Non-secret fields present in the committed JSON.
	for _, want := range []string{"bastion.example.com", "2222", "deploy", "key", "/home/deploy/id_ed25519"} {
		if !strings.Contains(jsonStr, want) {
			t.Errorf("committed JSON missing expected non-secret value %q.\nJSON:\n%s", want, jsonStr)
		}
	}
	// The secret must NOT be in the committed JSON.
	if strings.Contains(jsonStr, passphrase) {
		t.Errorf("SECURITY: passphrase value leaked into committed env JSON:\n%s", jsonStr)
	}

	// The secret MUST be in the .env file (under a reserved namespaced key).
	dotenv, ok := bjhsReadDotenv(t, collPath)
	if !ok {
		t.Fatalf("expected a .env file holding the jump-host passphrase; none found")
	}
	if !strings.Contains(dotenv, passphrase) {
		t.Errorf(".env missing the passphrase value.\n.env:\n%s", dotenv)
	}
	if !strings.Contains(dotenv, "__jumphost.") {
		t.Errorf(".env should key the jump-host secret under a reserved '__jumphost.' namespace.\n.env:\n%s", dotenv)
	}

	// Load restores ALL fields including the secret.
	got := bjhsLoadByName(t, collPath, "Staging")
	if got.JumpHost == nil {
		t.Fatalf("loaded environment has nil JumpHost")
	}
	jh := *got.JumpHost
	if jh.Host != "bastion.example.com" || jh.Port != 2222 || jh.User != "deploy" ||
		jh.Auth != model.JumpAuthKey || jh.KeyPath != "/home/deploy/id_ed25519" || jh.Insecure != true {
		t.Errorf("non-secret fields not restored: %+v", jh)
	}
	if jh.Passphrase != passphrase {
		t.Errorf("Passphrase not restored on load: got %q want %q", jh.Passphrase, passphrase)
	}
}

// SPEC 2: round-trip with password auth. Password not in JSON, present in .env,
// restored on load.
func TestBlind_JumpHostStore_PasswordAuthRoundTrip(t *testing.T) {
	collPath := bjhsCollPath(t)
	const password = "BJHS-PW-SENTINEL"
	env := model.Environment{
		Name: "Prod",
		JumpHost: &model.JumpHost{
			Host:     "jump.prod.internal",
			User:     "ops",
			Auth:     model.JumpAuthPassword,
			Password: password,
		},
	}

	if err := store.SaveEnvironment(collPath, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	jsonStr := bjhsReadEnvJSON(t, collPath)
	if strings.Contains(jsonStr, password) {
		t.Errorf("SECURITY: password value leaked into committed env JSON:\n%s", jsonStr)
	}
	if !strings.Contains(jsonStr, "password") {
		// The Auth value "password" should still appear; sanity that we didn't
		// strip the whole jump host.
		t.Errorf("committed JSON unexpectedly missing the auth kind 'password':\n%s", jsonStr)
	}

	dotenv, ok := bjhsReadDotenv(t, collPath)
	if !ok {
		t.Fatalf("expected a .env file holding the jump-host password; none found")
	}
	if !strings.Contains(dotenv, password) {
		t.Errorf(".env missing the password value.\n.env:\n%s", dotenv)
	}

	got := bjhsLoadByName(t, collPath, "Prod")
	if got.JumpHost == nil {
		t.Fatalf("loaded environment has nil JumpHost")
	}
	if got.JumpHost.Password != password {
		t.Errorf("Password not restored on load: got %q want %q", got.JumpHost.Password, password)
	}
	if got.JumpHost.Auth != model.JumpAuthPassword {
		t.Errorf("Auth not restored: got %q want %q", got.JumpHost.Auth, model.JumpAuthPassword)
	}
}

// SPEC 3: backward compat. An Environment with NO JumpHost serializes with no
// "jumpHost" key, loads unchanged, and leaves no stray reserved keys; with no
// secrets at all there is no .env file.
func TestBlind_JumpHostStore_NoJumpHostBackwardCompat(t *testing.T) {
	collPath := bjhsCollPath(t)
	env := model.Environment{
		Name: "Local",
		Variables: []model.Variable{
			{Key: "base", Value: "http://localhost", Enabled: true},
		},
	}

	if err := store.SaveEnvironment(collPath, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	jsonStr := bjhsReadEnvJSON(t, collPath)
	if strings.Contains(jsonStr, "jumpHost") {
		t.Errorf("env with no jump host should not serialize a 'jumpHost' key:\n%s", jsonStr)
	}

	// No secrets of any kind -> no .env file should be written.
	if dotenv, ok := bjhsReadDotenv(t, collPath); ok {
		t.Errorf("expected no .env file when there are no secrets; found one:\n%s", dotenv)
	}

	got := bjhsLoadByName(t, collPath, "Local")
	if got.JumpHost != nil {
		t.Errorf("loaded JumpHost should be nil, got %+v", *got.JumpHost)
	}
	if len(got.Variables) != 1 || got.Variables[0].Key != "base" || got.Variables[0].Value != "http://localhost" {
		t.Errorf("variables not loaded unchanged: %+v", got.Variables)
	}
}

// SPEC 3 (stray reserved keys): a non-secret-only jump host (key auth, no
// passphrase, no password) writes NO reserved __jumphost. keys, because there is
// nothing secret to store.
func TestBlind_JumpHostStore_NoSecretsNoReservedKeys(t *testing.T) {
	collPath := bjhsCollPath(t)
	env := model.Environment{
		Name: "Staging",
		JumpHost: &model.JumpHost{
			Host:    "bastion",
			User:    "deploy",
			Auth:    model.JumpAuthKey,
			KeyPath: "/k", // key auth but no passphrase set
		},
	}
	if err := store.SaveEnvironment(collPath, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}
	if dotenv, ok := bjhsReadDotenv(t, collPath); ok {
		if strings.Contains(dotenv, "__jumphost.") {
			t.Errorf("no jump-host secrets were set, so no reserved __jumphost. keys should appear:\n%s", dotenv)
		}
	}
	// And it still round-trips.
	got := bjhsLoadByName(t, collPath, "Staging")
	if got.JumpHost == nil || got.JumpHost.Host != "bastion" || got.JumpHost.KeyPath != "/k" {
		t.Errorf("jump host without secrets did not round-trip: %+v", got.JumpHost)
	}
}

// SPEC 4: no collision. A user Secret Variable literally named "password"
// coexists in .env with a JumpHost.Password without either clobbering the other.
func TestBlind_JumpHostStore_NoCollisionWithUserPasswordSecret(t *testing.T) {
	collPath := bjhsCollPath(t)
	const userPwVal = "BJHS-USER-VARIABLE-PASSWORD"
	const jumpPwVal = "BJHS-JUMPHOST-PASSWORD"
	env := model.Environment{
		Name: "Staging",
		Variables: []model.Variable{
			{Key: "password", Value: userPwVal, Enabled: true, Secret: true},
		},
		JumpHost: &model.JumpHost{
			Host:     "bastion",
			User:     "deploy",
			Auth:     model.JumpAuthPassword,
			Password: jumpPwVal,
		},
	}

	if err := store.SaveEnvironment(collPath, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	dotenv, ok := bjhsReadDotenv(t, collPath)
	if !ok {
		t.Fatalf("expected a .env file; none found")
	}
	if !strings.Contains(dotenv, userPwVal) {
		t.Errorf(".env missing the user 'password' secret value.\n.env:\n%s", dotenv)
	}
	if !strings.Contains(dotenv, jumpPwVal) {
		t.Errorf(".env missing the jump-host password value.\n.env:\n%s", dotenv)
	}

	got := bjhsLoadByName(t, collPath, "Staging")
	// User secret variable restored to its own value.
	var userVar *model.Variable
	for i := range got.Variables {
		if got.Variables[i].Key == "password" {
			userVar = &got.Variables[i]
		}
	}
	if userVar == nil {
		t.Fatalf("user 'password' variable missing after load")
	}
	if userVar.Value != userPwVal {
		t.Errorf("user 'password' variable clobbered: got %q want %q", userVar.Value, userPwVal)
	}
	if got.JumpHost == nil || got.JumpHost.Password != jumpPwVal {
		t.Errorf("jump-host password clobbered or lost: got %+v want %q", got.JumpHost, jumpPwVal)
	}
}
