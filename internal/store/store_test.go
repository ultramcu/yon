package store_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// sampleCollection builds a non-trivial Collection exercising every meaningful
// field: collection-level auth, multiple requests, params/headers (enabled and
// disabled), per-request auth of each kind, and bodies of each type.
func sampleCollection() model.Collection {
	return model.Collection{
		Version: 1,
		Name:    "Round Trip",
		Auth: model.Auth{
			Kind:  model.AuthBearer,
			Token: "collection-default-token",
		},
		Requests: []model.Request{
			{
				Name:   "List users",
				Method: model.MethodGet,
				URL:    "https://api.example.com/users",
				Params: []model.Param{
					{Key: "page", Value: "1", Enabled: true},
					{Key: "limit", Value: "50", Enabled: false},
				},
				Headers: []model.Param{
					{Key: "Accept", Value: "application/json", Enabled: true},
				},
				Auth: model.Auth{Kind: model.AuthInherit},
				Body: model.Body{Type: model.BodyNone},
			},
			{
				Name:   "Create user",
				Method: model.MethodPost,
				URL:    "https://api.example.com/users",
				Auth: model.Auth{
					Kind:     model.AuthBasic,
					Username: "admin",
					Password: "s3cret",
				},
				Body: model.Body{
					Type:    model.BodyJSON,
					Content: "{\n  \"name\": \"Ada\"\n}",
				},
			},
			{
				Name:   "Delete user",
				Method: model.MethodDelete,
				URL:    "https://api.example.com/users/7",
				Auth:   model.Auth{Kind: model.AuthNone},
				Body:   model.Body{Type: model.BodyText, Content: "bye"},
			},
		},
	}
}

// TestSaveLoadRoundTrip writes a non-trivial Collection and reads it back,
// asserting deep equality on the meaningful fields.
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rt.yon")

	want := sampleCollection()
	if err := store.Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\n got = %#v\nwant = %#v", got, want)
	}

	// Spot-check a few individual fields so a regression is easy to read even
	// if DeepEqual were ever loosened.
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Auth.Token != want.Auth.Token {
		t.Errorf("Auth.Token = %q, want %q", got.Auth.Token, want.Auth.Token)
	}
	if len(got.Requests) != len(want.Requests) {
		t.Fatalf("len(Requests) = %d, want %d", len(got.Requests), len(want.Requests))
	}
	if got.Requests[0].Params[1].Enabled != false {
		t.Errorf("disabled param round-trip lost: Enabled = true, want false")
	}
	if got.Requests[1].Body.Content != want.Requests[1].Body.Content {
		t.Errorf("Body.Content = %q, want %q",
			got.Requests[1].Body.Content, want.Requests[1].Body.Content)
	}
}

// TestSaveDefaultsVersionWhenZero asserts Save writes version 1 when the
// in-memory Collection left Version at its zero value.
func TestSaveDefaultsVersionWhenZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zero.yon")

	c := model.Collection{Name: "no version"} // Version left at 0
	if err := store.Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Read it back: Load should yield a usable version (1), and the on-disk
	// file should record version 1 rather than 0.
	got, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != store.CurrentVersion {
		t.Errorf("Load.Version = %d, want CurrentVersion %d", got.Version, store.CurrentVersion)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), `"version": 0`) {
		t.Errorf("Save wrote version 0 to disk; want it defaulted to 1:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"version": 1`) {
		t.Errorf("Save did not write version 1 to disk:\n%s", raw)
	}
}

// TestLoadTreatsVersionZeroAsOne hand-writes a file whose version is 0 and
// asserts Load accepts it and normalizes the version to 1.
func TestLoadTreatsVersionZeroAsOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v0.yon")

	const data = `{
  "version": 0,
  "name": "legacy",
  "auth": { "kind": "none" }
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load of version-0 file should succeed, got error: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Load.Version = %d, want 1 (version 0 treated as 1)", got.Version)
	}
	if got.Name != "legacy" {
		t.Errorf("Load.Name = %q, want %q", got.Name, "legacy")
	}
}

// TestLoadRejectsNewerVersion hand-writes a file with a version greater than
// CurrentVersion and asserts Load returns a clear, version-mentioning error.
func TestLoadRejectsNewerVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.yon")

	const data = `{
  "version": 999,
  "name": "from the future",
  "auth": { "kind": "none" }
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := store.Load(path)
	if err == nil {
		t.Fatal("Load of version-999 file should error, got nil")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "version") {
		t.Errorf("error should mention the version; got %q", err.Error())
	}
	// The error should also surface the offending number so the user can act.
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("error should mention the offending version 999; got %q", err.Error())
	}
}

// TestEnsureExt covers the extension-normalization helper.
func TestEnsureExt(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"appends when missing", "mycoll", "mycoll" + store.Extension},
		{"leaves existing lowercase", "mycoll.yon", "mycoll.yon"},
		{"appends to other extension", "data.json", "data.json" + store.Extension},
		{"path with dir, no ext", filepath.Join("a", "b", "coll"), filepath.Join("a", "b", "coll") + store.Extension},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := store.EnsureExt(tc.in); got != tc.want {
				t.Errorf("EnsureExt(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestEnsureExtCaseInsensitive checks that an existing extension differing only
// in case is treated as already-present (not double-appended). The spec leaves
// case sensitivity open ("case-insensitively if spec'd"); we assert only that
// the helper does not produce a doubled ".yon.yon", which would be wrong under
// either interpretation.
func TestEnsureExtCaseInsensitive(t *testing.T) {
	got := store.EnsureExt("MyColl.YON")
	if strings.Count(strings.ToLower(got), ".yon") > 1 {
		t.Errorf("EnsureExt(%q) = %q double-appended the extension", "MyColl.YON", got)
	}
}

// TestSaveWritesIndentedJSON asserts the saved file is valid, indented JSON.
func TestSaveWritesIndentedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "indent.yon")

	if err := store.Save(path, sampleCollection()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(raw)

	if !strings.Contains(text, "\n") {
		t.Error("saved file has no newlines; expected indented JSON")
	}
	// encoding/json's MarshalIndent with a 2-space indent nests "name" under
	// the top-level object, so a two-space-indented "name" must appear.
	if !strings.Contains(text, "\n  \"name\":") {
		t.Errorf("saved file is not 2-space indented; got:\n%s", text)
	}
	// Validate it parses as JSON via a successful Load round-trip.
	if _, err := store.Load(path); err != nil {
		t.Errorf("saved file did not parse back via Load: %v", err)
	}
}

// TestLoadMissingFileErrors asserts a clear error (not a panic) for a path that
// does not exist.
func TestLoadMissingFileErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := store.Load(filepath.Join(dir, "nope.yon"))
	if err == nil {
		t.Fatal("Load of a missing file should error, got nil")
	}
}

// TestLoadSampleFixture loads the checked-in fixture documenting the .yon
// format and asserts it parses with the expected top-level shape. It does not
// depend on exact contents beyond what SCOPE implies.
func TestLoadSampleFixture(t *testing.T) {
	path := filepath.Join("testdata", "sample.yon")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no sample fixture: %v", err)
	}

	got, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load(sample.yon): %v", err)
	}
	if got.Version < 1 || got.Version > store.CurrentVersion {
		t.Errorf("fixture Version = %d, want in [1, %d]", got.Version, store.CurrentVersion)
	}
	if len(got.Requests) == 0 {
		t.Error("fixture should contain at least one request")
	}
}

// TestSaveDoesNotClobberOnReSave is a best-effort check that re-saving over an
// existing file leaves a valid, loadable file (i.e. Save does not leave a
// truncated/partial file behind on the final path).
func TestSaveDoesNotClobberOnReSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resave.yon")

	if err := store.Save(path, sampleCollection()); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	// Overwrite with a different, smaller Collection.
	small := model.Collection{Version: 1, Name: "small", Auth: model.Auth{Kind: model.AuthNone}}
	if err := store.Save(path, small); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load after re-save: %v", err)
	}
	if got.Name != "small" || len(got.Requests) != 0 {
		t.Errorf("re-save left stale content: got %#v", got)
	}
}

// TestSave_BlanksSecretCollectionVariable is the regression test for DEFECT A:
// a collection-level variable hand-marked Secret must never have its plaintext
// value written into the committed .yon, and Save must not mutate the caller's
// Collection.
//
// Without the fix Save marshals the Collection directly, so "LEAKVALUE" lands in
// the committed file. With the fix Save operates on a copy and blanks the value
// of any Secret collection variable before marshaling.
func TestSave_BlanksSecretCollectionVariable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "leak.yon")

	coll := model.Collection{
		Version: 1,
		Name:    "Leak",
		Variables: []model.Variable{
			{Key: "tok", Value: "LEAKVALUE", Enabled: true, Secret: true},
		},
	}

	if err := store.Save(path, coll); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written .yon: %v", err)
	}
	if strings.Contains(string(data), "LEAKVALUE") {
		t.Fatalf("committed .yon leaked secret collection-variable value:\n%s", data)
	}
	// The key and secret flag should still be recorded (only the value drops).
	if !strings.Contains(string(data), `"tok"`) {
		t.Errorf("committed .yon dropped the secret variable key entirely:\n%s", data)
	}

	// Save must NOT mutate the caller's Collection.
	if got := coll.Variables[0].Value; got != "LEAKVALUE" {
		t.Errorf("Save mutated caller's collection variable value = %q, want %q", got, "LEAKVALUE")
	}
}
