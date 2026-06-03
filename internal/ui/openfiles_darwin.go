//go:build darwin

package ui

// On macOS a Finder double-click does NOT pass the file on the command line
// (that path, used by `yon file.yon` and by Windows/Linux, is handled in Run).
// Instead the OS sends a `kAEOpenDocuments` Apple Event, which Fyne v2 exposes
// no hook for. installOpenFilesHandler (in openfiles_darwin.m) installs an
// NSAppleEventManager handler that catches the event, resolves each item to a
// filesystem path, and calls back into yonOpenFile below.

/*
#cgo LDFLAGS: -framework Cocoa

// Defined in openfiles_darwin.m.
void installOpenFilesHandler(void);
*/
import "C"

import "sync"

// openFilesMu guards openFilesCallback, which the Apple Event handler reads from
// the main thread while registerOpenFilesHandler writes it during startup.
var (
	openFilesMu       sync.Mutex
	openFilesCallback func(string)
)

//export yonOpenFile
func yonOpenFile(cpath *C.char) {
	// Copy the C string into Go memory synchronously (the autoreleased NSString
	// backing it is only valid for the duration of this call) before handing it
	// to the callback.
	path := C.GoString(cpath)
	openFilesMu.Lock()
	cb := openFilesCallback
	openFilesMu.Unlock()
	if cb != nil {
		cb(path)
	}
}

// registerOpenFilesHandler records cb as the destination for files opened via
// the macOS "open document" Apple Event (Finder double-click, `open file.yon`,
// drag-onto-Dock-icon) and installs the underlying handler. cb is invoked once
// per file, on the Cocoa main thread, so it must marshal any UI work onto the
// Fyne loop itself (e.g. via fyne.Do). Safe to call before fyneApp.Run; the
// event is dispatched once the run loop starts.
func registerOpenFilesHandler(cb func(string)) {
	openFilesMu.Lock()
	openFilesCallback = cb
	openFilesMu.Unlock()
	C.installOpenFilesHandler()
}
