package model

import "strings"

// JumpHost auth kinds (the JumpHost.Auth field). Auth is a free-form string in
// JSON; these are the two values v1 understands.
const (
	// JumpAuthKey authenticates with a private key (KeyPath + optional
	// Passphrase).
	JumpAuthKey = "key"
	// JumpAuthPassword authenticates with a password.
	JumpAuthPassword = "password"
)

// JumpHost is the SSH jump-host configuration carried by an Environment: the
// SSH server an Environment's Requests should be dialed THROUGH (see CONTEXT.md
// — this is settings, not the live Tunnel). It is pure data; the tunnel manager
// turns it into a connection.
//
// Storage: Host/Port/User/Auth/KeyPath/Insecure live in the committed
// environment JSON, while the secret fields Passphrase/Password are blanked in
// the committed JSON and kept in the gitignored .env (see the store package),
// mirroring how a Secret Variable is split out.
type JumpHost struct {
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"` // default 22
	User     string `json:"user"`
	Auth     string `json:"auth"`               // "key" | "password"
	KeyPath  string `json:"keyPath,omitempty"`  // private key path for "key" auth
	Insecure bool   `json:"insecure,omitempty"` // skip host-key check
	// Secret fields — blanked in committed JSON, stored in .env:
	Passphrase string `json:"passphrase,omitempty"` // unlocks the private key
	Password   string `json:"password,omitempty"`   // for "password" auth
}

// Resolve returns a copy of the JumpHost with every {{variable}}-bearing field
// expanded via resolve (the same kind of func(string) string used by
// yonner.Options.Resolve / the UI's varScope). Host, Port, User, KeyPath,
// Passphrase and Password are resolved; Port is resolved as text then handed
// back unchanged (it is numeric, so it never carries a template — only the text
// fields can). Auth and Insecure are not templated and are copied verbatim.
//
// complete reports whether the resolved config is ready to dial: it is false
// when any resolved field STILL contains a "{{…}}" template, because the
// resolver leaves an unknown reference literal (Postman behaviour). The tunnel
// manager must refuse to connect when complete is false so Yon never dials a
// literal "{{host}}". A nil resolver is treated as identity (fields pass
// through unchanged), so a literal config resolves to itself and is complete.
func (j JumpHost) Resolve(resolve func(string) string) (resolved JumpHost, complete bool) {
	if resolve == nil {
		resolve = func(s string) string { return s }
	}

	out := j // copy non-templated fields (Port, Auth, Insecure) as-is
	out.Host = resolve(j.Host)
	out.User = resolve(j.User)
	out.KeyPath = resolve(j.KeyPath)
	out.Passphrase = resolve(j.Passphrase)
	out.Password = resolve(j.Password)

	// complete is true only when no resolved text field still holds a template.
	complete = !hasTemplate(out.Host) &&
		!hasTemplate(out.User) &&
		!hasTemplate(out.KeyPath) &&
		!hasTemplate(out.Passphrase) &&
		!hasTemplate(out.Password)
	return out, complete
}

// hasTemplate reports whether s still contains an unresolved "{{…}}" reference
// (an opening "{{" followed by a later "}}"). The resolver leaves unknown
// references literal, so this is how we detect an incomplete jump-host config.
func hasTemplate(s string) bool {
	open := strings.Index(s, "{{")
	if open < 0 {
		return false
	}
	return strings.Contains(s[open+2:], "}}")
}

// Variable is a single {{template}} value. Key is the name used as {{Key}} in
// any text field (URL, params, headers, body, auth). A disabled Variable is kept
// but not applied. When Secret is true the value is NOT persisted in the
// committed environment/collection file; it is stored separately in a gitignored
// .env file (see the store package) so credentials stay out of version control.
type Variable struct {
	Key     string `json:"key"`
	Value   string `json:"value,omitempty"`
	Enabled bool   `json:"enabled"`
	Secret  bool   `json:"secret,omitempty"`
}

// Environment is a named set of Variables (e.g. "Local", "Staging", "Prod").
// Exactly one environment is active per collection at a time; its variables take
// precedence over collection-level variables. Environments are persisted in
// sibling files, one per environment, not inside the .yon file.
type Environment struct {
	Name      string     `json:"name"`
	Variables []Variable `json:"variables,omitempty"`
	// JumpHost is the optional SSH jump host dialed through when this
	// environment is active. nil (omitempty) means none, so an environment
	// without a jump host serializes exactly as before.
	JumpHost *JumpHost `json:"jumpHost,omitempty"`
}
