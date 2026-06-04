package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/store"
)

// SPEC 1: Store round-trip — a Collection with Folders + requests carrying
// FolderID survives Save->Load (folders, names, collapsed, each request's
// FolderID preserved).
func TestBlind_StoreRoundTrip_FoldersAndFolderID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yon")

	want := model.NewCollection("Coll")
	want.Folders = []model.Folder{
		{ID: "fAAAA", Name: "Alpha", Collapsed: false},
		{ID: "fBBBB", Name: "Beta", Collapsed: true},
	}
	want.Requests = []model.Request{
		{Name: "r0", Method: model.MethodGet, URL: "http://x/0", FolderID: "fAAAA"},
		{Name: "r1", Method: model.MethodPost, URL: "http://x/1", FolderID: "fBBBB"},
		{Name: "r2", Method: model.MethodPut, URL: "http://x/2", FolderID: ""},
	}

	if err := store.Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Folders) != len(want.Folders) {
		t.Fatalf("folder count: got %d want %d", len(got.Folders), len(want.Folders))
	}
	for i, wf := range want.Folders {
		gf := got.Folders[i]
		if gf.ID != wf.ID || gf.Name != wf.Name || gf.Collapsed != wf.Collapsed {
			t.Fatalf("folder[%d]: got %+v want %+v", i, gf, wf)
		}
	}
	if len(got.Requests) != len(want.Requests) {
		t.Fatalf("request count: got %d want %d", len(got.Requests), len(want.Requests))
	}
	for i, wr := range want.Requests {
		if got.Requests[i].FolderID != wr.FolderID {
			t.Fatalf("request[%d] FolderID: got %q want %q", i, got.Requests[i].FolderID, wr.FolderID)
		}
		if got.Requests[i].Name != wr.Name {
			t.Fatalf("request[%d] Name: got %q want %q", i, got.Requests[i].Name, wr.Name)
		}
	}
}

// SPEC 2a: Backward compat — an OLD .yon written WITHOUT folders/folderId loads
// with no folders and every request top-level (FolderID == "").
func TestBlind_BackwardCompat_OldFileNoFolders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.yon")

	old := `{
  "version": 1,
  "name": "Legacy",
  "auth": {"kind": "none"},
  "requests": [
    {"name": "a", "method": "GET", "url": "http://x/a", "auth": {"kind": "inherit"}, "body": {"type": "none"}},
    {"name": "b", "method": "POST", "url": "http://x/b", "auth": {"kind": "inherit"}, "body": {"type": "none"}}
  ]
}
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatalf("write old file: %v", err)
	}

	got, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load old file: %v", err)
	}
	if len(got.Folders) != 0 {
		t.Fatalf("expected no folders in old file, got %d: %+v", len(got.Folders), got.Folders)
	}
	if len(got.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(got.Requests))
	}
	for i, r := range got.Requests {
		if r.FolderID != "" {
			t.Fatalf("request[%d] FolderID: got %q want \"\" (top-level)", i, r.FolderID)
		}
	}
}

// SPEC 2b: omitempty — a Collection with NO folders serializes WITHOUT a
// "folders" key, and requests with no folder omit "folderId". Inspect on-disk
// JSON.
func TestBlind_OmitEmpty_NoFoldersKeyOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nofolders.yon")

	c := model.NewCollection("Plain")
	c.Requests = []model.Request{
		{Name: "r", Method: model.MethodGet, URL: "http://x/r"},
	}
	if err := store.Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	disk := string(raw)
	if strings.Contains(disk, "\"folders\"") {
		t.Fatalf("on-disk JSON unexpectedly contains \"folders\" key:\n%s", disk)
	}
	if strings.Contains(disk, "\"folderId\"") {
		t.Fatalf("on-disk JSON unexpectedly contains \"folderId\" key:\n%s", disk)
	}
}
