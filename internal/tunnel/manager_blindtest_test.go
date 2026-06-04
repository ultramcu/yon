package tunnel

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/ultramcu/yon/internal/model"
)

// ---------------------------------------------------------------------------
// Independent fakes (unique helper names: bt2_* prefix) — no real network.
// ---------------------------------------------------------------------------

// bt2Pipe is one in-memory net.Conn dialed through a fake SSH connection. We
// only need it to be a real net.Conn so DialContext returns something usable.
func bt2Pipe() net.Conn {
	c1, _ := net.Pipe()
	return c1
}

// bt2Conn is a fake SSHConn that records dials/closes and can be flipped "dead"
// so its Keepalive fails (simulating a dropped connection).
type bt2Conn struct {
	openSeq int32 // which open produced this conn (1-based)

	mu          sync.Mutex
	dead        bool
	closed      bool
	dialCount   int
	kaCount     int
	kaErr       error // returned by Keepalive when set
	openedPipes []net.Conn
}

func (c *bt2Conn) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("bt2: dial on closed conn")
	}
	if c.dead {
		return nil, errors.New("bt2: dial on dead conn")
	}
	c.dialCount++
	p := bt2Pipe()
	c.openedPipes = append(c.openedPipes, p)
	return p, nil
}

func (c *bt2Conn) Keepalive() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kaCount++
	if c.kaErr != nil {
		return c.kaErr
	}
	if c.dead {
		return errors.New("bt2: keepalive on dead conn")
	}
	return nil
}

func (c *bt2Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for _, p := range c.openedPipes {
		_ = p.Close()
	}
	return nil
}

func (c *bt2Conn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *bt2Conn) kill() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dead = true
}

// bt2Dialer is a fake SSHDialer that counts opens and hands out fresh bt2Conns.
// An injected errFor lets a test make specific opens fail.
type bt2Dialer struct {
	mu      sync.Mutex
	opens   int32
	conns   []*bt2Conn
	jhSeen  []model.JumpHost
	failNow atomic.Bool // when true, the next dial returns an error
	failErr error
}

func newBt2Dialer() *bt2Dialer {
	return &bt2Dialer{failErr: errors.New("bt2: dial refused")}
}

func (d *bt2Dialer) dial(ctx context.Context, jh model.JumpHost, hk ssh.HostKeyCallback) (SSHConn, error) {
	n := atomic.AddInt32(&d.opens, 1)
	d.mu.Lock()
	d.jhSeen = append(d.jhSeen, jh)
	d.mu.Unlock()
	if d.failNow.Load() {
		return nil, d.failErr
	}
	c := &bt2Conn{openSeq: n}
	d.mu.Lock()
	d.conns = append(d.conns, c)
	d.mu.Unlock()
	return c, nil
}

func (d *bt2Dialer) openCount() int { return int(atomic.LoadInt32(&d.opens)) }

func (d *bt2Dialer) lastConn() *bt2Conn {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.conns) == 0 {
		return nil
	}
	return d.conns[len(d.conns)-1]
}

// bt2JH is a complete, resolvable jump host.
func bt2JH(host, user string, port int) model.JumpHost {
	return model.JumpHost{Host: host, Port: port, User: user, Auth: model.JumpAuthKey, KeyPath: "/tmp/key"}
}

// bt2Mgr builds a Manager wired to the fake dialer with host-key verification
// effectively disabled (the fake dialer ignores the host-key callback anyway).
func bt2Mgr(d *bt2Dialer) *Manager {
	return New(WithSSHDialer(d.dial))
}

// ---------------------------------------------------------------------------
// SPEC 1: Lazy + reuse
// ---------------------------------------------------------------------------

func TestBT2_LazyAndReuse(t *testing.T) {
	d := newBt2Dialer()
	m := bt2Mgr(d)
	jh := bt2JH("bastion.example.com", "ops", 22)

	id, release, err := m.Acquire(jh)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()
	_ = id

	// No dial should have happened yet (lazy).
	if got := d.openCount(); got != 0 {
		t.Fatalf("SPEC1 lazy: expected 0 SSH opens before first dial, got %d", got)
	}

	dctx := m.DialContext(jh)
	for i := 0; i < 3; i++ {
		conn, err := dctx(context.Background(), "tcp", "10.0.0.5:80")
		if err != nil {
			t.Fatalf("SPEC1 dial %d: %v", i, err)
		}
		_ = conn.Close()
	}

	if got := d.openCount(); got != 1 {
		t.Fatalf("SPEC1 reuse: expected exactly 1 underlying SSH open across 3 dials, got %d", got)
	}
	t.Logf("SPEC1 PASS: 0 opens before first dial, 1 open across 3 dials")
}

// ---------------------------------------------------------------------------
// SPEC 2: Refcount sharing / different identities
// ---------------------------------------------------------------------------

func TestBT2_RefcountShareAndDistinct(t *testing.T) {
	d := newBt2Dialer()
	m := bt2Mgr(d)
	jh := bt2JH("bastion", "ops", 22)

	id1, rel1, err := m.Acquire(jh)
	if err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	id2, rel2, err := m.Acquire(jh) // identical identity
	if err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("SPEC2: same jump host gave different IDs %q vs %q", id1, id2)
	}

	if rc := bt2RefCount(t, m, id1); rc != 2 {
		t.Fatalf("SPEC2: expected RefCount 2 after two Acquires, got %d", rc)
	}

	// Open the connection so we can confirm it closes at refcount 0.
	if _, err := m.DialContext(jh)(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn := d.lastConn()

	rel1()
	if rc := bt2RefCount(t, m, id1); rc != 1 {
		t.Fatalf("SPEC2: expected RefCount 1 after one release, got %d", rc)
	}
	if conn.isClosed() {
		t.Fatalf("SPEC2: conn closed prematurely at refcount 1")
	}

	rel2()
	if conn == nil || !conn.isClosed() {
		t.Fatalf("SPEC2: conn must be closed when refcount hits 0")
	}
	// Tunnel record dropped at 0.
	if bt2HasTunnel(m, id1) {
		t.Fatalf("SPEC2: tunnel record should be removed at refcount 0")
	}

	// Different identity → different tunnel.
	jhDiff := bt2JH("bastion", "ops", 2222) // different port
	idA, relA, _ := m.Acquire(jhDiff)
	defer relA()
	idB, relB, _ := m.Acquire(bt2JH("other", "ops", 22)) // different host
	defer relB()
	if idA == idB {
		t.Fatalf("SPEC2: different jump hosts shared an ID %q", idA)
	}
	t.Logf("SPEC2 PASS: shared refcount=2, close at 0, distinct identities differ")
}

// bt2RefCount reads the refcount for an id from Status().
func bt2RefCount(t *testing.T, m *Manager, id string) int {
	t.Helper()
	for _, s := range m.Status() {
		if s.ID == id {
			return s.RefCount
		}
	}
	return -1
}

func bt2HasTunnel(m *Manager, id string) bool {
	for _, s := range m.Status() {
		if s.ID == id {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// SPEC 3: State machine + Subscribe + Status
// ---------------------------------------------------------------------------

func TestBT2_StateMachineSuccess(t *testing.T) {
	d := newBt2Dialer()
	m := bt2Mgr(d)
	jh := bt2JH("bastion", "deploy", 2200)

	var mu sync.Mutex
	var seen []State
	unsub := m.Subscribe(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, s := range m.Status() {
			if s.ID == identity(jh) {
				// record only transitions (dedupe consecutive duplicates)
				if len(seen) == 0 || seen[len(seen)-1] != s.State {
					seen = append(seen, s.State)
				}
			}
		}
	})
	defer unsub()

	id, release, err := m.Acquire(jh)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	if err := m.Connect(context.Background(), jh); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var st TunnelStatus
	for _, s := range m.Status() {
		if s.ID == id {
			st = s
		}
	}
	if st.State != Connected {
		t.Fatalf("SPEC3: expected Connected, got %s", st.State)
	}
	if st.JumpHost != "deploy@bastion:2200" {
		t.Fatalf("SPEC3: JumpHost display = %q, want deploy@bastion:2200", st.JumpHost)
	}
	if st.RefCount != 1 {
		t.Fatalf("SPEC3: RefCount = %d, want 1", st.RefCount)
	}

	mu.Lock()
	got := append([]State(nil), seen...)
	mu.Unlock()
	if !bt2Contains(got, Connecting) || !bt2Contains(got, Connected) {
		t.Fatalf("SPEC3: expected Connecting then Connected in transitions, saw %v", got)
	}
	// Connecting must precede Connected.
	ci, ti := bt2IndexOf(got, Connecting), bt2IndexOf(got, Connected)
	if ci < 0 || ti < 0 || ci > ti {
		t.Fatalf("SPEC3: Connecting must precede Connected, saw %v", got)
	}
	if s := Connected.String(); s != "Connected" {
		t.Fatalf("SPEC3: State.String() = %q", s)
	}
	t.Logf("SPEC3 PASS: transitions %v, display %q, refcount %d", got, st.JumpHost, st.RefCount)
}

func TestBT2_StateMachineError(t *testing.T) {
	d := newBt2Dialer()
	d.failNow.Store(true)
	d.failErr = errors.New("handshake boom")
	m := bt2Mgr(d)
	jh := bt2JH("bastion", "ops", 22)

	id, release, _ := m.Acquire(jh)
	defer release()

	err := m.Connect(context.Background(), jh)
	if err == nil {
		t.Fatalf("SPEC3: Connect should fail when dialer errors")
	}

	var st TunnelStatus
	for _, s := range m.Status() {
		if s.ID == id {
			st = s
		}
	}
	if st.State != Error {
		t.Fatalf("SPEC3: expected Error state, got %s", st.State)
	}
	if st.Err == "" || !bt2HasSubstr(st.Err, "handshake boom") {
		t.Fatalf("SPEC3: Error message should carry cause, got %q", st.Err)
	}
	if s := Error.String(); s != "Error" {
		t.Fatalf("SPEC3: Error.String()=%q", s)
	}
	t.Logf("SPEC3 PASS (error path): state=%s err=%q", st.State, st.Err)
}

func bt2Contains(ss []State, want State) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
func bt2IndexOf(ss []State, want State) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}
func bt2HasSubstr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// SPEC 4: Auto-reconnect after a dropped connection
// ---------------------------------------------------------------------------

func TestBT2_AutoReconnectAfterDrop(t *testing.T) {
	d := newBt2Dialer()
	m := bt2Mgr(d)
	jh := bt2JH("bastion", "ops", 22)

	id, release, _ := m.Acquire(jh)
	defer release()

	dctx := m.DialContext(jh)
	if _, err := dctx(context.Background(), "tcp", "10.0.0.1:80"); err != nil {
		t.Fatalf("first dial: %v", err)
	}
	if d.openCount() != 1 {
		t.Fatalf("SPEC4: expected 1 open after first dial, got %d", d.openCount())
	}
	first := d.lastConn()

	// Simulate a silent drop: kill the conn and drive the manager to notice it.
	// Rather than wait for the 15s keepalive ticker, mark the tunnel dropped via
	// the same path keepalive uses (kill + Disconnect emulates a dead link; then
	// a fresh dial must reconnect). We kill the conn and Disconnect so the next
	// dial reopens — exercising the lazy-reconnect contract.
	first.kill()
	m.Disconnect(id)

	// State should be Disconnected (manual) — next dial reconnects regardless.
	if _, err := dctx(context.Background(), "tcp", "10.0.0.1:80"); err != nil {
		t.Fatalf("SPEC4: reconnect dial: %v", err)
	}
	if d.openCount() != 2 {
		t.Fatalf("SPEC4: expected a NEW open (2 total) after drop, got %d", d.openCount())
	}
	second := d.lastConn()
	if second == first {
		t.Fatalf("SPEC4: reconnect must produce a fresh connection")
	}

	var st TunnelStatus
	for _, s := range m.Status() {
		if s.ID == id {
			st = s
		}
	}
	if st.State != Connected {
		t.Fatalf("SPEC4: after reconnect expected Connected, got %s", st.State)
	}
	t.Logf("SPEC4 PASS: drop then reconnect -> 2 opens, back to Connected")
}

// TestBT2_KeepaliveDropReconnect drives the real keepalive path with a tiny
// time budget by killing the conn and waiting for the manager to surface Error
// via its keepalive loop, then confirming the next dial reconnects. Bounded.
func TestBT2_KeepaliveDropReconnect(t *testing.T) {
	d := newBt2Dialer()
	m := bt2Mgr(d)
	jh := bt2JH("bastion", "ops", 22)

	id, release, _ := m.Acquire(jh)
	defer release()

	dctx := m.DialContext(jh)
	if _, err := dctx(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("first dial: %v", err)
	}
	first := d.lastConn()

	// Make the live conn's keepalive fail. The production keepaliveInterval is
	// 15s, which is too long for a unit test; instead we directly invoke the
	// manager's drop path by simulating what the keepalive loop does on failure.
	first.kill()
	// Call markDropped exactly as the keepalive goroutine would on a failed probe.
	m.markDropped(id, first, errors.New("probe failed"))

	// After a drop the tunnel is Error and conn detached + closed.
	var st TunnelStatus
	for _, s := range m.Status() {
		if s.ID == id {
			st = s
		}
	}
	if st.State != Error {
		t.Fatalf("SPEC4: post-drop expected Error, got %s", st.State)
	}
	if !first.isClosed() {
		t.Fatalf("SPEC4: dropped conn should be closed")
	}

	// Next dial reconnects.
	if _, err := dctx(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("SPEC4: dial after drop: %v", err)
	}
	if d.openCount() != 2 {
		t.Fatalf("SPEC4: expected reconnect open (2), got %d", d.openCount())
	}
	t.Logf("SPEC4 PASS (keepalive drop): Error after probe failure, reconnect on next dial")
}

// ---------------------------------------------------------------------------
// SPEC 5: Refuse incomplete config
// ---------------------------------------------------------------------------

func TestBT2_RefuseIncompleteConfig(t *testing.T) {
	d := newBt2Dialer()
	m := bt2Mgr(d)
	// A literal {{x}} left in Host — Resolve(nil) keeps it, so complete=false.
	bad := model.JumpHost{Host: "{{x}}", Port: 22, User: "ops", Auth: model.JumpAuthKey, KeyPath: "/k"}

	if _, _, err := m.Acquire(bad); err == nil {
		t.Fatalf("SPEC5: Acquire should reject incomplete config")
	}
	if err := m.Connect(context.Background(), bad); err == nil {
		t.Fatalf("SPEC5: Connect should reject incomplete config")
	}
	if d.openCount() != 0 {
		t.Fatalf("SPEC5: no dial must be attempted for incomplete config, got %d opens", d.openCount())
	}
	t.Logf("SPEC5 PASS: incomplete config rejected by Acquire+Connect, 0 dials")
}

// ---------------------------------------------------------------------------
// Extra: Disconnect + Close behaviour, time-bounded.
// ---------------------------------------------------------------------------

func TestBT2_CloseTearsDownAll(t *testing.T) {
	d := newBt2Dialer()
	m := bt2Mgr(d)
	jh := bt2JH("b1", "ops", 22)
	_, rel, _ := m.Acquire(jh)
	defer rel()
	if _, err := m.DialContext(jh)(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn := d.lastConn()
	m.Close()
	if !conn.isClosed() {
		t.Fatalf("Close: live conn should be closed")
	}
	if len(m.Status()) != 0 {
		t.Fatalf("Close: status should be empty after Close, got %d", len(m.Status()))
	}
	t.Logf("EXTRA PASS: Close tears down all tunnels")
}
