# SSH jump host — feature design

Reach a host that is only reachable from a bastion by giving an **Environment** an
**SSH jump host**: when that Environment is active, Yon holds open one in-process
**Tunnel** to the jump host and dials every Request through it — the URL is never
rewritten. Terms: see [CONTEXT.md](../CONTEXT.md). Key decisions:
[ADR 0001](adr/0001-ssh-jump-host-in-process-dial-through.md) (approach),
[ADR 0002](adr/0002-ssh-host-key-verification.md) (host-key policy).

## Data model (`internal/model`)

```go
type Environment struct {
    Name      string
    Variables []Variable
    JumpHost  *JumpHost `json:"jumpHost,omitempty"` // nil = none
}

type JumpHost struct {
    Host     string `json:"host"`
    Port     int    `json:"port,omitempty"` // default 22
    User     string `json:"user"`
    Auth     string `json:"auth"`           // "key" | "password"
    KeyPath  string `json:"keyPath,omitempty"`
    Insecure bool   `json:"insecure,omitempty"` // skip host-key check
    // secret — blanked in committed JSON, stored in .env:
    Passphrase string `json:"passphrase,omitempty"`
    Password   string `json:"password,omitempty"`
}
```

- **Storage:** Host/Port/User/Auth/KeyPath/Insecure live in the committed
  environment JSON; **Passphrase/Password** are blanked there and written to the
  gitignored `.env` under reserved keys (e.g. `__jumphost.passphrase`,
  `__jumphost.password`) via the existing Secret plumbing in `internal/store`.
- **Backward compatible:** an Environment with no `jumpHost` serializes and loads
  exactly as today (`omitempty`).
- **Variable resolution:** every field (Host, Port, User, KeyPath, Passphrase,
  Password) is resolved with the active Environment's `{{variables}}` before
  connecting. If any field still contains `{{…}}` after resolution the config is
  incomplete → no connect, surface an error (never dial a literal `{{host}}`).

## Engine (`internal/yonner`)

- `Options` gains a way to supply an SSH-backed dialer (a `DialContext` produced by
  the tunnel manager for the active jump host; nil = none).
- `newClient`: when a jump host is active, set `transport.DialContext =
  sshClient.DialContext` **and** `transport.Proxy = nil` (bypass HTTP proxy — see
  ADR 0001). No jump host → transport unchanged (proxy honored).
- The UI-free-core rule holds: `yonner` only consumes a dial function; it never
  imports Fyne or owns connection lifecycle.

## Tunnel manager (app-level, `internal/ui` App)

- **One Tunnel per distinct jump-host identity** (resolved host+port+user+auth+
  keyPath), **refcounted** across windows/collections; closed when refcount hits 0
  or on app quit.
- **Lazy open** on the first send under an Environment that has a jump host;
  **reused** for later sends; **keepalive** (~15s) detects drops; **auto-reconnect**
  on the next send if dropped; **manual Connect / Disconnect** from the status view.
- **States:** `Disconnected` · `Connecting` · `Connected` · `Error(message)`.
  State changes publish an event so views refresh (via `fyne.Do`).
- **Host-key verification:** per [ADR 0002](adr/0002-ssh-host-key-verification.md)
  — read `~/.ssh/known_hosts` read-only + Yon's own store; unknown → TOFU prompt
  (accept → Yon's store); mismatch → hard reject; per-jump-host `Insecure` skips.
- **Testability:** the SSH-dial function is injected into the manager, so
  lifecycle/refcount/state are unit-testable with a fake dialer.

## UI

- **Environment manager:** an "SSH jump host" section to set host/port/user, auth
  (key + key path + passphrase, or password), and the *Skip host key check* toggle.
- **Tunnels window** (`Collection ▸ Tunnels…`): an app-global table, one row per
  Tunnel — jump host (`user@host:port`), State, Used by (env/collection + refcount),
  Since (uptime), last Error, and **Connect / Disconnect** buttons. Live-updating.
- **Footer indicator** (per window, beside the version): when the active
  Environment has a jump host, a coloured dot + env name (`🟢 Staging` /
  `🔴 Staging`); clicking it opens the Tunnels window.

## Failure UX

- A connect / auth / host-key failure makes the send return an error shown in the
  response area (like any connection error), and the Tunnels window + footer show
  `Error` with the message.
- If a Tunnel drops or is manually Disconnected with requests in flight, those
  requests fail with a clear error; the next send lazily reconnects.

## Scope / non-goals (v1)

- Exactly **one** jump host per Environment; **no chained ProxyJump**.
- Auth is **private key (path + passphrase) or password** only — **no SSH agent**
  (Pageant/named-pipe on Windows makes it cross-platform-fiddly); follow-up.
- **No** reaching the bastion through a local HTTP proxy; follow-up.
- **No** local port-forward or SOCKS — dial-through only (ADR 0001).

## Dependency

- `golang.org/x/crypto/ssh` (pure Go, no cgo — keeps Yon a single binary).

## Testing

- Manager lifecycle/refcount/state via an **injected fake dialer** (no network).
- Config + secret **`.env` round-trip**; backward-compat (env without a jump host
  is byte-identical to before).
- **Variable resolution** of jump-host fields (incl. unresolved → no-connect).
- **Proxy-bypass**: with a jump host `transport.Proxy == nil`; without, it's honored.
- One **integration test** against an **in-process `x/crypto/ssh` server** (real
  handshake + dial-through to a target served behind it).
