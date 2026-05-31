// Command yon is a minimal, cross-platform desktop GUI for testing HTTP APIs:
// a lightweight, offline, no-login alternative to Postman, built in Go + Fyne.
//
// main wires the Fyne app to the ui controller and nothing else: per ADR-0002
// only main and internal/ui import Fyne. The ui controller restores the previous
// Session (open Collections + their Tabs) on launch and saves it on shutdown.
package main

import (
	"os"

	"fyne.io/fyne/v2/app"

	"github.com/ultramcu/yon/internal/ui"
)

func main() {
	a := app.NewWithID(ui.AppID)
	// Any .yon file paths passed on the command line are opened on launch
	// (and take precedence over restoring the previous session).
	ui.New(a).Run(os.Args[1:]...)
}
