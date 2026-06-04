package tunnel

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ultramcu/yon/internal/model"
)

// rwcConn is a trivial SSHConn for the ctx-cancellation edge-case test.
type rwcConn struct{}

func (rwcConn) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c, _ := net.Pipe()
	return c, nil
}
func (rwcConn) Keepalive() error { return nil }
func (rwcConn) Close() error     { return nil }

// rwcDialer blocks inside dial (signalling entry) so a second caller is forced
// onto the in-flight waiter path while the first holds the gate.
type rwcDialer struct {
	calls   int32
	entered chan struct{}
	gate    chan struct{}
}

func newRWCDialer() *rwcDialer {
	return &rwcDialer{entered: make(chan struct{}, 8), gate: make(chan struct{})}
}

func (d *rwcDialer) dial(ctx context.Context, jh model.JumpHost, hk ssh.HostKeyCallback) (SSHConn, error) {
	atomic.AddInt32(&d.calls, 1)
	d.entered <- struct{}{}
	<-d.gate
	return rwcConn{}, nil
}

func (d *rwcDialer) callCount() int { return int(atomic.LoadInt32(&d.calls)) }

// TestEnsure_WaiterCtxCancel covers the dedup edge case the blind suite omits: a
// goroutine WAITING on an in-flight dial whose ctx is cancelled must return
// ctx.Err() promptly (no goroutine leak, no second dial), while the in-flight
// dialer finishes undisturbed.
func TestEnsure_WaiterCtxCancel(t *testing.T) {
	d := newRWCDialer()
	m := New(WithSSHDialer(d.dial))
	jh := testJumpHost()
	_, release, _ := m.Acquire(jh)
	defer release()

	dialer := m.DialContext(jh)

	// Goroutine 1 becomes the dialer and blocks inside dial.
	firstDone := make(chan struct{})
	go func() {
		_, _ = dialer(context.Background(), "tcp", "x:1")
		close(firstDone)
	}()
	<-d.entered // first dialer is inside (blocked on the gate)

	// Goroutine 2 finds the in-flight gate and waits — but its ctx is cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	waiterErr := make(chan error, 1)
	go func() {
		_, err := dialer(ctx, "tcp", "x:1")
		waiterErr <- err
	}()
	time.Sleep(20 * time.Millisecond) // let the waiter park on the wait channel
	cancel()

	select {
	case err := <-waiterErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled waiter err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter did not return promptly (possible leak/deadlock)")
	}

	if d.callCount() != 1 {
		t.Fatalf("waiter must not have dialed: dial count = %d, want 1", d.callCount())
	}

	// The in-flight dialer finishes undisturbed once released.
	close(d.gate)
	<-firstDone
	if m.Status()[0].State != Connected {
		t.Fatalf("after dialer finished, state = %v, want Connected", m.Status()[0].State)
	}
}
