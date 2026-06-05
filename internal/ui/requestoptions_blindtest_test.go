package ui

// Blind tests for the per-request Options UI helpers (issue #26, Dev B).
// Written from the contract only: these pin the pure, Fyne-free mappers
//
//	requestOptionsFromControls(timeoutText, followSel, tlsSel string) *model.RequestOptions
//	seedOptionControls(ro *model.RequestOptions) (timeoutText, followSel, tlsSel string)
//
// The Select label strings are Dev B's in-package consts (optDefault, optFollow,
// optNoFollow, optTLSAllow, optTLSVerify). To avoid hardcoding labels we prefer
// to obtain the canonical strings from seedOptionControls and feed them back into
// requestOptionsFromControls (round-trip), asserting on label literals only where
// a specific mapping is the thing under test. Deterministic and -race-clean: the
// helpers are pure, with no clock, goroutines, or shared state.

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// ptrInt / ptrBool build addressable pointers for expected tri-state fields.
func ptrInt(n int) *int    { return &n }
func ptrBool(b bool) *bool { return &b }

// TestRequestOptionsFromControls pins the raw-controls → *RequestOptions mapping,
// field by field, including the all-default → nil backward-compat case.
func TestRequestOptionsFromControls(t *testing.T) {
	// Canonical Default-state label strings, sourced from the inverse helper on a
	// nil RequestOptions so we never hardcode the "Default (global)" literal.
	defTimeout, defFollow, defTLS := seedOptionControls(nil)
	if defTimeout != "" {
		t.Fatalf("seedOptionControls(nil) timeout = %q, want blank", defTimeout)
	}

	tests := []struct {
		name        string
		timeoutText string
		followSel   string
		tlsSel      string
		want        *model.RequestOptions
	}{
		{
			name:        "all default/blank -> nil",
			timeoutText: defTimeout,
			followSel:   defFollow,
			tlsSel:      defTLS,
			want:        nil,
		},
		{
			name:        "only timeout 30",
			timeoutText: "30",
			followSel:   defFollow,
			tlsSel:      defTLS,
			want:        &model.RequestOptions{TimeoutSeconds: ptrInt(30)},
		},
		{
			name:        "follow do-not-follow overrides truthy global",
			timeoutText: defTimeout,
			followSel:   "Don't follow",
			tlsSel:      defTLS,
			want:        &model.RequestOptions{FollowRedirects: ptrBool(false)},
		},
		{
			name:        "follow yes",
			timeoutText: defTimeout,
			followSel:   "Follow",
			tlsSel:      defTLS,
			want:        &model.RequestOptions{FollowRedirects: ptrBool(true)},
		},
		{
			name:        "tls allow insecure",
			timeoutText: defTimeout,
			followSel:   defFollow,
			tlsSel:      "Allow (insecure)",
			want:        &model.RequestOptions{InsecureTLS: ptrBool(true)},
		},
		{
			name:        "tls verify secure",
			timeoutText: defTimeout,
			followSel:   defFollow,
			tlsSel:      "Verify (secure)",
			want:        &model.RequestOptions{InsecureTLS: ptrBool(false)},
		},
		{
			name:        "invalid timeout abc -> nil timeout",
			timeoutText: "abc",
			followSel:   defFollow,
			tlsSel:      defTLS,
			want:        nil,
		},
		{
			name:        "blank timeout -> nil timeout",
			timeoutText: "",
			followSel:   defFollow,
			tlsSel:      defTLS,
			want:        nil,
		},
		{
			name:        "all three set",
			timeoutText: "12",
			followSel:   "Follow",
			tlsSel:      "Verify (secure)",
			want: &model.RequestOptions{
				TimeoutSeconds:  ptrInt(12),
				FollowRedirects: ptrBool(true),
				InsecureTLS:     ptrBool(false),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := requestOptionsFromControls(tc.timeoutText, tc.followSel, tc.tlsSel)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("requestOptionsFromControls(%q,%q,%q) = %s, want %s",
					tc.timeoutText, tc.followSel, tc.tlsSel, fmtRO(got), fmtRO(tc.want))
			}
		})
	}
}

// TestSeedRoundTrip pins that seedOptionControls is the inverse of
// requestOptionsFromControls: feeding the seeded control strings back through the
// forward mapper deep-equals the original (nil round-trips to nil).
func TestSeedRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		ro   *model.RequestOptions
	}{
		{name: "nil (no overrides)", ro: nil},
		{name: "only timeout", ro: &model.RequestOptions{TimeoutSeconds: ptrInt(45)}},
		{name: "timeout zero (explicit no-timeout)", ro: &model.RequestOptions{TimeoutSeconds: ptrInt(0)}},
		{name: "only follow=false", ro: &model.RequestOptions{FollowRedirects: ptrBool(false)}},
		{name: "only follow=true", ro: &model.RequestOptions{FollowRedirects: ptrBool(true)}},
		{name: "only tls=true", ro: &model.RequestOptions{InsecureTLS: ptrBool(true)}},
		{
			name: "all set",
			ro: &model.RequestOptions{
				TimeoutSeconds:  ptrInt(7),
				FollowRedirects: ptrBool(false),
				InsecureTLS:     ptrBool(true),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := requestOptionsFromControls(seedOptionControls(tc.ro))
			if !reflect.DeepEqual(got, tc.ro) {
				t.Fatalf("round-trip mismatch: got %s, want %s", fmtRO(got), fmtRO(tc.ro))
			}
		})
	}
}

// fmtRO renders a *RequestOptions (with its pointer fields dereferenced) for
// readable failure messages.
func fmtRO(ro *model.RequestOptions) string {
	if ro == nil {
		return "nil"
	}
	f := func(b *bool) string {
		if b == nil {
			return "nil"
		}
		return strconv.FormatBool(*b)
	}
	to := "nil"
	if ro.TimeoutSeconds != nil {
		to = strconv.Itoa(*ro.TimeoutSeconds)
	}
	return "{Timeout=" + to + " Follow=" + f(ro.FollowRedirects) + " TLS=" + f(ro.InsecureTLS) + "}"
}
