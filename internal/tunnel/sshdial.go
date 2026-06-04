package tunnel

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ultramcu/yon/internal/model"
)

// SSHConn is the slice of an SSH client the manager needs: dial a target
// through the connection and close it. *ssh.Client satisfies this (its DialContext
// has exactly this shape), and tests provide a fake. Keeping it an interface is
// what makes the manager's lifecycle/refcount/state/keepalive unit-testable
// without a network (see the CRITICAL note in the task).
type SSHConn interface {
	// DialContext opens a TCP connection to addr THROUGH the SSH connection.
	// This is the func yonner installs as transport.DialContext.
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	// Keepalive probes that the SSH connection is still alive (the manager calls
	// it on a ~15s ticker). A non-nil error means the connection has dropped, so
	// the manager marks the Tunnel down and lazily reconnects on the next dial.
	Keepalive() error
	// Close tears down the SSH connection (and every channel dialed through it).
	Close() error
}

// SSHDialer establishes one SSH connection to a (resolved) jump host, verifying
// the host key via hostKey. It is INJECTED into the Manager so the whole
// lifecycle can be driven by a fake in tests; realSSHDialer is the production
// implementation backed by golang.org/x/crypto/ssh.
//
// The supplied jh is expected to be already variable-resolved and complete (the
// caller resolves; see model.JumpHost.Resolve). hostKey is the ssh.HostKeyCallback
// the manager built from its verifier for this jump host's Insecure setting.
type SSHDialer func(ctx context.Context, jh model.JumpHost, hostKey ssh.HostKeyCallback) (SSHConn, error)

// sshClientConn adapts *ssh.Client to SSHConn. *ssh.Client already has a
// DialContext method with the right signature, so this is a thin wrapper that
// only exists to satisfy the interface explicitly.
type sshClientConn struct{ *ssh.Client }

func (c sshClientConn) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return c.Client.DialContext(ctx, network, addr)
}

// Keepalive sends a global SSH request the server is expected to reject; a
// transport-level error means the connection has dropped. The reply payload is
// irrelevant, so a "request failed" reply (wantReply=true, ok=false) still
// counts as alive.
func (c sshClientConn) Keepalive() error {
	_, _, err := c.Client.SendRequest("keepalive@yon", true, nil)
	return err
}

// realSSHDialer is the default SSHDialer: it builds the ssh.ClientConfig from
// jh (key or password auth), dials the bastion over TCP honouring ctx, performs
// the SSH handshake with the host-key callback, and returns the live client as
// an SSHConn. Auth secrets (passphrase/password) come from jh, which the caller
// loaded from the gitignored .env.
func realSSHDialer(ctx context.Context, jh model.JumpHost, hostKey ssh.HostKeyCallback) (SSHConn, error) {
	authMethods, err := authMethodsFor(jh)
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            jh.User,
		Auth:            authMethods,
		HostKeyCallback: hostKey,
		Timeout:         30 * time.Second,
	}

	addr := net.JoinHostPort(jh.Host, strconv.Itoa(portOrDefault(jh.Port)))

	// Dial the bastion's TCP connection honouring ctx (so a Connect can be
	// cancelled / time out), then run the SSH handshake over it.
	var dialer net.Dialer
	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tunnel: dial jump host %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, cfg)
	if err != nil {
		_ = tcpConn.Close()
		return nil, fmt.Errorf("tunnel: ssh handshake with %s: %w", addr, err)
	}
	return sshClientConn{ssh.NewClient(sshConn, chans, reqs)}, nil
}

// authMethodsFor builds the SSH auth methods from the jump host config: a private
// key (with optional passphrase) for "key" auth, or a password for "password"
// auth. v1 supports no SSH agent (see ssh-jump-host.md non-goals).
func authMethodsFor(jh model.JumpHost) ([]ssh.AuthMethod, error) {
	switch jh.Auth {
	case model.JumpAuthKey:
		signer, err := signerFromKey(jh.KeyPath, jh.Passphrase)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	case model.JumpAuthPassword:
		return []ssh.AuthMethod{ssh.Password(jh.Password)}, nil
	default:
		return nil, fmt.Errorf("tunnel: unsupported jump host auth %q (want %q or %q)",
			jh.Auth, model.JumpAuthKey, model.JumpAuthPassword)
	}
}

// signerFromKey loads and parses the private key at keyPath, decrypting it with
// passphrase when the key is encrypted.
func signerFromKey(keyPath, passphrase string) (ssh.Signer, error) {
	if keyPath == "" {
		return nil, fmt.Errorf("tunnel: key auth requires a key path")
	}
	pem, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("tunnel: read private key %q: %w", keyPath, err)
	}
	if passphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase(pem, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("tunnel: parse encrypted private key %q: %w", keyPath, err)
		}
		return signer, nil
	}
	signer, err := ssh.ParsePrivateKey(pem)
	if err != nil {
		return nil, fmt.Errorf("tunnel: parse private key %q: %w", keyPath, err)
	}
	return signer, nil
}

// portOrDefault returns the jump host port, defaulting to 22 when unset.
func portOrDefault(port int) int {
	if port <= 0 {
		return 22
	}
	return port
}
