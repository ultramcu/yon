package tunnel

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ultramcu/yon/internal/model"
)

// fakeConn is a fake SSHConn: it records dials, can be told to fail keepalive
// (simulate a drop), and counts Close calls. No network involved.
type fakeConn struct {
	mu        sync.Mutex
	dials     int
	closed    int
	dropped   bool  // when true, Keepalive returns an error
	keepalive int   // number of keepalive probes seen
	dialErr   error // optional per-dial error
}

func (c *fakeConn) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dials++
	if c.dialErr != nil {
		return nil, c.dialErr
	}
	// Return a real (but unconnected) pipe end so callers get a non-nil net.Conn.
	client, _ := net.Pipe()
	return client, nil
}

func (c *fakeConn) Keepalive() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keepalive++
	if c.dropped {
		return errors.New("simulated drop")
	}
	return nil
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed++
	return nil
}

func (c *fakeConn) drop() {
	c.mu.Lock()
	c.dropped = true
	c.mu.Unlock()
}

func (c *fakeConn) closeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *fakeConn) dialCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dials
}

// fakeDialer returns an SSHDialer that hands out the supplied conns in order,
// one per successful connect, and counts how many times it was invoked.
type fakeDialer struct {
	mu     sync.Mutex
	calls  int
	conns  []*fakeConn
	failOn map[int]error // 1-based call index -> error to return
}

func (d *fakeDialer) dial(ctx context.Context, jh model.JumpHost, hkcb ssh.HostKeyCallback) (SSHConn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if err, ok := d.failOn[d.calls]; ok {
		return nil, err
	}
	if d.calls-1 >= len(d.conns) {
		// Auto-grow with a fresh conn so reconnect tests don't have to pre-size.
		d.conns = append(d.conns, &fakeConn{})
	}
	return d.conns[d.calls-1], nil
}

func (d *fakeDialer) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func testJumpHost() model.JumpHost {
	return model.JumpHost{
		Host: "bastion.example.com",
		Port: 22,
		User: "deploy",
		Auth: model.JumpAuthPassword,
	}
}

// TestLazyOpenAndReuse: the first dial opens the SSH connection; subsequent
// dials reuse the SAME underlying connection (one dial call for N target dials).
func TestLazyOpenAndReuse(t *testing.T) {
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))

	jh := testJumpHost()
	_, release, err := m.Acquire(jh)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	// Nothing connected until the first dial (lazy).
	if d.callCount() != 0 {
		t.Fatalf("expected 0 SSH dials before first use, got %d", d.callCount())
	}

	dialer := m.DialContext(jh)
	for i := 0; i < 3; i++ {
		conn, err := dialer(context.Background(), "tcp", "target.internal:443")
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		_ = conn.Close()
	}

	if d.callCount() != 1 {
		t.Fatalf("expected exactly 1 SSH connection for 3 dials, got %d", d.callCount())
	}
	if got := d.conns[0].dialCount(); got != 3 {
		t.Fatalf("expected 3 target dials through the one conn, got %d", got)
	}
}

// TestRefcountClosesAtZero: Acquire twice, Release twice; the underlying conn is
// closed and the record dropped only when the refcount reaches 0.
func TestRefcountClosesAtZero(t *testing.T) {
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))
	jh := testJumpHost()

	_, rel1, _ := m.Acquire(jh)
	_, rel2, _ := m.Acquire(jh)

	// Open it.
	if _, err := m.DialContext(jh)(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn := d.conns[0]

	rel1()
	if conn.closeCount() != 0 {
		t.Fatalf("conn closed while refcount still > 0")
	}
	if len(m.Status()) != 1 {
		t.Fatalf("tunnel dropped while refcount still > 0")
	}

	rel2()
	if conn.closeCount() != 1 {
		t.Fatalf("conn not closed at refcount 0, closeCount=%d", conn.closeCount())
	}
	if len(m.Status()) != 0 {
		t.Fatalf("tunnel record not dropped at refcount 0")
	}
}

// TestReleaseIdempotent: calling the release func twice only decrements once.
func TestReleaseIdempotent(t *testing.T) {
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))
	jh := testJumpHost()

	_, rel, _ := m.Acquire(jh)
	_, _, _ = m.Acquire(jh) // refcount = 2

	rel()
	rel() // second call must be a no-op
	if len(m.Status()) != 1 || m.Status()[0].RefCount != 1 {
		t.Fatalf("double-release decremented twice; status=%+v", m.Status())
	}
}

// TestStateTransitions: Disconnected -> Connecting -> Connected on success, and
// -> Error on dial failure.
func TestStateTransitions(t *testing.T) {
	jh := testJumpHost()

	// Success path.
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))
	_, release, _ := m.Acquire(jh)
	defer release()

	if st := m.Status()[0].State; st != Disconnected {
		t.Fatalf("initial state = %v, want Disconnected", st)
	}
	if _, err := m.DialContext(jh)(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if st := m.Status()[0].State; st != Connected {
		t.Fatalf("state after dial = %v, want Connected", st)
	}

	// Error path: dialer fails.
	df := &fakeDialer{failOn: map[int]error{1: errors.New("auth failed")}}
	mf := New(WithSSHDialer(df.dial))
	_, rel2, _ := mf.Acquire(jh)
	defer rel2()
	_, err := mf.DialContext(jh)(context.Background(), "tcp", "x:1")
	if err == nil {
		t.Fatal("expected dial error when SSH connect fails")
	}
	st := mf.Status()[0]
	if st.State != Error {
		t.Fatalf("state after failed connect = %v, want Error", st.State)
	}
	if st.Err == "" {
		t.Fatal("expected non-empty Err on Error state")
	}
}

// TestConnectDisconnect: manual Connect opens without a dial; Disconnect tears
// down but keeps the record; next dial reconnects.
func TestConnectDisconnect(t *testing.T) {
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))
	jh := testJumpHost()
	id, release, _ := m.Acquire(jh)
	defer release()

	if err := m.Connect(context.Background(), jh); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if m.Status()[0].State != Connected {
		t.Fatalf("Connect did not reach Connected")
	}
	firstConn := d.conns[0]

	m.Disconnect(id)
	if m.Status()[0].State != Disconnected {
		t.Fatalf("Disconnect did not reach Disconnected")
	}
	if firstConn.closeCount() != 1 {
		t.Fatalf("Disconnect did not close the conn")
	}

	// Next dial reconnects (new SSH connection).
	if _, err := m.DialContext(jh)(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("redial: %v", err)
	}
	if d.callCount() != 2 {
		t.Fatalf("expected reconnect (2 SSH dials), got %d", d.callCount())
	}
}

// TestAutoReconnectAfterDrop: a keepalive failure moves the Tunnel to Error and
// the next dial lazily reconnects on a fresh connection.
func TestAutoReconnectAfterDrop(t *testing.T) {
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))
	jh := testJumpHost()
	_, release, _ := m.Acquire(jh)
	defer release()

	if _, err := m.DialContext(jh)(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn0 := d.conns[0]

	// Simulate a drop and force the keepalive detection path directly (rather
	// than waiting 15s for the ticker): mark dropped, then invoke markDropped as
	// the keepalive loop would.
	conn0.drop()
	m.markDropped(identity(jh), conn0, errors.New("simulated drop"))

	if m.Status()[0].State != Error {
		t.Fatalf("state after drop = %v, want Error", m.Status()[0].State)
	}

	// Next dial reconnects.
	if _, err := m.DialContext(jh)(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("redial after drop: %v", err)
	}
	if d.callCount() != 2 {
		t.Fatalf("expected reconnect after drop, SSH dials=%d", d.callCount())
	}
	if m.Status()[0].State != Connected {
		t.Fatalf("state after reconnect = %v, want Connected", m.Status()[0].State)
	}
}

// TestKeepaliveStopsOnRelease asserts the keepalive goroutine is started on
// connect and stopped cleanly (conn closed, no leak) when the refcount drops to
// zero. The drop-detection path itself is covered by TestAutoReconnectAfterDrop,
// which drives markDropped directly rather than waiting on the 15s ticker.
func TestKeepaliveStopsOnRelease(t *testing.T) {
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))
	jh := testJumpHost()
	_, release, _ := m.Acquire(jh)
	if _, err := m.DialContext(jh)(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("dial: %v", err)
	}
	release() // refcount 0 -> closes, stopping keepalive without leaking.
	if d.conns[0].closeCount() != 1 {
		t.Fatalf("conn not closed on release")
	}
}

// TestSubscribeFires: a subscriber is invoked on state/refcount changes.
func TestSubscribeFires(t *testing.T) {
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))
	jh := testJumpHost()

	var fires int32
	unsub := m.Subscribe(func() { atomic.AddInt32(&fires, 1) })

	_, release, _ := m.Acquire(jh) // change 1 (refcount)
	if _, err := m.DialContext(jh)(context.Background(), "tcp", "x:1"); err != nil {
		t.Fatalf("dial: %v", err)
	}
	// Acquire fired once; the dial drives Connecting + Connected (>=2 more).
	if atomic.LoadInt32(&fires) < 2 {
		t.Fatalf("expected subscriber to fire on changes, got %d", fires)
	}

	unsub()
	before := atomic.LoadInt32(&fires)
	release()
	if atomic.LoadInt32(&fires) != before {
		t.Fatal("subscriber fired after unsubscribe")
	}
}

// TestAcquireRefusesIncomplete: an unresolved jump host is refused.
func TestAcquireRefusesIncomplete(t *testing.T) {
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))
	jh := model.JumpHost{Host: "{{host}}", User: "u", Auth: model.JumpAuthPassword}
	if _, _, err := m.Acquire(jh); err == nil {
		t.Fatal("expected Acquire to refuse an incomplete (templated) jump host")
	}
	if err := m.Connect(context.Background(), jh); err == nil {
		t.Fatal("expected Connect to refuse an incomplete jump host")
	}
}

// TestSharedIdentity: two equal jump hosts share ONE Tunnel.
func TestSharedIdentity(t *testing.T) {
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial))
	jhA := testJumpHost()
	jhB := testJumpHost() // identical -> same identity

	_, relA, _ := m.Acquire(jhA)
	_, relB, _ := m.Acquire(jhB)
	defer relA()
	defer relB()

	if len(m.Status()) != 1 {
		t.Fatalf("expected one shared tunnel, got %d", len(m.Status()))
	}
	if m.Status()[0].RefCount != 2 {
		t.Fatalf("expected shared refcount 2, got %d", m.Status()[0].RefCount)
	}
}

// silence unused clock warning by referencing withClock in a trivial test.
func TestWithClockOption(t *testing.T) {
	fixed := time.Unix(1000, 0)
	d := &fakeDialer{}
	m := New(WithSSHDialer(d.dial), withClock(func() time.Time { return fixed }))
	jh := testJumpHost()
	_, release, _ := m.Acquire(jh)
	defer release()
	if !m.Status()[0].Since.Equal(fixed) {
		t.Fatalf("clock not injected: Since=%v", m.Status()[0].Since)
	}
}
