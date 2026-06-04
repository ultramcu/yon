# SSH jump host key verification: known_hosts + TOFU, never touching ~/.ssh

When opening a Tunnel, Yon verifies the jump host's SSH host key as follows:
read the user's `~/.ssh/known_hosts` **read-only** plus Yon's own known-hosts
store; if the host is known and the key **matches**, connect; if the host is
**unknown**, show a trust-on-first-use prompt with the key fingerprint and, on
accept, remember it in **Yon's own store** (never writing to `~/.ssh/known_hosts`);
if the host is known but the key **mismatches**, **hard-reject** with an MITM
warning (no prompt). A per-jump-host **"Skip host key check (insecure)"** toggle
bypasses all of this, mirroring the existing *Allow insecure TLS* escape hatch.

Why this shape (it's security-sensitive and non-obvious):

- **Read but don't write `~/.ssh/known_hosts`.** A desktop app should not mutate
  the user's system SSH config (surprising, risks corrupting it, awkward to undo).
  Reading it is just a convenience since devs usually already trust their bastions.
- **TOFU only for unknown hosts; mismatch is a hard reject.** A changed key on a
  *known* host is the classic MITM signal — prompting "accept?" there would train
  users to click through real attacks.
- **Insecure toggle, not insecure-by-default.** Verification is on by default;
  the opt-out is explicit and per-jump-host, consistent with the app's TLS toggle.
