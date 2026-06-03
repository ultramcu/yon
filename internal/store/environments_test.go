package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// localEnv is the canonical fixture used across these tests: one non-secret
// variable (baseUrl) and one secret variable (token). The secret value
// "s3cret" must never appear in the committed env JSON, only in the .env.
func localEnv() model.Environment {
	return model.Environment{
		Name: "Local",
		Variables: []model.Variable{
			{Key: "baseUrl", Value: "http://localhost", Enabled: true},
			{Key: "token", Value: "s3cret", Enabled: true, Secret: true},
		},
	}
}

func findEnv(envs []model.Environment, name string) (model.Environment, bool) {
	for _, e := range envs {
		if e.Name == name {
			return e, true
		}
	}
	return model.Environment{}, false
}

func findVar(env model.Environment, key string) (model.Variable, bool) {
	for _, v := range env.Variables {
		if v.Key == key {
			return v, true
		}
	}
	return model.Variable{}, false
}

// readFile is a small helper that fails the test if the file can't be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// 1. Round-trip: a saved env (incl. a secret) loads back with the secret value
// restored from the .env file.
func TestSaveLoadEnvironment_RoundTripRestoresSecret(t *testing.T) {
	dir := t.TempDir()
	collPath := filepath.Join(dir, "api.yon")

	if err := store.SaveEnvironment(collPath, localEnv()); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	envs, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}

	env, ok := findEnv(envs, "Local")
	if !ok {
		t.Fatalf("expected an environment named %q, got %d envs: %+v", "Local", len(envs), envs)
	}

	base, ok := findVar(env, "baseUrl")
	if !ok {
		t.Fatalf("missing variable baseUrl in loaded env: %+v", env)
	}
	if base.Value != "http://localhost" {
		t.Errorf("baseUrl value = %q, want %q", base.Value, "http://localhost")
	}

	tok, ok := findVar(env, "token")
	if !ok {
		t.Fatalf("missing variable token in loaded env: %+v", env)
	}
	if tok.Value != "s3cret" {
		t.Errorf("secret token value not restored: got %q, want %q", tok.Value, "s3cret")
	}
}

// 2. The committed env JSON must NOT contain the secret value, but must contain
// the non-secret value and the secret key (so the variable is still declared).
func TestSaveEnvironment_SecretBlankedInCommittedJSON(t *testing.T) {
	dir := t.TempDir()
	collPath := filepath.Join(dir, "api.yon")

	if err := store.SaveEnvironment(collPath, localEnv()); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	envDir := store.EnvironmentsDir(collPath)
	entries, err := os.ReadDir(envDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", envDir, err)
	}

	// Find the env JSON file(s) and aggregate their contents.
	var jsonFound bool
	var combined string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		jsonFound = true
		combined += readFile(t, filepath.Join(envDir, e.Name()))
	}
	if !jsonFound {
		t.Fatalf("no environment file written under %s", envDir)
	}

	if strings.Contains(combined, "s3cret") {
		t.Errorf("committed env JSON leaks secret value %q:\n%s", "s3cret", combined)
	}
	if !strings.Contains(combined, "http://localhost") {
		t.Errorf("committed env JSON missing non-secret value %q:\n%s", "http://localhost", combined)
	}
	if !strings.Contains(combined, "token") {
		t.Errorf("committed env JSON missing secret key %q:\n%s", "token", combined)
	}
}

// 3. The gitignored .env in the collection dir must carry the secret value.
func TestSaveEnvironment_SecretWrittenToDotEnv(t *testing.T) {
	dir := t.TempDir()
	collPath := filepath.Join(dir, "api.yon")

	if err := store.SaveEnvironment(collPath, localEnv()); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	envFile := filepath.Join(dir, ".env")
	content := readFile(t, envFile)

	if !strings.Contains(content, "s3cret") {
		t.Errorf(".env does not contain secret value %q:\n%s", "s3cret", content)
	}
	// The .env should be key=value style referencing the token.
	if !strings.Contains(content, "token") {
		t.Errorf(".env does not reference the secret key %q:\n%s", "token", content)
	}
}

// 4. The collection dir's .gitignore must ignore .env, and a second save must
// not duplicate the ignore line.
func TestSaveEnvironment_GitignoreIgnoresDotEnvNoDuplicate(t *testing.T) {
	dir := t.TempDir()
	collPath := filepath.Join(dir, "api.yon")

	if err := store.SaveEnvironment(collPath, localEnv()); err != nil {
		t.Fatalf("SaveEnvironment (first): %v", err)
	}

	giPath := filepath.Join(dir, ".gitignore")
	first := readFile(t, giPath)

	countEnvLines := func(s string) int {
		n := 0
		for _, line := range strings.Split(s, "\n") {
			if strings.TrimSpace(line) == ".env" {
				n++
			}
		}
		return n
	}

	if countEnvLines(first) != 1 {
		t.Fatalf(".gitignore should contain exactly one %q line, got %d:\n%s", ".env", countEnvLines(first), first)
	}

	// Save again; the ignore line must not be duplicated.
	if err := store.SaveEnvironment(collPath, localEnv()); err != nil {
		t.Fatalf("SaveEnvironment (second): %v", err)
	}
	second := readFile(t, giPath)
	if countEnvLines(second) != 1 {
		t.Errorf("second save duplicated the %q gitignore line, got %d:\n%s", ".env", countEnvLines(second), second)
	}
}

// 5. EnvironmentsDir maps ".../api.yon" to a sibling ".../api.environments".
func TestEnvironmentsDir_SiblingName(t *testing.T) {
	dir := t.TempDir()
	collPath := filepath.Join(dir, "api.yon")

	got := store.EnvironmentsDir(collPath)
	if base := filepath.Base(got); base != "api.environments" {
		t.Errorf("EnvironmentsDir base = %q, want %q (full: %q)", base, "api.environments", got)
	}
	// And it should be a sibling of the collection file (same parent dir).
	if filepath.Dir(got) != filepath.Dir(collPath) {
		t.Errorf("EnvironmentsDir parent = %q, want sibling of %q", filepath.Dir(got), collPath)
	}
}

// 6. Delete removes the environment so a subsequent Load no longer returns it.
func TestDeleteEnvironment_RemovesIt(t *testing.T) {
	dir := t.TempDir()
	collPath := filepath.Join(dir, "api.yon")

	if err := store.SaveEnvironment(collPath, localEnv()); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	// Sanity: it exists before delete.
	before, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments (before): %v", err)
	}
	if _, ok := findEnv(before, "Local"); !ok {
		t.Fatalf("precondition failed: %q not present before delete", "Local")
	}

	if err := store.DeleteEnvironment(collPath, "Local"); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}

	after, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments (after): %v", err)
	}
	if _, ok := findEnv(after, "Local"); ok {
		t.Errorf("environment %q still present after delete: %+v", "Local", after)
	}
}

// 7. Unsaved/empty collPath: Load is a no-op (nil,nil); Save/Delete error.
func TestEnvironments_EmptyCollPath(t *testing.T) {
	envs, err := store.LoadEnvironments("")
	if err != nil {
		t.Errorf("LoadEnvironments(\"\") err = %v, want nil", err)
	}
	if envs != nil {
		t.Errorf("LoadEnvironments(\"\") = %+v, want nil slice", envs)
	}

	if err := store.SaveEnvironment("", localEnv()); err == nil {
		t.Errorf("SaveEnvironment(\"\", env) err = nil, want non-nil")
	}

	if err := store.DeleteEnvironment("", "Local"); err == nil {
		t.Errorf("DeleteEnvironment(\"\", ...) err = nil, want non-nil")
	}
}

// 8. A fresh collPath with nothing saved yet loads to an empty, error-free set.
func TestLoadEnvironments_NothingSaved(t *testing.T) {
	dir := t.TempDir()
	collPath := filepath.Join(dir, "api.yon")

	envs, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments on fresh collPath: %v", err)
	}
	if len(envs) != 0 {
		t.Errorf("LoadEnvironments on fresh collPath = %d envs, want 0: %+v", len(envs), envs)
	}
}

// TestSaveLoadEnvironment_SecretRoundTripWithEscapes is the regression test for
// DEFECT B: a secret value containing characters the dotenv writer escapes
// (newline, '"', '\', plus '=' and '#') must round-trip byte-identically
// through SaveEnvironment -> LoadEnvironments.
//
// Without the fix parseDotenv strips the surrounding quotes but does not reverse
// the writer's escaping, so the reloaded value differs from the original
// ("\n" survives as a backslash-n, '\"' as backslash-quote, etc.). With the fix
// parseDotenv unescapes double-quoted values exactly.
func TestSaveLoadEnvironment_SecretRoundTripWithEscapes(t *testing.T) {
	dir := t.TempDir()
	collPath := filepath.Join(dir, "api.yon")

	const want = "line1\nline2 \"q\" back\\slash =eq# hash"
	env := model.Environment{
		Name: "Local",
		Variables: []model.Variable{
			{Key: "token", Value: want, Enabled: true, Secret: true},
		},
	}

	if err := store.SaveEnvironment(collPath, env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	envs, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}
	got, ok := findEnv(envs, "Local")
	if !ok {
		t.Fatalf("LoadEnvironments did not return the Local environment: %+v", envs)
	}
	v, ok := findVar(got, "token")
	if !ok {
		t.Fatalf("Local environment missing the token variable: %+v", got)
	}
	if v.Value != want {
		t.Errorf("secret round-trip corrupted value:\n got  %q\n want %q", v.Value, want)
	}
}

// TestSaveLoadEnvironment_DistinctNamesNoFilenameCollision is the regression
// test for DEFECT C: two environment names that sanitise to the same base
// filename ("a/b" and "a:b" both -> "a_b") must not collide on disk, so both
// survive a save/load round-trip with their distinct variables intact.
//
// Without the fix the second SaveEnvironment overwrites the first's file and
// LoadEnvironments returns only one environment. With the fix the filename
// carries a hash of the original name, keeping the two files distinct.
func TestSaveLoadEnvironment_DistinctNamesNoFilenameCollision(t *testing.T) {
	dir := t.TempDir()
	collPath := filepath.Join(dir, "api.yon")

	envAB := model.Environment{
		Name:      "a/b",
		Variables: []model.Variable{{Key: "which", Value: "slash", Enabled: true}},
	}
	envAColonB := model.Environment{
		Name:      "a:b",
		Variables: []model.Variable{{Key: "which", Value: "colon", Enabled: true}},
	}

	if err := store.SaveEnvironment(collPath, envAB); err != nil {
		t.Fatalf("SaveEnvironment a/b: %v", err)
	}
	if err := store.SaveEnvironment(collPath, envAColonB); err != nil {
		t.Fatalf("SaveEnvironment a:b: %v", err)
	}

	envs, err := store.LoadEnvironments(collPath)
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("LoadEnvironments returned %d envs, want 2 (filename collision?): %+v", len(envs), envs)
	}

	slash, ok := findEnv(envs, "a/b")
	if !ok {
		t.Fatalf("missing env a/b: %+v", envs)
	}
	colon, ok := findEnv(envs, "a:b")
	if !ok {
		t.Fatalf("missing env a:b: %+v", envs)
	}
	if v, _ := findVar(slash, "which"); v.Value != "slash" {
		t.Errorf("env a/b var = %q, want %q", v.Value, "slash")
	}
	if v, _ := findVar(colon, "which"); v.Value != "colon" {
		t.Errorf("env a:b var = %q, want %q", v.Value, "colon")
	}
}
