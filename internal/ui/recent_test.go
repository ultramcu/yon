package ui

import (
	"fmt"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestRecentFiles_DedupOrderCap(t *testing.T) {
	a := New(test.NewApp())
	if got := a.recentFiles(); got != nil {
		t.Fatalf("fresh recent list should be empty, got %v", got)
	}

	a.rememberRecent("/tmp/a.yon")
	a.rememberRecent("/tmp/b.yon")
	a.rememberRecent("/tmp/a.yon") // re-opening a moves it to the front, no dup

	got := a.recentFiles()
	if len(got) != 2 || filepath.Base(got[0]) != "a.yon" || filepath.Base(got[1]) != "b.yon" {
		t.Fatalf("dedup/order wrong: %v", got)
	}

	for i := 0; i < 20; i++ {
		a.rememberRecent(fmt.Sprintf("/tmp/f%02d.yon", i))
	}
	if n := len(a.recentFiles()); n != maxRecentFiles {
		t.Fatalf("recent list not capped: got %d, want %d", n, maxRecentFiles)
	}
}

func TestRecentFiles_RemoveAndClear(t *testing.T) {
	a := New(test.NewApp())
	a.rememberRecent("/tmp/x.yon")
	a.rememberRecent("/tmp/y.yon")

	a.removeRecent("/tmp/x.yon")
	got := a.recentFiles()
	if len(got) != 1 || filepath.Base(got[0]) != "y.yon" {
		t.Fatalf("removeRecent wrong: %v", got)
	}

	a.clearRecent()
	if got := a.recentFiles(); got != nil {
		t.Fatalf("clearRecent should empty the list, got %v", got)
	}
}
