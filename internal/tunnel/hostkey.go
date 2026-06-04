package tunnel

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// TOFUFunc is the trust-on-first-use prompt for an UNKNOWN host key (ADR 0002).
// It is called only when the host appears in NEITHER the user's
// ~/.ssh/known_hosts NOR Yon's own store; the implementation (a UI dialog in
// Phase 2, or a policy in headless use) returns true to accept-and-remember the
// key or false to reject the connection. It is NEVER called on a key MISMATCH —
// a changed key on a known host is the classic MITM signal and is hard-rejected
// without a prompt.
type TOFUFunc func(hostport, fingerprint string) bool

// RejectTOFU is the safe default TOFU policy for headless use: it refuses every
// unknown host. The Phase 2 UI replaces this with an interactive prompt.
func RejectTOFU(hostport, fingerprint string) bool { return false }

// hostKeyVerifier implements the ADR 0002 host-key policy as an
// ssh.HostKeyCallback factory. It is constructed per dial (the callback closes
// over the dial's target hostport) but the underlying known-hosts files and
// TOFU callback are shared and injectable, so the whole policy is unit-testable
// without a network or the real ~/.ssh.
//
// Lookup order on every connect:
//  1. Read the user's ~/.ssh/known_hosts (READ-ONLY) and Yon's own store.
//  2. host known + key matches      → accept.
//  3. host known + key mismatches   → hard reject (no prompt; possible MITM).
//  4. host unknown                  → call TOFU; on accept, persist the key to
//     Yon's store (never to ~/.ssh) so the next connect verifies silently.
//
// A per-jump-host Insecure flag bypasses all of this (handled by the caller via
// callbackFor, mirroring the app's Allow-insecure-TLS escape hatch).
type hostKeyVerifier struct {
	// userKnownHosts is the user's ~/.ssh/known_hosts, read but never written.
	// Empty disables the read-only convenience lookup (used in tests).
	userKnownHosts string
	// yonKnownHosts is Yon's own store in the app config dir; the only file this
	// verifier ever writes to (on TOFU-accept).
	yonKnownHosts string
	// tofu decides unknown hosts; never called on mismatch. nil → RejectTOFU.
	tofu TOFUFunc

	// mu guards concurrent appends to yonKnownHosts from parallel dials.
	mu sync.Mutex
}

// newHostKeyVerifier builds a verifier from injectable paths and a TOFU
// callback. Either path may be empty (skipped). A nil tofu defaults to
// RejectTOFU.
func newHostKeyVerifier(userKnownHosts, yonKnownHosts string, tofu TOFUFunc) *hostKeyVerifier {
	if tofu == nil {
		tofu = RejectTOFU
	}
	return &hostKeyVerifier{
		userKnownHosts: userKnownHosts,
		yonKnownHosts:  yonKnownHosts,
		tofu:           tofu,
	}
}

// callbackFor returns the ssh.HostKeyCallback to use for a connection to
// hostport (e.g. "bastion.example.com:22"). When insecure is true it returns
// ssh.InsecureIgnoreHostKey() — the explicit per-jump-host opt-out — and the
// known-hosts files and TOFU are never consulted.
func (v *hostKeyVerifier) callbackFor(insecure bool) ssh.HostKeyCallback {
	if insecure {
		return ssh.InsecureIgnoreHostKey() //nolint:gosec // opt-in per-jump-host toggle (ADR 0002)
	}
	return v.verify
}

// verify is the ssh.HostKeyCallback implementing the policy above. It is called
// by the SSH handshake with the address dialed and the key the server
// presented.
func (v *hostKeyVerifier) verify(hostport string, remote net.Addr, key ssh.PublicKey) error {
	// Build a combined known-hosts callback over whichever of the two stores
	// currently exist. Missing files are simply skipped (knownhosts.New errors
	// on a missing file, so we filter first); an empty set means "unknown".
	known, err := v.knownHostsCallback()
	if err != nil {
		return err
	}

	var checkErr error
	if known != nil {
		checkErr = known(hostport, remote, key)
		if checkErr == nil {
			return nil // known + match → accept.
		}
	} else {
		// No store at all yet → treat as unknown so TOFU can bootstrap it.
		checkErr = &knownhosts.KeyError{}
	}

	// Distinguish UNKNOWN host from key MISMATCH. knownhosts.KeyError.Want is
	// empty for an unknown host and non-empty (the accepted keys) for a
	// mismatch — the latter is a hard reject without a prompt (ADR 0002).
	var keyErr *knownhosts.KeyError
	if errors.As(checkErr, &keyErr) {
		if len(keyErr.Want) > 0 {
			return fmt.Errorf("tunnel: host key mismatch for %s (possible MITM); refusing to connect: %w", hostport, checkErr)
		}
		// Unknown host → trust-on-first-use prompt.
		return v.handleUnknown(hostport, key)
	}

	// Any other error (revoked key, parse failure) is fatal.
	return checkErr
}

// handleUnknown runs the TOFU prompt for an unknown host and, on accept,
// persists the key to Yon's own store so future connects verify silently.
func (v *hostKeyVerifier) handleUnknown(hostport string, key ssh.PublicKey) error {
	fingerprint := ssh.FingerprintSHA256(key)
	if !v.tofu(hostport, fingerprint) {
		return fmt.Errorf("tunnel: host key for %s was not trusted (%s)", hostport, fingerprint)
	}
	if err := v.persist(hostport, key); err != nil {
		return fmt.Errorf("tunnel: trusted %s but could not save host key: %w", hostport, err)
	}
	return nil
}

// knownHostsCallback returns an ssh.HostKeyCallback over the existing
// known-hosts files (user's read-only file + Yon's store). It returns (nil, nil)
// when neither file exists yet — the caller treats that as "host unknown".
func (v *hostKeyVerifier) knownHostsCallback() (ssh.HostKeyCallback, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	var files []string
	for _, f := range []string{v.userKnownHosts, v.yonKnownHosts} {
		if f == "" {
			continue
		}
		if _, err := os.Stat(f); err == nil {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	cb, err := knownhosts.New(files...)
	if err != nil {
		return nil, fmt.Errorf("tunnel: read known_hosts: %w", err)
	}
	return cb, nil
}

// persist appends the accepted host key to Yon's own store (creating it and any
// parent dirs as needed). It NEVER writes to the user's ~/.ssh/known_hosts.
func (v *hostKeyVerifier) persist(hostport string, key ssh.PublicKey) error {
	if v.yonKnownHosts == "" {
		return errors.New("no Yon known-hosts store configured")
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(v.yonKnownHosts), 0o700); err != nil {
		return err
	}

	// knownhosts.Line normalizes the address and serializes a valid OpenSSH
	// known_hosts entry for this key.
	line := knownhosts.Line([]string{knownhosts.Normalize(hostport)}, key)

	f, err := os.OpenFile(v.yonKnownHosts, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return err
	}
	return nil
}

// defaultUserKnownHosts returns the conventional ~/.ssh/known_hosts path, or ""
// if the home dir can't be resolved (the read-only convenience is then skipped).
func defaultUserKnownHosts() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

// defaultYonKnownHosts returns Yon's own known-hosts store under the user config
// dir, or "" if it can't be resolved.
func defaultYonKnownHosts() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "yon", "known_hosts")
}
