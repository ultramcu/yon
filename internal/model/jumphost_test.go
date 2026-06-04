package model_test

import (
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// resolver builds a func(string) string that expands the given {{key}} -> value
// pairs and leaves any other {{...}} reference literal (matching the real
// variables.Scope.Resolve behaviour).
func resolver(pairs map[string]string) func(string) string {
	return func(s string) string {
		for k, v := range pairs {
			s = strings.ReplaceAll(s, "{{"+k+"}}", v)
		}
		return s
	}
}

// TestJumpHostResolve_Templated resolves every text field via the supplied
// resolver and reports the config complete.
func TestJumpHostResolve_Templated(t *testing.T) {
	jh := model.JumpHost{
		Host:       "{{bastionHost}}",
		Port:       2222,
		User:       "{{bastionUser}}",
		Auth:       model.JumpAuthKey,
		KeyPath:    "{{home}}/id_ed25519",
		Insecure:   true,
		Passphrase: "{{pp}}",
	}
	res, complete := jh.Resolve(resolver(map[string]string{
		"bastionHost": "jump.example.com",
		"bastionUser": "deploy",
		"home":        "/home/me",
		"pp":          "s3cret",
	}))
	if !complete {
		t.Fatalf("complete = false; want true (all fields resolved)")
	}
	if res.Host != "jump.example.com" || res.User != "deploy" ||
		res.KeyPath != "/home/me/id_ed25519" || res.Passphrase != "s3cret" {
		t.Fatalf("unexpected resolved fields: %+v", res)
	}
	// Non-templated fields copied verbatim.
	if res.Port != 2222 || res.Auth != model.JumpAuthKey || !res.Insecure {
		t.Fatalf("non-templated fields not preserved: %+v", res)
	}
}

// TestJumpHostResolve_Incomplete reports an unresolved {{x}} as incomplete and
// does NOT silently pass a literal template as if it were a real value.
func TestJumpHostResolve_Incomplete(t *testing.T) {
	jh := model.JumpHost{
		Host: "{{missingHost}}",
		User: "deploy",
		Auth: model.JumpAuthPassword,
	}
	res, complete := jh.Resolve(resolver(map[string]string{"other": "x"}))
	if complete {
		t.Fatalf("complete = true; want false (Host still {{missingHost}})")
	}
	if !strings.Contains(res.Host, "{{") {
		t.Fatalf("expected unresolved template left literal in Host, got %q", res.Host)
	}
}

// TestJumpHostResolve_LiteralIsComplete: a fully-literal config resolves to
// itself and is complete, even with a nil (identity) resolver.
func TestJumpHostResolve_LiteralIsComplete(t *testing.T) {
	jh := model.JumpHost{
		Host:     "10.0.0.1",
		Port:     22,
		User:     "admin",
		Auth:     model.JumpAuthPassword,
		Password: "hunter2",
	}
	res, complete := jh.Resolve(nil)
	if !complete {
		t.Fatalf("complete = false; want true for literal config")
	}
	if res != jh {
		t.Fatalf("literal config did not resolve to itself: %+v vs %+v", res, jh)
	}
}
