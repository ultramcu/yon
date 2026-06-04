package tunnel

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// genHostKey returns a fresh ed25519 ssh.PublicKey for use as a host key.
func genHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	return sshPub
}

// writeKnownHosts writes a known_hosts file mapping hostport -> key.
func writeKnownHosts(t *testing.T, path, hostport string, key ssh.PublicKey) {
	t.Helper()
	line := knownhosts.Line([]string{knownhosts.Normalize(hostport)}, key)
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
}

// fakeAddr satisfies net.Addr for the host-key callback.
type fakeAddr struct{ s string }

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return a.s }

const hostport = "bastion.example.com:22"

func remoteAddr() net.Addr {
	// knownhosts resolves the address against the supplied hostname; the remote
	// addr just needs host:port form.
	host, port, _ := net.SplitHostPort(hostport)
	return fakeAddr{net.JoinHostPort(host, port)}
}

// TestKnownAndMatch: a host present in known_hosts with a matching key verifies.
func TestKnownAndMatch(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "known_hosts")
	key := genHostKey(t)
	writeKnownHosts(t, user, hostport, key)

	v := newHostKeyVerifier(user, filepath.Join(dir, "yon_kh"), RejectTOFU)
	if err := v.verify(hostport, remoteAddr(), key); err != nil {
		t.Fatalf("known+match should verify, got: %v", err)
	}
}

// TestUnknownTOFUAcceptPersists: an unknown host triggers TOFU; on accept the
// key is saved to Yon's store and re-verifies silently next time (without TOFU).
func TestUnknownTOFUAcceptPersists(t *testing.T) {
	dir := t.TempDir()
	yonKH := filepath.Join(dir, "yon", "known_hosts")
	key := genHostKey(t)

	tofuCalls := 0
	v := newHostKeyVerifier("", yonKH, func(hp, fp string) bool {
		tofuCalls++
		if !strings.HasPrefix(fp, "SHA256:") {
			t.Errorf("expected SHA256 fingerprint, got %q", fp)
		}
		return true
	})

	if err := v.verify(hostport, remoteAddr(), key); err != nil {
		t.Fatalf("TOFU-accept should verify, got: %v", err)
	}
	if tofuCalls != 1 {
		t.Fatalf("expected TOFU called once, got %d", tofuCalls)
	}
	if _, err := os.Stat(yonKH); err != nil {
		t.Fatalf("accepted key not persisted to Yon store: %v", err)
	}

	// Second verify must NOT call TOFU again (now known).
	tofuCalls = 0
	if err := v.verify(hostport, remoteAddr(), key); err != nil {
		t.Fatalf("re-verify after persist should pass, got: %v", err)
	}
	if tofuCalls != 0 {
		t.Fatalf("TOFU called again after persist (%d); should be silent", tofuCalls)
	}
}

// TestUnknownTOFUReject: rejecting the prompt fails the connection and persists
// nothing.
func TestUnknownTOFUReject(t *testing.T) {
	dir := t.TempDir()
	yonKH := filepath.Join(dir, "yon", "known_hosts")
	key := genHostKey(t)

	v := newHostKeyVerifier("", yonKH, func(hp, fp string) bool { return false })
	if err := v.verify(hostport, remoteAddr(), key); err == nil {
		t.Fatal("TOFU-reject should fail verification")
	}
	if _, err := os.Stat(yonKH); !os.IsNotExist(err) {
		t.Fatalf("rejected key should not be persisted, stat err: %v", err)
	}
}

// TestKnownMismatchHardReject: a known host presenting a DIFFERENT key is hard
// rejected WITHOUT calling TOFU (MITM signal).
func TestKnownMismatchHardReject(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "known_hosts")
	knownKey := genHostKey(t)
	writeKnownHosts(t, user, hostport, knownKey)

	otherKey := genHostKey(t) // different key presented by the server

	tofuCalled := false
	v := newHostKeyVerifier(user, filepath.Join(dir, "yon_kh"), func(hp, fp string) bool {
		tofuCalled = true
		return true
	})

	err := v.verify(hostport, remoteAddr(), otherKey)
	if err == nil {
		t.Fatal("key mismatch must be rejected")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected mismatch error, got: %v", err)
	}
	if tofuCalled {
		t.Fatal("TOFU must NOT be called on a key mismatch (hard reject)")
	}
}

// TestInsecureSkips: the per-jump-host Insecure flag bypasses all verification.
func TestInsecureSkips(t *testing.T) {
	v := newHostKeyVerifier("", "", RejectTOFU)
	cb := v.callbackFor(true)
	// InsecureIgnoreHostKey accepts any key without consulting the store.
	if err := cb(hostport, remoteAddr(), genHostKey(t)); err != nil {
		t.Fatalf("insecure should accept any key, got: %v", err)
	}
}

// TestNeverWritesUserKnownHosts: TOFU-accept persists to Yon's store only; the
// user's ~/.ssh/known_hosts is left untouched.
func TestNeverWritesUserKnownHosts(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "known_hosts")
	// Seed an unrelated host so the file exists and is read.
	other := genHostKey(t)
	writeKnownHosts(t, user, "other.host:22", other)
	userBefore, _ := os.ReadFile(user)

	yonKH := filepath.Join(dir, "yon", "known_hosts")
	v := newHostKeyVerifier(user, yonKH, func(hp, fp string) bool { return true })

	if err := v.verify(hostport, remoteAddr(), genHostKey(t)); err != nil {
		t.Fatalf("TOFU-accept: %v", err)
	}
	userAfter, _ := os.ReadFile(user)
	if string(userBefore) != string(userAfter) {
		t.Fatal("user's ~/.ssh/known_hosts must never be modified")
	}
	if _, err := os.Stat(yonKH); err != nil {
		t.Fatalf("Yon store should hold the new key: %v", err)
	}
}
