package ui

import (
	"testing"
	"time"
)

// Settings.options() converts the app-level connection Settings into a
// yonner.Options for a send. Per SCOPE/settings.go: Timeout and InsecureTLS
// pass through; FollowRedirects is always true in v1 (no per-request override).

func TestSettingsOptions_PassesThroughAndAlwaysFollowsRedirects(t *testing.T) {
	cases := []Settings{
		{Timeout: 30 * time.Second, InsecureTLS: false},
		{Timeout: 5 * time.Second, InsecureTLS: true},
		{Timeout: 0, InsecureTLS: false},
	}
	for _, s := range cases {
		opts := s.options()
		if opts.Timeout != s.Timeout {
			t.Errorf("Timeout = %v, want %v", opts.Timeout, s.Timeout)
		}
		if opts.InsecureTLS != s.InsecureTLS {
			t.Errorf("InsecureTLS = %v, want %v", opts.InsecureTLS, s.InsecureTLS)
		}
		if !opts.FollowRedirects {
			t.Errorf("FollowRedirects = false, want always true in v1")
		}
	}
}
