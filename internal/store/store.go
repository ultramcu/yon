// Package store handles reading and writing Yon Collections to disk as `.yon`
// files. A `.yon` file is the on-disk serialization of exactly one
// model.Collection, encoded as indented JSON using only the standard library.
//
// The on-disk format carries a top-level integer schema "version" field so the
// format can be migrated forward without breaking older files. This package
// validates that version on Load and refuses files written by a newer build.
//
// Per the UI-free-core rule this package is part of the UI-free core and must not import
// Fyne or any UI package.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ultramcu/yon/internal/model"
)

// Extension is the file extension (including the leading dot) for Yon
// Collection files.
const Extension = ".yon"

// CurrentVersion is the highest `.yon` schema version this build can read and
// write. Files declaring a higher version are rejected by Load. New Collections
// are saved with this version when Collection.Version is zero.
const CurrentVersion = 1

// EnsureExt returns path unchanged if it already ends in the `.yon` extension
// (case-insensitively), otherwise it returns path with Extension appended. It
// is handy for "Save As" flows where the user may omit the extension.
func EnsureExt(path string) string {
	if strings.EqualFold(filepath.Ext(path), Extension) {
		return path
	}
	return path + Extension
}

// Save writes Collection c to path as indented JSON (2-space indent) for clean,
// stable, git-friendly diffs. If c.Version is zero it is defaulted to
// CurrentVersion before writing. The write is atomic: the data is first written
// to a temporary file in the same directory and then renamed into place, so a
// failed or interrupted write never leaves a partially written `.yon` file at
// path. All returned errors are wrapped with the target path for context.
func Save(path string, c model.Collection) error {
	if c.Version == 0 {
		c.Version = CurrentVersion
	}

	// Collection variables do NOT support secrets (secrets belong to
	// environments and live only in the gitignored .env). Defensively blank the
	// value of any collection variable hand-marked Secret so a secret value can
	// never leak into the committed .yon. Operate on a copy of the Variables
	// slice so the caller's Collection is never mutated; the key and secret flag
	// are preserved, only the value is dropped.
	if len(c.Variables) > 0 {
		vars := make([]model.Variable, len(c.Variables))
		copy(vars, c.Variables)
		for i := range vars {
			if vars[i].Secret {
				vars[i].Value = ""
			}
		}
		c.Variables = vars
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("store: marshal collection for %q: %w", path, err)
	}
	// MarshalIndent omits the trailing newline; add one so the file is a
	// well-formed text file and produces cleaner diffs.
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".yon-*.tmp")
	if err != nil {
		return fmt.Errorf("store: create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail out before the successful rename.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store: write temp file %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: close temp file %q: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("store: rename %q to %q: %w", tmpName, path, err)
	}
	return nil
}

// Load reads and unmarshals the `.yon` file at path into a model.Collection.
//
// The schema version is validated: a version of 0 (an older file or one written
// before versioning) is treated as version 1, while a version greater than
// CurrentVersion is rejected with a clear error so a newer file is never
// silently misread by an older build. The returned Collection's Version is
// normalized to the effective version. All returned errors are wrapped with the
// source path for context.
func Load(path string) (model.Collection, error) {
	var c model.Collection

	data, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("store: read %q: %w", path, err)
	}

	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("store: parse %q: %w", path, err)
	}

	if c.Version <= 0 {
		// Missing (0) or garbage/negative version → treat as the current schema.
		c.Version = CurrentVersion
	}
	if c.Version > CurrentVersion {
		return model.Collection{}, fmt.Errorf(
			"store: unsupported .yon version %d in %q (this build supports up to %d)",
			c.Version, path, CurrentVersion,
		)
	}

	return c, nil
}
