// Package tunnel is Yon's app-level SSH jump-host connection manager: the
// headless core that turns a (resolved) model.JumpHost into a live, in-process
// SSH connection — the Tunnel (see CONTEXT.md) — and hands yonner a DialContext
// that dials every Request THROUGH it (ADR 0001). It owns connection lifecycle,
// refcounting, host-key verification (ADR 0002) and keepalive.
//
// Per the UI-free-core rule this package never imports Fyne; it exposes a
// Subscribe hook and a status snapshot so a future UI can observe it. The actual
// SSH dial is INJECTED (SSHDialer) so the whole lifecycle is unit-testable with a
// fake dialer and no network.
package tunnel

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ultramcu/yon/internal/model"
)

// State is the lifecycle state of one Tunnel.
type State int

const (
	// Disconnected: no live SSH connection (never opened, closed, or dropped).
	Disconnected State = iota
	// Connecting: an SSH handshake is in progress.
	Connecting
	// Connected: the SSH connection is up and dialing through.
	Connected
	// Error: the last connect/keepalive failed; Err carries the message. The
	// next dial will retry (lazy reconnect).
	Error
)

func (s State) String() string {
	switch s {
	case Disconnected:
		return "Disconnected"
	case Connecting:
		return "Connecting"
	case Connected:
		return "Connected"
	case Error:
		return "Error"
	default:
		return "Unknown"
	}
}

// keepaliveInterval is the default cadence at which a connected Tunnel probes
// liveness to detect a silently dropped connection (~15s per the design). It is
// the default for Manager.keepaliveInterval; the running loop reads the field
// so tests can shrink it via withKeepaliveInterval.
const keepaliveInterval = 15 * time.Second

// TunnelStatus is a read-only snapshot of one Tunnel for a status view. It is a
// value copy, safe to hand to a UI without locks.
type TunnelStatus struct {
	// ID is the distinct jump-host identity (see identity); also the Disconnect key.
	ID string
	// JumpHost is the display form "user@host:port".
	JumpHost string
	// State is the current lifecycle state.
	State State
	// Err is the last error message (empty unless State == Error).
	Err string
	// Since is when the Tunnel entered its current State.
	Since time.Time
	// RefCount is how many holders (windows/collections) currently Acquire it.
	RefCount int
}

// Manager owns one Tunnel per distinct jump-host identity, shared and refcounted
// across the whole app. Tunnels open lazily (on first dial), are reused, kept
// alive, auto-reconnected after a drop, and torn down when their refcount hits 0
// or on Close. The Manager is safe for concurrent use.
type Manager struct {
	dial     SSHDialer
	verifier *hostKeyVerifier

	mu      sync.Mutex
	tunnels map[string]*tunnel

	// subs are change listeners (e.g. a UI refresh). Fired on every state /
	// refcount change so observers can re-read Status.
	subMu sync.Mutex
	subs  []func()

	now func() time.Time // injectable clock for tests

	// keepaliveInterval is how often a connected Tunnel probes liveness; the
	// running loop reads this (production: 15s; tests inject a tiny interval via
	// withKeepaliveInterval so the real loop can be exercised quickly).
	keepaliveInterval time.Duration
}

// Option configures a Manager.
type Option func(*Manager)

// WithSSHDialer injects the SSH dial function (default: realSSHDialer). Tests
// pass a fake here to drive the lifecycle without a network.
func WithSSHDialer(d SSHDialer) Option {
	return func(m *Manager) { m.dial = d }
}

// WithKnownHosts injects the host-key store paths (default: ~/.ssh/known_hosts
// read-only + Yon's store in the config dir). Tests pass temp paths.
func WithKnownHosts(userKnownHosts, yonKnownHosts string) Option {
	return func(m *Manager) {
		m.verifier.userKnownHosts = userKnownHosts
		m.verifier.yonKnownHosts = yonKnownHosts
	}
}

// WithTOFU injects the trust-on-first-use callback for unknown host keys
// (default: RejectTOFU, safe for headless use). Phase 2 supplies the UI prompt.
func WithTOFU(tofu TOFUFunc) Option {
	return func(m *Manager) {
		if tofu != nil {
			m.verifier.tofu = tofu
		}
	}
}

// withClock injects a clock (tests). Unexported: production always uses time.Now.
func withClock(now func() time.Time) Option {
	return func(m *Manager) { m.now = now }
}

// withKeepaliveInterval sets the keepalive probe interval. Unexported:
// production always uses the 15s default; tests inject a tiny interval to drive
// the real keepalive loop without waiting 15s.
func withKeepaliveInterval(d time.Duration) Option {
	return func(m *Manager) { m.keepaliveInterval = d }
}

// New builds a Manager. With no options it uses the real SSH dialer, the real
// known-hosts paths, and the reject-by-default TOFU policy.
func New(opts ...Option) *Manager {
	m := &Manager{
		dial:              realSSHDialer,
		verifier:          newHostKeyVerifier(defaultUserKnownHosts(), defaultYonKnownHosts(), RejectTOFU),
		tunnels:           make(map[string]*tunnel),
		now:               time.Now,
		keepaliveInterval: keepaliveInterval,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// tunnel is the per-identity live connection state. All fields are guarded by
// the Manager's mu (the manager is the single owner; tunnels are never used
// outside it).
type tunnel struct {
	id   string
	jh   model.JumpHost // resolved config (used to reconnect lazily)
	conn SSHConn        // nil unless state == Connected

	state State
	err   error
	since time.Time
	refs  int

	// connecting is non-nil while exactly one goroutine is dialing this tunnel
	// (the in-flight gate against the thundering herd). The dialer creates it,
	// drops mu to run the blocking dial, then re-takes mu, stores the result, and
	// closes the channel so waiters wake and reuse it. Other cold dials that
	// arrive meanwhile find it non-nil and wait on it instead of dialing again.
	connecting chan struct{}

	// keepaliveStop cancels the running keepalive loop (nil unless Connected).
	keepaliveStop chan struct{}
}

// identity is the distinct jump-host key: two jump hosts that resolve to the
// same host+port+user+auth+keyPath share ONE Tunnel. Secrets are deliberately
// excluded so they don't appear in the key (and don't fragment sharing).
func identity(jh model.JumpHost) string {
	return fmt.Sprintf("%s\x00%d\x00%s\x00%s\x00%s",
		jh.Host, portOrDefault(jh.Port), jh.User, jh.Auth, jh.KeyPath)
}

// display returns the "user@host:port" form for status views.
func display(jh model.JumpHost) string {
	return jh.User + "@" + net.JoinHostPort(jh.Host, strconv.Itoa(portOrDefault(jh.Port)))
}

// Acquire registers a holder for the jump host's Tunnel and returns its ID plus
// a release func. It does NOT open the connection (that's lazy, on first dial).
// Each window/collection using a jump host Acquires on start and Releases on
// stop; when the refcount reaches 0 the Tunnel is disconnected and dropped.
//
// The caller must pass an already-RESOLVED, complete jump host. Acquire errors
// if the config is incomplete (still carries {{templates}}) so an unresolved
// "{{host}}" is never connected.
func (m *Manager) Acquire(jh model.JumpHost) (id string, release func(), err error) {
	if _, complete := jh.Resolve(nil); !complete {
		return "", nil, fmt.Errorf("tunnel: jump host config is incomplete (unresolved variables); not connecting")
	}

	id = identity(jh)

	m.mu.Lock()
	t := m.tunnels[id]
	if t == nil {
		t = &tunnel{id: id, jh: jh, state: Disconnected, since: m.now()}
		m.tunnels[id] = t
	}
	t.refs++
	m.mu.Unlock()

	m.notify()

	var once sync.Once
	return id, func() {
		once.Do(func() { m.release(id) })
	}, nil
}

// release drops one reference; at 0 the Tunnel is disconnected and removed.
func (m *Manager) release(id string) {
	m.mu.Lock()
	t := m.tunnels[id]
	if t == nil {
		m.mu.Unlock()
		return
	}
	t.refs--
	if t.refs <= 0 {
		m.closeLocked(t)
		delete(m.tunnels, id)
	}
	m.mu.Unlock()
	m.notify()
}

// DialContext returns the dialer yonner installs as transport.DialContext for a
// given jump host. On each call it ensures the Tunnel is up (lazy open /
// reconnect after a drop) and then dials the Request's target THROUGH it. The
// returned func captures the jump-host identity, so it stays valid across
// reconnects.
//
// The caller must Acquire the jump host first (so the Tunnel isn't torn down
// mid-flight); DialContext does not itself hold a reference.
func (m *Manager) DialContext(jh model.JumpHost) func(ctx context.Context, network, addr string) (net.Conn, error) {
	id := identity(jh)
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := m.ensure(ctx, id, jh)
		if err != nil {
			return nil, err
		}
		return conn.DialContext(ctx, network, addr)
	}
}

// Connect eagerly opens (or reconnects) the Tunnel for jh without performing a
// dial — used by a manual "Connect" button. It Acquires-then-Releases is NOT
// implied; the caller owns refcounting. Returns once the SSH connection is up or
// the attempt fails.
func (m *Manager) Connect(ctx context.Context, jh model.JumpHost) error {
	if _, complete := jh.Resolve(nil); !complete {
		return fmt.Errorf("tunnel: jump host config is incomplete (unresolved variables); not connecting")
	}
	id := identity(jh)

	// Ensure the tunnel record exists even if nobody Acquired it (manual connect
	// from the status view before any send).
	m.mu.Lock()
	if m.tunnels[id] == nil {
		m.tunnels[id] = &tunnel{id: id, jh: jh, state: Disconnected, since: m.now()}
	}
	m.mu.Unlock()

	_, err := m.ensure(ctx, id, jh)
	return err
}

// Disconnect tears down the Tunnel with the given ID (a manual "Disconnect"
// button). The record is kept (so its refcount survives) but moved to
// Disconnected; the next dial lazily reconnects. In-flight dials through the old
// connection fail, as designed.
func (m *Manager) Disconnect(id string) {
	m.mu.Lock()
	t := m.tunnels[id]
	if t != nil {
		m.closeLocked(t)
	}
	m.mu.Unlock()
	m.notify()
}

// ensure returns a live SSHConn for the tunnel, opening it lazily on first use
// and reconnecting after a drop. Concurrent cold dials of the same identity are
// de-duplicated by an in-flight gate (tunnel.connecting): the FIRST goroutine
// dials the bastion exactly once while the rest wait on its channel and reuse
// the winner's connection (or share its error). This honours ADR 0001 ("opened
// lazily on the first send … kept alive and reused") and avoids a thundering
// herd of real SSH handshakes against the bastion. The blocking dial runs
// OUTSIDE m.mu so other Status()/Acquire() calls never stall on a slow
// handshake.
func (m *Manager) ensure(ctx context.Context, id string, jh model.JumpHost) (SSHConn, error) {
	for {
		m.mu.Lock()
		t := m.tunnels[id]
		if t == nil {
			// No Acquire/Connect record yet: create an ephemeral one so a bare
			// DialContext still works (refcount stays 0; caller is expected to
			// Acquire, but we don't crash if it didn't).
			t = &tunnel{id: id, jh: jh, state: Disconnected, since: m.now()}
			m.tunnels[id] = t
		}

		// Fast path: already connected and alive → reuse.
		if t.state == Connected && t.conn != nil {
			conn := t.conn
			m.mu.Unlock()
			return conn, nil
		}

		// A dial is already in flight (cold first-dial or reconnect-after-drop):
		// don't open a second SSH connection — wait for the in-flight one, then
		// loop to re-read the (now Connected or Error) state under mu and reuse it.
		if t.connecting != nil {
			wait := t.connecting
			m.mu.Unlock()
			select {
			case <-wait:
				// In-flight dial finished; re-loop to read its result.
				continue
			case <-ctx.Done():
				// Our caller gave up while waiting; don't leak this goroutine and
				// don't disturb the dial the other goroutine is driving.
				return nil, ctx.Err()
			}
		}

		// We are the first cold dialer: claim the in-flight gate so concurrent
		// callers wait instead of dialing, then release mu for the blocking dial.
		ready := make(chan struct{})
		t.connecting = ready
		t.setStateLocked(Connecting, nil, m.now())
		m.mu.Unlock()
		m.notify()

		// Build the host-key callback for this jump host's Insecure setting (ADR
		// 0002). Insecure skips all verification; otherwise the verifier reads
		// known_hosts + Yon's store and TOFU-prompts unknown hosts.
		hkcb := m.verifier.callbackFor(jh.Insecure)

		conn, err := m.dial(ctx, jh, hkcb)

		m.mu.Lock()
		// Publish the result and open the gate so waiters wake. Clearing
		// connecting first means a later cold dial (e.g. after a drop) can dial
		// again rather than being stuck behind a stale channel.
		t.connecting = nil
		if err != nil {
			t.setStateLocked(Error, err, m.now())
		} else {
			t.conn = conn
			t.setStateLocked(Connected, nil, m.now())
			m.startKeepaliveLocked(t)
		}
		close(ready)
		m.mu.Unlock()
		m.notify()

		if err != nil {
			return nil, err
		}
		return conn, nil
	}
}

// startKeepaliveLocked launches the per-tunnel keepalive loop. On a probe
// failure it marks the Tunnel dropped (Error) so the next dial reconnects.
// Caller holds mu.
func (m *Manager) startKeepaliveLocked(t *tunnel) {
	stop := make(chan struct{})
	t.keepaliveStop = stop
	conn := t.conn
	id := t.id

	go func() {
		ticker := time.NewTicker(m.keepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if err := conn.Keepalive(); err != nil {
					m.markDropped(id, conn, err)
					return
				}
			}
		}
	}()
}

// markDropped transitions a tunnel to Error after a keepalive failure, but only
// if it is still the live connection (a manual reconnect may have replaced it).
func (m *Manager) markDropped(id string, conn SSHConn, cause error) {
	m.mu.Lock()
	t := m.tunnels[id]
	if t != nil && t.conn == conn {
		// Don't call closeLocked (it would re-close conn and stop this very
		// goroutine's channel); just detach and surface the drop.
		t.conn = nil
		t.keepaliveStop = nil
		_ = conn.Close()
		t.setStateLocked(Error, fmt.Errorf("tunnel: connection dropped: %w", cause), m.now())
	}
	m.mu.Unlock()
	m.notify()
}

// closeLocked tears down a tunnel's live connection and stops its keepalive,
// returning it to Disconnected. Caller holds mu.
func (m *Manager) closeLocked(t *tunnel) {
	if t.keepaliveStop != nil {
		close(t.keepaliveStop)
		t.keepaliveStop = nil
	}
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
	t.setStateLocked(Disconnected, nil, m.now())
}

// setStateLocked updates state/err/since. Caller holds mu.
func (t *tunnel) setStateLocked(s State, err error, now time.Time) {
	t.state = s
	t.err = err
	t.since = now
}

// Status returns a snapshot of every known Tunnel, sorted by display name for a
// stable view.
func (m *Manager) Status() []TunnelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]TunnelStatus, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		errMsg := ""
		if t.err != nil {
			errMsg = t.err.Error()
		}
		out = append(out, TunnelStatus{
			ID:       t.id,
			JumpHost: display(t.jh),
			State:    t.state,
			Err:      errMsg,
			Since:    t.since,
			RefCount: t.refs,
		})
	}
	return out
}

// Subscribe registers fn to be called (from an arbitrary goroutine) whenever any
// Tunnel's state or refcount changes, so a UI can refresh. It returns an
// unsubscribe func. The callback must not block.
func (m *Manager) Subscribe(fn func()) (unsubscribe func()) {
	m.subMu.Lock()
	idx := len(m.subs)
	m.subs = append(m.subs, fn)
	m.subMu.Unlock()
	return func() {
		m.subMu.Lock()
		if idx < len(m.subs) {
			m.subs[idx] = nil
		}
		m.subMu.Unlock()
	}
}

// notify fires all subscribers. Called after every state/refcount change.
func (m *Manager) notify() {
	m.subMu.Lock()
	subs := make([]func(), len(m.subs))
	copy(subs, m.subs)
	m.subMu.Unlock()
	for _, fn := range subs {
		if fn != nil {
			fn()
		}
	}
}

// Close disconnects every Tunnel and clears the manager (app quit). After Close
// the manager can still be reused (records are recreated lazily).
func (m *Manager) Close() {
	m.mu.Lock()
	for id, t := range m.tunnels {
		m.closeLocked(t)
		delete(m.tunnels, id)
	}
	m.mu.Unlock()
	m.notify()
}

// compile-time check: *ssh.Client (via the adapter) satisfies SSHConn.
var _ SSHConn = sshClientConn{}
var _ ssh.HostKeyCallback = (*hostKeyVerifier)(nil).verify
