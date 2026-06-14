//go:build !windows

package encode

import "os/exec"

// hideConsole is a no-op off Windows: there is no console window to suppress.
func hideConsole(*exec.Cmd) {}
