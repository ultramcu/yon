package ui

import (
	"github.com/ultramcu/yon/internal/model"
)

// resolveActiveJumpHost is the pure core of activeJumpHost: given an
// environment (and whether one is active) plus a {{template}} resolver, it
// returns the RESOLVED, COMPLETE jump host and true, or (zero, false) when
// there is no active environment, the environment carries no jump host, or the
// resolved config still holds unresolved variables. It takes no Window/Fyne so
// it can be exercised directly in a unit test.
func resolveActiveJumpHost(env model.Environment, ok bool, resolve func(string) string) (model.JumpHost, bool) {
	if !ok || env.JumpHost == nil {
		return model.JumpHost{}, false
	}
	resolved, complete := env.JumpHost.Resolve(resolve)
	if !complete {
		return model.JumpHost{}, false
	}
	return resolved, true
}

// activeJumpHost returns the window's active environment's resolved, complete
// jump host and true, or (zero, false) when no environment is active, the
// active environment has no jump host, or its config still carries unresolved
// {{variables}}. It adapts the window's active environment and variable scope
// into the pure resolveActiveJumpHost. Both the request editor (to inject the
// dialer) and the Tunnels window (Dev B) read this.
func (a *App) activeJumpHost(w *Window) (model.JumpHost, bool) {
	env, ok := w.activeEnv()
	return resolveActiveJumpHost(env, ok, w.varScope().Resolve)
}

// rebindTunnel keeps the window's jump-host refcount in step with its active
// environment. It releases the window's previous binding (if any) and, when a
// complete jump host is active, Acquires the matching Tunnel and stores the new
// release func. With no active jump host the window holds no reference, so a
// Tunnel nobody else uses is torn down. It is idempotent and safe to call on
// every env switch and on window close paths.
func (a *App) rebindTunnel(w *Window) {
	jh, ok := a.activeJumpHost(w)

	a.tunnelMu.Lock()
	defer a.tunnelMu.Unlock()

	// Release the window's previous binding before acquiring a new one so the old
	// Tunnel's refcount drops (and it is torn down if it was the last holder).
	if release := a.tunnelRelease[w]; release != nil {
		release()
		delete(a.tunnelRelease, w)
	}

	if !ok {
		return
	}

	// Acquire only registers a holder (the connection opens lazily on first
	// dial); a config that resolved complete should not error, but if it does we
	// simply leave the window unbound rather than crash.
	_, release, err := a.tunnels.Acquire(jh)
	if err != nil {
		return
	}
	a.tunnelRelease[w] = release
}
