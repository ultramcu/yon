package ui

// Blind tests for Dev A's jump-host plumbing helpers (Phase 2 UI).
//
// These tests are written purely from the contract — no Fyne widgets are
// rendered. They pin two pure helpers:
//
//   resolveActiveJumpHost(env, ok, resolve) (model.JumpHost, bool)
//   jumpHostFromForm(use, host, port, user, auth, keyPath, passphrase,
//                    password string, insecure bool) *model.JumpHost
//
// plus the security-critical invariant that an INCOMPLETE (unresolved
// {{...}}) jump host must NOT report ready-to-dial. See contract notes inline.

import (
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// ---------------------------------------------------------------------------
// A. resolveActiveJumpHost
// ---------------------------------------------------------------------------

// mapResolve builds a func(string) string that expands {{key}} tokens from m,
// leaving unknown references literal (Postman behaviour, mirroring
// model.JumpHost.Resolve's resolver contract). This lets us drive the
// "missing variable stays {{missing}}" case deterministically.
func mapResolve(m map[string]string) func(string) string {
	return func(s string) string {
		out := s
		for k, v := range m {
			out = strings.ReplaceAll(out, "{{"+k+"}}", v)
		}
		return out
	}
}

func TestResolveActiveJumpHost(t *testing.T) {
	t.Run("no active env -> zero,false", func(t *testing.T) {
		env := model.Environment{
			Name:     "Prod",
			JumpHost: &model.JumpHost{Host: "bastion.example.com", User: "ops"},
		}
		// ok=false: even a complete jump host must not be returned when no
		// environment is active.
		got, ok := resolveActiveJumpHost(env, false, mapResolve(nil))
		if ok {
			t.Fatalf("ok=true for inactive env, want false")
		}
		if got != (model.JumpHost{}) {
			t.Fatalf("got %+v, want zero JumpHost when not ok", got)
		}
	})

	t.Run("active env, JumpHost nil -> false", func(t *testing.T) {
		env := model.Environment{Name: "Local"} // JumpHost == nil
		got, ok := resolveActiveJumpHost(env, true, mapResolve(nil))
		if ok {
			t.Fatalf("ok=true with nil JumpHost, want false")
		}
		if got != (model.JumpHost{}) {
			t.Fatalf("got %+v, want zero JumpHost when JumpHost nil", got)
		}
	})

	t.Run("active + complete -> resolved Host and true", func(t *testing.T) {
		env := model.Environment{
			Name: "Staging",
			JumpHost: &model.JumpHost{
				Host: "{{host}}",
				User: "{{user}}",
				Port: 2222,
				Auth: model.JumpAuthKey,
			},
		}
		resolve := mapResolve(map[string]string{
			"host": "bastion.internal",
			"user": "deploy",
		})
		got, ok := resolveActiveJumpHost(env, true, resolve)
		if !ok {
			t.Fatalf("ok=false for a fully-resolvable jump host, want true")
		}
		if got.Host != "bastion.internal" {
			t.Errorf("Host = %q, want %q (substituted)", got.Host, "bastion.internal")
		}
		if got.User != "deploy" {
			t.Errorf("User = %q, want %q (substituted)", got.User, "deploy")
		}
		if got.Port != 2222 {
			t.Errorf("Port = %d, want 2222 (carried through)", got.Port)
		}
		if strings.Contains(got.Host, "{{") {
			t.Errorf("Host %q still holds a template after resolve", got.Host)
		}
	})

	// SECURITY-CRITICAL: a reference that is NOT in the resolve map stays
	// literal "{{missing}}". resolveActiveJumpHost must report false so Yon
	// never dials a literal template. This is the case the contract calls out.
	t.Run("active + incomplete ({{missing}}) -> false (must not connect)", func(t *testing.T) {
		env := model.Environment{
			Name: "Prod",
			JumpHost: &model.JumpHost{
				Host: "{{missing}}", // not provided by the resolver
				User: "ops",
				Auth: model.JumpAuthKey,
			},
		}
		resolve := mapResolve(map[string]string{
			"host": "bastion.internal", // note: key is "host", not "missing"
		})
		got, ok := resolveActiveJumpHost(env, true, resolve)
		if ok {
			t.Fatalf("ok=true for incomplete jump host (Host=%q) — security bug: must be false", got.Host)
		}
	})
}

// ---------------------------------------------------------------------------
// B. jumpHostFromForm
// ---------------------------------------------------------------------------

func TestJumpHostFromForm(t *testing.T) {
	t.Run("use=false -> nil", func(t *testing.T) {
		if jh := jumpHostFromForm(false, "bastion", "22", "ops", model.JumpAuthKey, "/k", "pp", "", false); jh != nil {
			t.Fatalf("got %+v, want nil when use=false", jh)
		}
	})

	t.Run("host empty -> nil", func(t *testing.T) {
		if jh := jumpHostFromForm(true, "", "22", "ops", model.JumpAuthKey, "/k", "pp", "", false); jh != nil {
			t.Fatalf("got %+v, want nil when host empty", jh)
		}
	})

	t.Run("key auth maps fields and parses port", func(t *testing.T) {
		jh := jumpHostFromForm(true, "bastion.example.com", "2222", "deploy",
			model.JumpAuthKey, "/home/u/.ssh/id_ed25519", "secretpp", "", true)
		if jh == nil {
			t.Fatal("got nil, want non-nil JumpHost for use=true with host set")
		}
		if jh.Host != "bastion.example.com" {
			t.Errorf("Host = %q, want %q", jh.Host, "bastion.example.com")
		}
		if jh.User != "deploy" {
			t.Errorf("User = %q, want %q", jh.User, "deploy")
		}
		if jh.Port != 2222 {
			t.Errorf("Port = %d, want 2222 (parsed from \"2222\")", jh.Port)
		}
		if jh.Auth != model.JumpAuthKey {
			t.Errorf("Auth = %q, want %q", jh.Auth, model.JumpAuthKey)
		}
		if jh.KeyPath != "/home/u/.ssh/id_ed25519" {
			t.Errorf("KeyPath = %q, want the key path", jh.KeyPath)
		}
		if jh.Passphrase != "secretpp" {
			t.Errorf("Passphrase = %q, want %q (key-auth secret carried)", jh.Passphrase, "secretpp")
		}
		if !jh.Insecure {
			t.Errorf("Insecure = false, want true (passed through)")
		}
	})

	t.Run("blank port -> 0 (default 22 downstream)", func(t *testing.T) {
		jh := jumpHostFromForm(true, "host", "", "u", model.JumpAuthKey, "/k", "", "", false)
		if jh == nil {
			t.Fatal("got nil, want non-nil")
		}
		if jh.Port != 0 {
			t.Errorf("Port = %d, want 0 for blank port", jh.Port)
		}
	})

	t.Run("non-numeric port -> 0", func(t *testing.T) {
		jh := jumpHostFromForm(true, "host", "notaport", "u", model.JumpAuthKey, "/k", "", "", false)
		if jh == nil {
			t.Fatal("got nil, want non-nil")
		}
		if jh.Port != 0 {
			t.Errorf("Port = %d, want 0 for non-numeric port", jh.Port)
		}
	})

	t.Run("password auth carries Password", func(t *testing.T) {
		jh := jumpHostFromForm(true, "host", "22", "u",
			model.JumpAuthPassword, "", "", "hunter2", false)
		if jh == nil {
			t.Fatal("got nil, want non-nil")
		}
		if jh.Auth != model.JumpAuthPassword {
			t.Errorf("Auth = %q, want %q", jh.Auth, model.JumpAuthPassword)
		}
		if jh.Password != "hunter2" {
			t.Errorf("Password = %q, want %q (password-auth secret carried)", jh.Password, "hunter2")
		}
	})
}

// ---------------------------------------------------------------------------
// C. Dialer-injection invariant (encoded against the pure helper).
//
// Wiring a real *Window + Fyne render to assert opts.DialContext is set is too
// heavy and brittle for a -race-clean unit test, so per the contract this
// invariant is encoded against resolveActiveJumpHost, which is the exact gate
// the send path consults before it installs a dialer:
//
//   - active + COMPLETE jump host  -> ok==true  -> send sets opts.DialContext
//   - no / INCOMPLETE jump host     -> ok==false -> opts.DialContext stays nil
//
// The full DialContext wiring is covered by the verifier's manual/smoke check.
// ---------------------------------------------------------------------------

func TestDialerInjectionGate(t *testing.T) {
	complete := model.Environment{
		Name:     "Live",
		JumpHost: &model.JumpHost{Host: "bastion", User: "ops", Auth: model.JumpAuthKey},
	}
	incomplete := model.Environment{
		Name:     "Live",
		JumpHost: &model.JumpHost{Host: "{{unset}}", User: "ops", Auth: model.JumpAuthKey},
	}

	resolve := mapResolve(map[string]string{}) // resolves nothing

	// Complete jump host -> gate open: the send path WILL inject a dialer.
	if _, ok := resolveActiveJumpHost(complete, true, resolve); !ok {
		t.Errorf("gate closed for a complete jump host; send would leave DialContext nil")
	}
	// No active env -> gate closed: DialContext must stay nil.
	if _, ok := resolveActiveJumpHost(complete, false, resolve); ok {
		t.Errorf("gate open with no active env; send would inject a dialer it must not")
	}
	// Incomplete jump host -> gate closed: DialContext must stay nil.
	if _, ok := resolveActiveJumpHost(incomplete, true, resolve); ok {
		t.Errorf("gate open for incomplete jump host; send would dial a literal template")
	}
}
