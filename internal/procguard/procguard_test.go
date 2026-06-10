package procguard

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func TestReapNilProcess(t *testing.T) {
	if err := Reap(nil); err != nil {
		t.Fatalf("Reap(nil) = %v, want nil", err)
	}
}

// TestReapFinishedProcess exercises the real code path with a process that has
// already exited. On non-Windows Reap is a no-op and must return nil; on Windows
// OpenProcess of an exited pid may fail, which Reap must surface as an error
// rather than panic.
func TestReapFinishedProcess(t *testing.T) {
	cmd := exec.Command(finishedCmd())
	if err := cmd.Run(); err != nil {
		t.Skipf("could not run helper process: %v", err)
	}
	// cmd.Process is non-nil after Run; the process has exited.
	err := Reap(cmd.Process)
	if runtime.GOOS != "windows" && err != nil {
		t.Fatalf("Reap on non-windows = %v, want nil", err)
	}
}

func finishedCmd() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	if _, err := os.Stat("/bin/true"); err == nil {
		return "/bin/true"
	}
	return "true"
}
