package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// tunnelTOFUPrompt is the trust-on-first-use host-key callback handed to the
// tunnel Manager via tunnel.WithTOFU. It is invoked from a NON-UI goroutine
// (during an SSH dial) and MUST block until the user answers, returning true to
// trust the unknown host key (and persist it) or false to reject the dial.
//
// Because the SSH handshake runs off the Fyne goroutine, the confirm dialog is
// scheduled with fyne.Do and the answer is funnelled back over a buffered
// channel that this function then blocks on. If no window is open there is
// nothing to anchor the prompt to, so we reject (never auto-accept headless —
// matching tunnel.RejectTOFU's default).
func (a *App) tunnelTOFUPrompt(hostport, fingerprint string) bool {
	parent := a.anyWindow()
	if parent == nil {
		return false
	}

	ch := make(chan bool, 1)
	fyne.Do(func() {
		dialog.ShowConfirm(
			"Unknown SSH host key",
			tofuPromptMessage(hostport, fingerprint),
			func(ok bool) { ch <- ok },
			parent.win,
		)
	})
	return <-ch
}

// tofuPromptMessage builds the body text for the trust-on-first-use prompt. It
// is deliberately Fyne-free so the host-key warning copy can be unit-tested
// without a GUI.
func tofuPromptMessage(hostport, fingerprint string) string {
	return fmt.Sprintf(
		"The authenticity of host %q can't be established.\n\n"+
			"Key fingerprint:\n%s\n\n"+
			"Only accept if you trust this host. Accepting permanently trusts this "+
			"key; a later mismatch will be refused as a possible man-in-the-middle "+
			"attack.\n\nDo you want to trust and connect?",
		hostport, fingerprint,
	)
}
