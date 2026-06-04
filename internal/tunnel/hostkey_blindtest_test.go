package tunnel

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ---------------------------------------------------------------------------
// SPEC 6: Host-key verification (ADR 0002). Unique helper prefix: hk2_*.
// ---------------------------------------------------------------------------

// hk2Key makes a fresh ed25519 ssh.PublicKey for use as a host key.
func hk2Key(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("ssh pub: %v", err)
	}
	return sshPub
}

// hk2RemoteAddr is a stub net.Addr passed to the callback (the verifier keys on
// the hostport string, not the remote addr).
type hk2RemoteAddr struct{ s string }

func (a hk2RemoteAddr) Network() string { return "tcp" }
func (a hk2RemoteAddr) String() string  { return a.s }

// hk2WriteKnownHosts writes a single known_hosts entry for hostport->key.
func hk2WriteKnownHosts(t *testing.T, path, hostport string, key ssh.PublicKey) {
	t.Helper()
	line := knownhosts.Line([]string{knownhosts.Normalize(hostport)}, key)
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
}

func hk2Verifier(t *testing.T, userKH, yonKH string, tofu TOFUFunc) *hostKeyVerifier {
	t.Helper()
	return newHostKeyVerifier(userKH, yonKH, tofu)
}

// --- known host + matching key → ok ---------------------------------------

func TestHK2_KnownMatchingKeyAccepts(t *testing.T) {
	dir := t.TempDir()
	userKH := filepath.Join(dir, "user_known_hosts")
	yonKH := filepath.Join(dir, "yon_known_hosts")
	key := hk2Key(t)
	hostport := "bastion.example.com:22"
	hk2WriteKnownHosts(t, userKH, hostport, key)

	tofuCalled := false
	v := hk2Verifier(t, userKH, yonKH, func(string, string) bool { tofuCalled = true; return false })
	cb := v.callbackFor(false)

	if err := cb(hostport, hk2RemoteAddr{hostport}, key); err != nil {
		t.Fatalf("SPEC6 known+match: expected accept, got %v", err)
	}
	if tofuCalled {
		t.Fatalf("SPEC6 known+match: TOFU must not be called for a known matching key")
	}
	t.Logf("SPEC6 PASS: known host + matching key accepted, no TOFU")
}

// --- unknown host → TOFU; accept persists to Yon store; reject errors -------

func TestHK2_UnknownHostTOFUAcceptPersists(t *testing.T) {
	dir := t.TempDir()
	userKH := filepath.Join(dir, "user_known_hosts") // intentionally absent
	yonKH := filepath.Join(dir, "yon", "known_hosts")
	key := hk2Key(t)
	hostport := "newbox.example.com:22"

	calls := 0
	v := hk2Verifier(t, userKH, yonKH, func(hp, fp string) bool {
		calls++
		if hp != knownhosts.Normalize(hostport) && hp != hostport {
			t.Fatalf("SPEC6: TOFU hostport = %q", hp)
		}
		if fp == "" {
			t.Fatalf("SPEC6: TOFU fingerprint empty")
		}
		return true // accept
	})

	// First verify: unknown -> TOFU invoked -> accept -> persist.
	if err := v.verify(hostport, hk2RemoteAddr{hostport}, key); err != nil {
		t.Fatalf("SPEC6 unknown accept: %v", err)
	}
	if calls != 1 {
		t.Fatalf("SPEC6 unknown accept: TOFU called %d times, want 1", calls)
	}

	// Yon store should now contain the key; user's known_hosts untouched.
	if _, err := os.Stat(yonKH); err != nil {
		t.Fatalf("SPEC6: accepted key not persisted to Yon store: %v", err)
	}
	if _, err := os.Stat(userKH); !os.IsNotExist(err) {
		t.Fatalf("SPEC6: user known_hosts must NEVER be written (stat err=%v)", err)
	}

	// Second verify with a FRESH verifier (so no in-memory cache): now the host
	// is known from the persisted store, so TOFU must NOT be called again.
	calls2 := 0
	v2 := hk2Verifier(t, userKH, yonKH, func(string, string) bool { calls2++; return false })
	if err := v2.verify(hostport, hk2RemoteAddr{hostport}, key); err != nil {
		t.Fatalf("SPEC6 second verify (persisted): %v", err)
	}
	if calls2 != 0 {
		t.Fatalf("SPEC6: second verify must pass WITHOUT TOFU, but it was called %d times", calls2)
	}
	t.Logf("SPEC6 PASS: unknown->TOFU->accept persisted to Yon store; reverify silent; ~/.ssh never written")
}

func TestHK2_UnknownHostTOFURejectErrors(t *testing.T) {
	dir := t.TempDir()
	userKH := filepath.Join(dir, "user_known_hosts")
	yonKH := filepath.Join(dir, "yon", "known_hosts")
	key := hk2Key(t)
	hostport := "rejectme.example.com:22"

	v := hk2Verifier(t, userKH, yonKH, func(string, string) bool { return false }) // reject
	err := v.verify(hostport, hk2RemoteAddr{hostport}, key)
	if err == nil {
		t.Fatalf("SPEC6 reject: expected error when TOFU rejects")
	}
	// Nothing should have been persisted on reject.
	if _, statErr := os.Stat(yonKH); !os.IsNotExist(statErr) {
		t.Fatalf("SPEC6 reject: nothing should be persisted on reject (stat=%v)", statErr)
	}
	t.Logf("SPEC6 PASS: TOFU reject -> error, nothing persisted")
}

// --- known host + MISMATCHED key → hard reject, TOFU NOT called ------------

func TestHK2_MismatchHardRejectsNoTOFU(t *testing.T) {
	dir := t.TempDir()
	userKH := filepath.Join(dir, "user_known_hosts")
	yonKH := filepath.Join(dir, "yon", "known_hosts")
	goodKey := hk2Key(t)
	badKey := hk2Key(t) // different key the "server" presents
	hostport := "bastion.example.com:22"
	hk2WriteKnownHosts(t, userKH, hostport, goodKey)

	tofuCalled := false
	v := hk2Verifier(t, userKH, yonKH, func(string, string) bool { tofuCalled = true; return true })

	err := v.verify(hostport, hk2RemoteAddr{hostport}, badKey)
	if err == nil {
		t.Fatalf("SPEC6 mismatch: expected hard reject for a changed key")
	}
	if tofuCalled {
		t.Fatalf("SPEC6 mismatch: TOFU must NOT be called on a key mismatch (MITM)")
	}
	if !bt2HasSubstr(err.Error(), "mismatch") {
		t.Logf("note: mismatch error text = %q", err.Error())
	}
	t.Logf("SPEC6 PASS: known host + mismatched key hard-rejected, no TOFU")
}

// --- Insecure=true → skips verification -----------------------------------

func TestHK2_InsecureSkipsVerification(t *testing.T) {
	dir := t.TempDir()
	userKH := filepath.Join(dir, "user_known_hosts")
	yonKH := filepath.Join(dir, "yon", "known_hosts")
	v := hk2Verifier(t, userKH, yonKH, RejectTOFU)

	cb := v.callbackFor(true) // insecure
	// Any key for any host must be accepted, no files consulted/written.
	if err := cb("anything:22", hk2RemoteAddr{"anything:22"}, hk2Key(t)); err != nil {
		t.Fatalf("SPEC6 insecure: expected accept, got %v", err)
	}
	if _, err := os.Stat(yonKH); !os.IsNotExist(err) {
		t.Fatalf("SPEC6 insecure: nothing should be written (stat=%v)", err)
	}
	t.Logf("SPEC6 PASS: Insecure=true skips verification, writes nothing")
}

// --- user ~/.ssh/known_hosts is never written across a full TOFU flow ------

func TestHK2_UserKnownHostsNeverWritten(t *testing.T) {
	dir := t.TempDir()
	userKH := filepath.Join(dir, "user_known_hosts")
	yonKH := filepath.Join(dir, "yon", "known_hosts")

	// Seed the user file with a known host so it EXISTS; record its bytes.
	seedKey := hk2Key(t)
	hk2WriteKnownHosts(t, userKH, "seed.example.com:22", seedKey)
	before, err := os.ReadFile(userKH)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	v := hk2Verifier(t, userKH, yonKH, func(string, string) bool { return true })
	// Accept a brand-new unknown host (would persist somewhere).
	if err := v.verify("brandnew.example.com:22", hk2RemoteAddr{"brandnew.example.com:22"}, hk2Key(t)); err != nil {
		t.Fatalf("verify new: %v", err)
	}

	after, err := os.ReadFile(userKH)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("SPEC6: user ~/.ssh/known_hosts was modified! before=%q after=%q", before, after)
	}
	// And the new key landed in Yon's store instead.
	if _, err := os.Stat(yonKH); err != nil {
		t.Fatalf("SPEC6: new key should be in Yon store: %v", err)
	}
	t.Logf("SPEC6 PASS: user known_hosts byte-identical after TOFU accept; key went to Yon store")
}

var _ net.Addr = hk2RemoteAddr{}
