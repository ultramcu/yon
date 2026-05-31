package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_NonPositiveVersionNormalized guards that a garbage/negative version
// is treated as the current schema (not silently round-tripped as invalid).
func TestLoad_NonPositiveVersionNormalized(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yon")
	if err := os.WriteFile(p, []byte(`{"version":-5,"name":"X"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("negative version should load (normalized), got %v", err)
	}
	if c.Version != CurrentVersion {
		t.Fatalf("version = %d, want %d", c.Version, CurrentVersion)
	}
}
