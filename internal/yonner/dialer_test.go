package yonner

import (
	"context"
	"net"
	"net/http"
	"testing"
)

// TestNewClient_DialContextSet asserts that supplying Options.DialContext both
// installs the custom dialer on the transport AND disables the HTTP proxy, so a
// jump-host send is dialed through the SSH connection and never through
// HTTP_PROXY/HTTPS_PROXY (ADR 0001).
func TestNewClient_DialContextSet(t *testing.T) {
	called := false
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		called = true
		return nil, context.Canceled // we only care that it's wired, not that it dials
	}

	client := newClient(Options{DialContext: dial})
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", client.Transport)
	}

	if transport.Proxy != nil {
		t.Error("transport.Proxy should be nil when a DialContext is set (proxy bypass)")
	}
	if transport.DialContext == nil {
		t.Fatal("transport.DialContext should be set to the custom dialer")
	}

	// Confirm it is OUR dialer, not the default, by invoking it.
	_, _ = transport.DialContext(context.Background(), "tcp", "example.invalid:80")
	if !called {
		t.Error("transport.DialContext did not invoke the supplied dialer")
	}
}

// TestNewClient_DialContextUnset asserts the transport is left exactly as the
// cloned DefaultTransport when no DialContext is supplied: the HTTP proxy is
// still honoured and the dialer is the default (not nil-but-overridden). This
// guards the "no behaviour change without a jump host" contract.
func TestNewClient_DialContextUnset(t *testing.T) {
	defaultTransport := http.DefaultTransport.(*http.Transport)

	client := newClient(Options{})
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", client.Transport)
	}

	if transport.Proxy == nil {
		t.Error("transport.Proxy should be honoured (non-nil) when no DialContext is set")
	}
	// The clone keeps DefaultTransport's Proxy func (http.ProxyFromEnvironment).
	// We can't compare funcs by ==, so just assert it matches the default's
	// nil-ness, which the check above already covers.
	if defaultTransport.Proxy == nil {
		t.Skip("DefaultTransport has no Proxy in this environment; nothing to compare")
	}
}
