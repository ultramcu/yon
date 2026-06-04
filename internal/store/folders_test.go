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

// A Collection with Folders and requests carrying FolderID round-trips through
// Save/Load unchanged.
func TestSaveLoadRoundTrip_Folders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "folders.yon")

	want := model.Collection{
		Version: 1,
		Name:    "Grouped",
		Auth:    model.Auth{Kind: model.AuthNone},
		Folders: []model.Folder{
			{ID: "f0a1b2c3", Name: "Auth", Collapsed: true},
			{ID: "fdeadbee", Name: "Users"},
		},
		Requests: []model.Request{
			{Method: model.MethodGet, Name: "login", Auth: model.Auth{Kind: model.AuthInherit}, Body: model.Body{Type: model.BodyNone}, FolderID: "f0a1b2c3"},
			{Method: model.MethodGet, Name: "list", Auth: model.Auth{Kind: model.AuthInherit}, Body: model.Body{Type: model.BodyNone}, FolderID: "fdeadbee"},
			{Method: model.MethodGet, Name: "ping", Auth: model.Auth{Kind: model.AuthInherit}, Body: model.Body{Type: model.BodyNone}}, // top-level
		},
	}

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
}

// An OLD-format .yon with no "folders" and no "folderId" loads with no folders
// and all requests top-level (backward compatibility).
func TestLoad_OldFormatHasNoFolders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.yon")

	const data = `{
  "version": 1,
  "name": "legacy",
  "auth": { "kind": "none" },
  "requests": [
    { "method": "GET", "url": "https://example.com", "auth": { "kind": "inherit" }, "body": { "type": "none" } }
  ]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Folders != nil {
		t.Errorf("Folders = %#v, want nil for an old-format file", got.Folders)
	}
	if len(got.Requests) != 1 {
		t.Fatalf("len(Requests) = %d, want 1", len(got.Requests))
	}
	if got.Requests[0].FolderID != "" {
		t.Errorf("FolderID = %q, want \"\" (top-level) for an old-format request", got.Requests[0].FolderID)
	}
}

// New folder fields are omitted from the on-disk file when empty, so a
// Collection with no folders stays byte-identical to the pre-folders format.
func TestSave_OmitsEmptyFolderFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yon")

	c := model.Collection{
		Version: 1,
		Name:    "Ungrouped",
		Auth:    model.Auth{Kind: model.AuthNone},
		Requests: []model.Request{
			{Method: model.MethodGet, URL: "https://x", Auth: model.Auth{Kind: model.AuthInherit}, Body: model.Body{Type: model.BodyNone}},
		},
	}
	if err := store.Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "folders") {
		t.Errorf("on-disk file mentions \"folders\" though there are none:\n%s", raw)
	}
	if strings.Contains(string(raw), "folderId") {
		t.Errorf("on-disk file mentions \"folderId\" though no request is grouped:\n%s", raw)
	}
}
