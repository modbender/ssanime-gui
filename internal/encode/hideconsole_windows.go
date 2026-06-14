//go:build windows

package encode

import (
	"os/exec"
	"syscall"
)

// createNoWindow (CREATE_NO_WINDOW) stops Windows from allocating a console for
// a console subsystem child. Without it, a GUI-launched daemon spawning ffmpeg
// (a console app) gets a fresh terminal window popped up — and closing that
// window kills the encode, since it is the child's controlling console.
const createNoWindow = 0x08000000

// hideConsole suppresses the child console window for an ffmpeg/ffprobe spawn.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
