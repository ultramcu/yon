//go:build !darwin

package ui

// On Windows and Linux a double-clicked file is delivered as a command-line
// argument (Windows shell association, Linux .desktop `Exec=yon %F`), which Run
// already opens — so there is no OS event to hook here. registerOpenFilesHandler
// is a no-op to keep app.go platform-independent.
func registerOpenFilesHandler(cb func(string)) {}
