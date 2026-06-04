package model

import (
	"regexp"
	"testing"
)

// NewFolderID returns "f" + 8 hex chars and is unique across many calls.
func TestNewFolderID_ShapeAndUniqueness(t *testing.T) {
	re := regexp.MustCompile(`^f[0-9a-f]{8}$`)
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewFolderID()
		if !re.MatchString(id) {
			t.Fatalf("id %q does not match ^f[0-9a-f]{8}$", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q on iteration %d", id, i)
		}
		seen[id] = true
	}
}
