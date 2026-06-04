package model_test

import (
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// Blind tests for SPEC point 5 — JumpHost.Resolve. Written independently from
// the spec; helper names are prefixed bjh_ to avoid collisions.

// bjhMapResolver builds a resolver that expands "{{key}}" -> mapping[key], and
// leaves unknown "{{x}}" literal (Postman behaviour), so an absent key models an
// unresolved template.
func bjhMapResolver(mapping map[string]string) func(string) string {
	return func(s string) string {
		for k, v := range mapping {
			s = strings.ReplaceAll(s, "{{"+k+"}}", v)
		}
		return s
	}
}

// SPEC 5a: every text field with a {{var}} resolves and complete == true; the
// non-templated Port/Auth/Insecure are copied verbatim.
func TestBlind_JumpHostResolve_AllFieldsResolveComplete(t *testing.T) {
	in := model.JumpHost{
		Host:       "{{bastionHost}}",
		Port:       2222,
		User:       "{{bastionUser}}",
		Auth:       model.JumpAuthKey,
		KeyPath:    "{{home}}/id_ed25519",
		Insecure:   true,
		Passphrase: "{{pp}}",
		Password:   "{{pw}}",
	}
	resolver := bjhMapResolver(map[string]string{
		"bastionHost": "bastion.example.com",
		"bastionUser": "deploy",
		"home":        "/home/deploy",
		"pp":          "s3cretphrase",
		"pw":          "s3cretpass",
	})

	out, complete := in.Resolve(resolver)

	if !complete {
		t.Fatalf("expected complete==true when all templates resolve; got false (out=%+v)", out)
	}
	if out.Host != "bastion.example.com" {
		t.Errorf("Host = %q, want %q", out.Host, "bastion.example.com")
	}
	if out.User != "deploy" {
		t.Errorf("User = %q, want %q", out.User, "deploy")
	}
	if out.KeyPath != "/home/deploy/id_ed25519" {
		t.Errorf("KeyPath = %q, want %q", out.KeyPath, "/home/deploy/id_ed25519")
	}
	if out.Passphrase != "s3cretphrase" {
		t.Errorf("Passphrase = %q, want %q", out.Passphrase, "s3cretphrase")
	}
	if out.Password != "s3cretpass" {
		t.Errorf("Password = %q, want %q", out.Password, "s3cretpass")
	}
	// Non-templated fields copied verbatim.
	if out.Port != 2222 {
		t.Errorf("Port = %d, want 2222 (copied verbatim)", out.Port)
	}
	if out.Auth != model.JumpAuthKey {
		t.Errorf("Auth = %q, want %q (copied verbatim)", out.Auth, model.JumpAuthKey)
	}
	if out.Insecure != true {
		t.Errorf("Insecure = %v, want true (copied verbatim)", out.Insecure)
	}
}

// SPEC 5b: an unresolved {{x}} in any field -> complete == false, and the literal
// "{{x}}" must NOT be silently passed off as a real (resolved) value. We assert
// the field still LITERALLY contains the unresolved template and complete is false.
func TestBlind_JumpHostResolve_UnresolvedNotComplete(t *testing.T) {
	// Resolver knows everything EXCEPT the host -> host stays "{{missingHost}}".
	resolver := bjhMapResolver(map[string]string{
		"u":  "deploy",
		"kp": "/k",
		"pp": "p",
		"pw": "w",
	})
	in := model.JumpHost{
		Host:       "{{missingHost}}",
		Port:       22,
		User:       "{{u}}",
		Auth:       model.JumpAuthKey,
		KeyPath:    "{{kp}}",
		Passphrase: "{{pp}}",
		Password:   "{{pw}}",
	}

	out, complete := in.Resolve(resolver)

	if complete {
		t.Fatalf("expected complete==false with an unresolved Host; got true (out=%+v)", out)
	}
	// The literal template must survive; it must not be coerced into a real value.
	if !strings.Contains(out.Host, "{{missingHost}}") {
		t.Errorf("Host = %q, expected it to still contain the literal {{missingHost}} (no silent pass-through)", out.Host)
	}
	// The fields that DID resolve should be resolved, proving complete is false
	// solely because of the unresolved one.
	if out.User != "deploy" {
		t.Errorf("User = %q, want resolved %q", out.User, "deploy")
	}
}

// SPEC 5b (each text field): an unresolved template in any single text field
// flips complete to false.
func TestBlind_JumpHostResolve_EachFieldGatesComplete(t *testing.T) {
	cases := []struct {
		name  string
		field func(*model.JumpHost)
	}{
		{"Host", func(j *model.JumpHost) { j.Host = "{{missing}}" }},
		{"User", func(j *model.JumpHost) { j.User = "{{missing}}" }},
		{"KeyPath", func(j *model.JumpHost) { j.KeyPath = "{{missing}}" }},
		{"Passphrase", func(j *model.JumpHost) { j.Passphrase = "{{missing}}" }},
		{"Password", func(j *model.JumpHost) { j.Password = "{{missing}}" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := model.JumpHost{Host: "h", User: "u", Auth: model.JumpAuthKey}
			c.field(&j)
			// Resolver that resolves nothing (identity) leaves {{missing}} literal.
			_, complete := j.Resolve(func(s string) string { return s })
			if complete {
				t.Errorf("complete==true but field %s still holds {{missing}}; want false", c.name)
			}
		})
	}
}

// SPEC 5c: a fully-literal JumpHost with a nil resolver resolves to itself and is
// complete; Port/Auth/Insecure are copied verbatim.
func TestBlind_JumpHostResolve_NilResolverIdentity(t *testing.T) {
	in := model.JumpHost{
		Host:       "literal.example.com",
		Port:       22,
		User:       "alice",
		Auth:       model.JumpAuthPassword,
		KeyPath:    "/no/key",
		Insecure:   false,
		Passphrase: "phrase",
		Password:   "pw",
	}

	out, complete := in.Resolve(nil)

	if !complete {
		t.Fatalf("nil resolver on a literal config should be complete; got false")
	}
	if out != in {
		t.Errorf("nil resolver should resolve a literal JumpHost to itself.\n got: %+v\nwant: %+v", out, in)
	}
}

// SPEC 5: a value that merely contains "{{" without a closing "}}" is NOT an
// unresolved template (no real reference), so it should not gate completeness.
func TestBlind_JumpHostResolve_LoneBracesAreNotTemplates(t *testing.T) {
	in := model.JumpHost{Host: "a{{b", User: "u", Auth: model.JumpAuthKey}
	out, complete := in.Resolve(nil)
	if !complete {
		t.Errorf("a value with '{{' but no closing '}}' is not a real template; want complete==true, got false (Host=%q)", out.Host)
	}
}
