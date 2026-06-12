// Package tray provides shared types for the system-tray integration.
//
// The actual tray is wired directly in cmd/ssanime/main.go using fyne.io/systray,
// which must be called from the main goroutine. This package exposes the Pausable
// interface so the queue references can be typed without importing download/encode
// from the tray layer.
package tray

// Pausable is implemented by both download.Queue and encode.Queue.
// The tray menu's "Pause all" / "Resume all" toggle calls these.
type Pausable interface {
	Pause()
	Resume()
	Paused() bool
}
