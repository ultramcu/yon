// Package variables resolves Postman-style {{template}} references in Yon
// request text (URL, params, headers, auth, body).
//
// A Scope gathers the variable sources — the active environment, the
// collection-scoped variables, and the secret values loaded from the gitignored
// .env file — and exposes Lookup (single key) and Resolve (whole string). The
// engine is pure: it depends only on the standard library (and Yon's model
// package) and imports neither Fyne nor any networking package.
package variables

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/ultramcu/yon/internal/model"
)

// maxPasses bounds how many times Resolve re-expands a string so that a value
// referencing another variable still resolves, while a self-referential value
// can never loop forever.
const maxPasses = 10

// Scope is the set of variable sources used to resolve {{templates}}.
type Scope struct {
	Env        model.Environment // active environment (Name may be "")
	Collection []model.Variable  // collection-scoped variables
	Secrets    map[string]string // secret values by key (supplied from the .env file)
}

// Lookup returns the value bound to key and whether it was found. Precedence:
// the active environment's enabled variables win over collection-scoped enabled
// variables. For a Secret variable the value comes from Secrets[key] (the env/
// collection entry's own Value is ignored/blank). Disabled variables are skipped.
func (sc Scope) Lookup(key string) (string, bool) {
	if v, ok := lookupIn(sc.Env.Variables, key, sc.Secrets); ok {
		return v, true
	}
	return lookupIn(sc.Collection, key, sc.Secrets)
}

// lookupIn scans vars for the first enabled Variable whose Key matches key and
// returns its effective value. A Secret variable resolves to secrets[key] when
// that key is present in the Secrets map; otherwise it falls back to the
// Variable's own Value. This supports both flows: the UI builds the Scope with a
// nil Secrets map and relies on store.LoadEnvironments having merged the .env
// secret into Variable.Value, while a caller that supplies a Secrets map keeps
// the secret out of the Variable. A normal variable resolves to its own Value.
func lookupIn(vars []model.Variable, key string, secrets map[string]string) (string, bool) {
	for _, v := range vars {
		if !v.Enabled || v.Key != key {
			continue
		}
		if v.Secret {
			if s, ok := secrets[key]; ok {
				return s, true
			}
		}
		return v.Value, true
	}
	return "", false
}

// Resolve expands every {{name}} reference in s. {{name}} resolves via Lookup;
// {{$dynamic}} forms generate a value; an unknown {{x}} is left LITERAL (Postman
// behaviour). Supported dynamics: {{$uuid}} (RFC-4122 v4), {{$timestamp}} (unix
// seconds), {{$isoTimestamp}} (RFC3339 UTC), {{$randomInt}} (0..1000).
// Resolution is applied repeatedly so a variable may reference another variable,
// up to a small fixed pass limit with a cycle guard (a {{x}} that resolves to
// text containing {{x}} must not loop forever — leave it once it stops changing).
func (sc Scope) Resolve(s string) string {
	for pass := 0; pass < maxPasses; pass++ {
		next := sc.expandOnce(s)
		if next == s {
			break
		}
		s = next
	}
	return s
}

// expandOnce performs a single left-to-right pass over s, replacing each
// {{name}} reference whose name is known (a static variable or a dynamic
// generator) and leaving every unknown reference untouched.
func (sc Scope) expandOnce(s string) string {
	var b strings.Builder
	i := 0
	for {
		open := strings.Index(s[i:], "{{")
		if open < 0 {
			b.WriteString(s[i:])
			break
		}
		open += i
		close := strings.Index(s[open+2:], "}}")
		if close < 0 {
			// No closing delimiter; the rest is literal.
			b.WriteString(s[i:])
			break
		}
		close += open + 2

		name := strings.TrimSpace(s[open+2 : close])
		if value, ok := sc.expandRef(name); ok {
			b.WriteString(s[i:open])
			b.WriteString(value)
		} else {
			// Unknown reference: leave the literal {{...}} untouched.
			b.WriteString(s[i : close+2])
		}
		i = close + 2
	}
	return b.String()
}

// expandRef resolves a single reference name (already trimmed). A $-prefixed
// name is a dynamic generator; otherwise it is looked up in the scope. The
// second result reports whether the name is known.
func (sc Scope) expandRef(name string) (string, bool) {
	if strings.HasPrefix(name, "$") {
		return dynamic(name)
	}
	return sc.Lookup(name)
}

// dynamic generates a value for a {{$name}} dynamic reference. It reports false
// for an unrecognised generator so the reference is left literal.
func dynamic(name string) (string, bool) {
	switch name {
	case "$uuid":
		return uuidV4(), true
	case "$timestamp":
		return strconv.FormatInt(time.Now().Unix(), 10), true
	case "$isoTimestamp":
		return time.Now().UTC().Format(time.RFC3339), true
	case "$randomInt":
		return strconv.Itoa(randIntn(1001)), true // 0..1000 inclusive
	default:
		return "", false
	}
}

// uuidV4 returns a random RFC-4122 version-4 UUID.
func uuidV4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// randIntn returns a crypto-random int in [0, n). It assumes n > 0.
func randIntn(n int) int {
	if n <= 1 {
		return 0
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0
	}
	return int(binary.BigEndian.Uint64(buf[:]) % uint64(n))
}
