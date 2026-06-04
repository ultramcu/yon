package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ultramcu/yon/internal/tunnel"
)

// These blind tests pin the Phase-2 UI "surfaces" lane (Dev B): the Tunnels
// window rows, the TOFU prompt body, and the footer indicator. They exercise
// ONLY pure, Fyne-free helpers and use a fixed `now` so they are deterministic
// and -race-clean. Contract symbols under test (package ui):
//
//	tunnelStatusRows(sts []tunnel.TunnelStatus, now time.Time) [][]string
//	tofuPromptMessage(hostport, fingerprint string) string
//	tunnelIndicatorText(envName string, hasJumpHost bool, state tunnel.State) (text string, show bool)

// fixedNow is a deterministic clock for all assertions in this file.
var fixedNow = time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)

// TestTunnelStatusRows pins tunnelStatusRows: empty in → empty out; rows sorted
// stably by JumpHost; each row carries State.String(), "Used by N" from
// RefCount, the Err string, and a non-empty deterministic uptime for a past
// Since.
func TestTunnelStatusRows(t *testing.T) {
	// Empty input → empty output.
	if got := tunnelStatusRows(nil, fixedNow); len(got) != 0 {
		t.Fatalf("tunnelStatusRows(nil) len = %d, want 0", len(got))
	}
	if got := tunnelStatusRows([]tunnel.TunnelStatus{}, fixedNow); len(got) != 0 {
		t.Fatalf("tunnelStatusRows([]) len = %d, want 0", len(got))
	}

	past := fixedNow.Add(-90 * time.Second)
	// Given OUT of JumpHost order; output must come back sorted by JumpHost.
	in := []tunnel.TunnelStatus{
		{
			ID:       "id-zed",
			JumpHost: "zed@host:22",
			State:    tunnel.Error,
			Err:      "dial failed: connection refused",
			Since:    past,
			RefCount: 0,
		},
		{
			ID:       "id-alpha",
			JumpHost: "alpha@host:22",
			State:    tunnel.Connected,
			Err:      "",
			Since:    past,
			RefCount: 3,
		},
		{
			ID:       "id-mid",
			JumpHost: "mid@host:22",
			State:    tunnel.Connecting,
			Err:      "",
			Since:    past,
			RefCount: 1,
		},
	}

	rows := tunnelStatusRows(in, fixedNow)
	if len(rows) != len(in) {
		t.Fatalf("tunnelStatusRows len = %d, want %d", len(rows), len(in))
	}

	// Sorted stably by JumpHost.
	wantOrder := []string{"alpha@host:22", "mid@host:22", "zed@host:22"}
	for i, want := range wantOrder {
		if len(rows[i]) == 0 {
			t.Fatalf("row %d has no cells", i)
		}
		if rows[i][0] != want {
			t.Fatalf("row %d JumpHost cell = %q, want %q (rows not sorted by JumpHost)", i, rows[i][0], want)
		}
	}

	// Every row has the same, fixed number of cells:
	// [JumpHost, State, "Used by N", uptime, Err] == 5.
	const wantCells = 5
	for i, r := range rows {
		if len(r) != wantCells {
			t.Fatalf("row %d has %d cells, want %d (%v)", i, len(r), wantCells, r)
		}
	}

	// Build a lookup by JumpHost for per-field assertions independent of order.
	byHost := map[string][]string{}
	for _, r := range rows {
		byHost[r[0]] = r
	}

	cases := []struct {
		host     string
		state    tunnel.State
		refCount int
		err      string
	}{
		{"alpha@host:22", tunnel.Connected, 3, ""},
		{"mid@host:22", tunnel.Connecting, 1, ""},
		{"zed@host:22", tunnel.Error, 0, "dial failed: connection refused"},
	}
	for _, c := range cases {
		r := byHost[c.host]
		// State cell == State.String().
		if r[1] != c.state.String() {
			t.Errorf("%s State cell = %q, want %q", c.host, r[1], c.state.String())
		}
		// "Used by N" reflects RefCount.
		if !strings.Contains(r[2], "Used by") {
			t.Errorf("%s RefCount cell = %q, want it to contain %q", c.host, r[2], "Used by")
		}
		wantN := strconv.Itoa(c.refCount)
		if !strings.Contains(r[2], wantN) {
			t.Errorf("%s RefCount cell = %q, want it to contain RefCount %q", c.host, r[2], wantN)
		}
		// uptime non-empty for a Since in the past.
		if strings.TrimSpace(r[3]) == "" {
			t.Errorf("%s uptime cell is empty, want non-empty for a past Since", c.host)
		}
		// Err cell carried (empty allowed).
		if r[4] != c.err {
			t.Errorf("%s Err cell = %q, want %q", c.host, r[4], c.err)
		}
	}

	// Uptime is deterministic for a fixed now: a second call yields identical rows.
	rows2 := tunnelStatusRows(in, fixedNow)
	for i := range rows {
		for j := range rows[i] {
			if rows[i][j] != rows2[i][j] {
				t.Fatalf("non-deterministic cell row %d col %d: %q vs %q", i, j, rows[i][j], rows2[i][j])
			}
		}
	}
}

// TestTofuPromptMessage pins tofuPromptMessage: the dialog body must contain the
// hostport AND the fingerprint, and be non-empty (a trust warning).
func TestTofuPromptMessage(t *testing.T) {
	const hostport = "bastion.example.com:2222"
	const fingerprint = "SHA256:abc123DEF456ghiJKL789mnoPQR012stuVWX345yz="

	msg := tofuPromptMessage(hostport, fingerprint)

	if strings.TrimSpace(msg) == "" {
		t.Fatalf("tofuPromptMessage returned empty message")
	}
	if !strings.Contains(msg, hostport) {
		t.Errorf("tofuPromptMessage = %q, want it to contain hostport %q", msg, hostport)
	}
	if !strings.Contains(msg, fingerprint) {
		t.Errorf("tofuPromptMessage = %q, want it to contain fingerprint %q", msg, fingerprint)
	}
}

// TestTunnelIndicatorText pins tunnelIndicatorText: hidden when there is no jump
// host; shown and env-named when there is, and visibly DISTINCT between a
// Connected tunnel and a non-Connected (Error/Disconnected) one.
func TestTunnelIndicatorText(t *testing.T) {
	const envName = "Production"

	// No jump host → hidden, empty text.
	text, show := tunnelIndicatorText(envName, false, tunnel.Connected)
	if show {
		t.Errorf("tunnelIndicatorText(hasJumpHost=false) show = true, want false")
	}
	if text != "" {
		t.Errorf("tunnelIndicatorText(hasJumpHost=false) text = %q, want empty", text)
	}

	// Has jump host, Connected → shown, contains envName.
	connText, connShow := tunnelIndicatorText(envName, true, tunnel.Connected)
	if !connShow {
		t.Errorf("tunnelIndicatorText(Connected) show = false, want true")
	}
	if !strings.Contains(connText, envName) {
		t.Errorf("tunnelIndicatorText(Connected) text = %q, want it to contain envName %q", connText, envName)
	}

	// Has jump host, Error → shown, and DISTINCT from the Connected indicator.
	errText, errShow := tunnelIndicatorText(envName, true, tunnel.Error)
	if !errShow {
		t.Errorf("tunnelIndicatorText(Error) show = false, want true")
	}
	if errText == connText {
		t.Errorf("tunnelIndicatorText(Error) = %q must differ from Connected indicator %q (state not reflected)", errText, connText)
	}

	// Has jump host, Disconnected → shown, and DISTINCT from the Connected indicator.
	discText, discShow := tunnelIndicatorText(envName, true, tunnel.Disconnected)
	if !discShow {
		t.Errorf("tunnelIndicatorText(Disconnected) show = false, want true")
	}
	if discText == connText {
		t.Errorf("tunnelIndicatorText(Disconnected) = %q must differ from Connected indicator %q (state not reflected)", discText, connText)
	}
}
