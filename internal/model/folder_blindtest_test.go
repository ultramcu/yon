package model

import (
	"regexp"
	"testing"
)

// SPEC 3: NewFolderID — unique across many calls, non-empty, stable format.

var bt3FolderIDFormat = regexp.MustCompile(`^f[0-9a-f]{8}$`)

func TestBlind_NewFolderID_UniqueNonEmptyStable(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := NewFolderID()
		if id == "" {
			t.Fatalf("NewFolderID returned empty string on call %d", i)
		}
		if !bt3FolderIDFormat.MatchString(id) {
			t.Fatalf("NewFolderID %q does not match stable format /^f[0-9a-f]{8}$/", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("NewFolderID produced a duplicate id %q after %d calls", id, i)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique ids, got %d", n, len(seen))
	}
}
