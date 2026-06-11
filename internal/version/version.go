// Package version holds the app's build-time identity. Version and Commit are
// overridden at link time via -ldflags -X (by Mage for local builds and by the
// CI release jobs); the defaults below are what a plain `go build` reports.
package version

// Version is the build version, normally a `git describe --tags --always --dirty`
// value. "dev" when not injected.
var Version = "dev"

// Commit is the short commit SHA (`git rev-parse --short HEAD`). Empty when not
// injected.
var Commit = ""
