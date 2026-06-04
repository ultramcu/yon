# Changelog

All notable changes to Yon are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Yon adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.12.0] - 2026-06-04

### Added
- **SSH jump host per Environment** ([#21]). An Environment can define an SSH
  jump host; when it is active, Yon dials every Request through one in-process
  SSH Tunnel (`golang.org/x/crypto/ssh`) — the URL is never rewritten.
  - Environment manager gains an *SSH jump host* section: host / port / user,
    private-key (path + passphrase) or password auth, and a *Skip host key
    check* toggle. Passphrase and password are stored as Secrets in the
    gitignored `.env`, never in the committed Collection.
  - **Tunnels window** (`Collection ▸ Tunnels…`): a live table of every Tunnel
    (state, refcount, uptime, last error) with a Disconnect button; the next
    send reconnects automatically.
  - **Footer indicator** beside the version: a coloured dot and the active
    environment's name when a jump host is configured; click it to open the
    Tunnels window.
  - **Host-key verification (trust on first use):** an unknown host shows a
    fingerprint confirmation, and a changed key on a known host is hard-rejected.
    Yon reads `~/.ssh/known_hosts` read-only and never writes to it.

### Notes
- Jump-host fields are resolved against the active Environment's variables; an
  incomplete config (an unresolved `{{var}}`) never dials.
- The HTTP proxy is bypassed while a jump host is active.
- No external `ssh` binary is required — Yon stays a single download.

[0.12.0]: https://github.com/ultramcu/yon/releases/tag/v0.12.0
[#21]: https://github.com/ultramcu/yon/issues/21
