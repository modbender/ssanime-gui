//go:build !windows

package procguard

import "os"

// Reap assigns p to a process-management group that the OS tears down when this
// (the current) process dies, so a force-kill of the daemon does not orphan the
// child. Best-effort: any failure is returned for logging but is non-fatal to
// the caller.
//
// No-op on non-Windows: there is no portable equivalent to a Windows job object
// wired up yet, so this returns nil.
func Reap(p *os.Process) error { return nil }
