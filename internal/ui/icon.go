package ui

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

// iconPNG is the Yon app icon (the dart-Y mark), embedded so the binary is
// self-contained with no external asset at runtime.
//
//go:embed icon.png
var iconPNG []byte

// appIcon is the embedded icon as a Fyne resource, set on the app and on each
// Collection window.
var appIcon = fyne.NewStaticResource("yon.png", iconPNG)
