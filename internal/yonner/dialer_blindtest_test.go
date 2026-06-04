package yonner

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ultramcu/yon/internal/model"
)

// ---------------------------------------------------------------------------
// SPEC 7: yonner proxy-bypass. Unique helper prefix: dl2_*.
// ---------------------------------------------------------------------------

// dl2CountingDialer wraps a real dialer and counts how many times it is used.
type dl2CountingDialer struct {
	calls int32
}

func (d *dl2CountingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	atomic.AddInt32(&d.calls, 1)
	var nd net.Dialer
	return nd.DialContext(ctx, network, addr)
}

func (d *dl2CountingDialer) count() int { return int(atomic.LoadInt32(&d.calls)) }

// --- Transport inspection: DialContext set => custom dialer + Proxy nil ----

func TestDL2_CustomDialerDisablesProxy(t *testing.T) {
	dialer := &dl2CountingDialer{}
	opts := Options{
		Timeout:     5 * time.Second,
		DialContext: dialer.DialContext,
	}
	c := newClient(opts)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("SPEC7: transport is not *http.Transport (%T)", c.Transport)
	}
	if tr.Proxy != nil {
		t.Fatalf("SPEC7: transport.Proxy must be nil when DialContext is set")
	}
	if tr.DialContext == nil {
		t.Fatalf("SPEC7: transport.DialContext must be set")
	}
	t.Logf("SPEC7 PASS (inspection): DialContext set -> Proxy nil, DialContext installed")
}

// --- Transport inspection: DialContext unset => default proxy honored ------

func TestDL2_NoCustomDialerKeepsProxy(t *testing.T) {
	opts := Options{Timeout: 5 * time.Second} // DialContext nil
	c := newClient(opts)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("SPEC7: transport is not *http.Transport (%T)", c.Transport)
	}
	// http.DefaultTransport.Clone() preserves the env-proxy func; it must remain.
	if tr.Proxy == nil {
		t.Fatalf("SPEC7: transport.Proxy must be honored (non-nil) when DialContext unset")
	}
	t.Logf("SPEC7 PASS (inspection): DialContext unset -> Proxy preserved (default transport)")
}

// --- Drive a real request through the custom dialer to a test server -------

func TestDL2_RequestUsesCustomDialer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "brewed")
	}))
	defer srv.Close()

	dialer := &dl2CountingDialer{}
	opts := Options{
		Timeout:         5 * time.Second,
		FollowRedirects: true,
		DialContext:     dialer.DialContext,
	}

	req := model.Request{Method: "GET", URL: srv.URL}
	resp, err := Send(context.Background(), req, model.Collection{}, opts)
	if err != nil {
		t.Fatalf("SPEC7: Send via custom dialer: %v", err)
	}
	if resp.Status != http.StatusTeapot {
		t.Fatalf("SPEC7: status = %d, want 418", resp.Status)
	}
	if string(resp.Body) != "brewed" {
		t.Fatalf("SPEC7: body = %q, want brewed", resp.Body)
	}
	if dialer.count() == 0 {
		t.Fatalf("SPEC7: custom dialer was NOT used (count=0)")
	}
	t.Logf("SPEC7 PASS (live): request went through custom dialer (%d dial(s)), got 418", dialer.count())
}

// --- With DialContext unset, the custom dialer is never used ----------------

func TestDL2_RequestSkipsCustomDialerWhenUnset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dialer := &dl2CountingDialer{} // built but NOT installed in Options
	opts := Options{Timeout: 5 * time.Second, FollowRedirects: true}

	req := model.Request{Method: "GET", URL: srv.URL}
	if _, err := Send(context.Background(), req, model.Collection{}, opts); err != nil {
		t.Fatalf("SPEC7: Send default: %v", err)
	}
	if dialer.count() != 0 {
		t.Fatalf("SPEC7: custom dialer must NOT be used when DialContext unset, count=%d", dialer.count())
	}
	t.Logf("SPEC7 PASS (live): default transport used, custom dialer untouched")
}
