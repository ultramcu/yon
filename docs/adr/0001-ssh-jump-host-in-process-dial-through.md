# In-process SSH jump host (dial-through), not shelled-out `ssh` or port-forwarding

To let a Request reach a host that is only reachable from a bastion, an
Environment may define an **SSH jump host**. Yon dials each Request's TCP
connection *through* an in-process SSH connection (`golang.org/x/crypto/ssh`,
`transport.DialContext = sshClient.DialContext`) — the **Tunnel** — so the
Request keeps its real URL (no rewriting). Tunnels are owned by an app-level,
**refcounted** manager (one per distinct jump host, shared across windows),
opened **lazily** on the first send under that Environment, kept alive and
reused, and closable manually from the tunnel-status view.

Why this and not the obvious alternatives:

- **vs. shelling out to the system `ssh`** (`ssh -W` / `ProxyCommand`): rejected.
  Yon ships as a self-contained, unsigned single binary on macOS/Windows/Linux;
  a system `ssh` is not guaranteed (esp. Windows), and `ssh` prompts for
  passphrase/password on a TTY, which is unworkable to drive from a GUI. In-process
  also gives precise, object-level connection status for the live-tunnel view
  instead of parsing a child process.
- **vs. local port-forward (`ssh -L`) or SOCKS (`ssh -D`)**: rejected. A local
  forward forces the user to rewrite URLs to `localhost:NNNN`; SOCKS is heavier
  than needed. Dial-through keeps the real URL.

Auth is private-key (path + optional passphrase) or password; secret values live
in the gitignored `.env` like other Secrets.

**Consequence — proxy is bypassed under a jump host.** When a Tunnel is active the
transport's HTTP proxy (`HTTP_PROXY`/`HTTPS_PROXY`) is disabled (`transport.Proxy
= nil`); otherwise the transport would dial the proxy *through* the SSH connection.
Requests are dialed straight to their target from the jump host's network. Without
a jump host, proxy behaviour is unchanged. (Reaching the bastion itself via a local
proxy is out of scope for v1.)
