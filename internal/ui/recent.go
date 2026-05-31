package ui

import (
	"encoding/json"
	"path/filepath"
)

// Recent-files list: the most-recently opened/saved .yon paths, persisted in
// Preferences as a JSON array (newest first) and surfaced as File → Open Recent.
const (
	prefKeyRecentFiles = "recentFiles"
	maxRecentFiles     = 10
)

// recentFiles returns the remembered .yon paths, newest first (nil if none).
func (a *App) recentFiles() []string {
	raw := a.prefs().String(prefKeyRecentFiles)
	if raw == "" {
		return nil
	}
	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return nil
	}
	return paths
}

// rememberRecent records path as the most-recent .yon file: absolutised,
// de-duplicated, moved to the front, and capped at maxRecentFiles.
func (a *App) rememberRecent(path string) {
	if path == "" {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	out := []string{path}
	for _, p := range a.recentFiles() {
		if p == path {
			continue
		}
		out = append(out, p)
		if len(out) >= maxRecentFiles {
			break
		}
	}
	a.saveRecent(out)
}

// removeRecent drops path from the recent list (e.g. when it no longer exists).
func (a *App) removeRecent(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	var out []string
	for _, p := range a.recentFiles() {
		if p != path {
			out = append(out, p)
		}
	}
	a.saveRecent(out)
}

// clearRecent empties the recent-files list.
func (a *App) clearRecent() { a.saveRecent(nil) }

func (a *App) saveRecent(paths []string) {
	b, err := json.Marshal(paths)
	if err != nil {
		return
	}
	a.prefs().SetString(prefKeyRecentFiles, string(b))
}
