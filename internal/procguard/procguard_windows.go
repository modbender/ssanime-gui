//go:build windows

package procguard

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// job is created on the first Reap call and held open for the rest of the
// process lifetime. It is never closed on purpose: the OS closing it at process
// exit is exactly the event that fires JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE and
// reaps the children.
var (
	jobOnce sync.Once
	job     windows.Handle
	jobErr  error
)

func initJob() {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		jobErr = fmt.Errorf("create job object: %w", err)
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(h)
		jobErr = fmt.Errorf("set job object kill-on-close: %w", err)
		return
	}
	job = h
}

// Reap assigns p to a job object that the OS tears down when this (the current)
// process dies, so a force-kill of the daemon does not orphan the child.
// Best-effort: any failure is returned for logging but is non-fatal to the
// caller.
//
// When the daemon is itself already inside a job (Tauri's sidecar job, a CI
// job), the child auto-joins that outer job at creation and this adds a nested
// job; nested jobs are supported on Windows 8+ so either job's teardown reaps
// the child. On pre-Win8 single-job systems AssignProcessToJobObject returns
// ERROR_ACCESS_DENIED, which surfaces as a soft error here rather than a crash.
func Reap(p *os.Process) error {
	if p == nil {
		return nil
	}
	jobOnce.Do(initJob)
	if jobErr != nil {
		return jobErr
	}

	proc, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(p.Pid),
	)
	if err != nil {
		return fmt.Errorf("open process %d: %w", p.Pid, err)
	}
	defer windows.CloseHandle(proc)

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return fmt.Errorf("assign process %d to job (already in a single-job system): %w", p.Pid, err)
		}
		return fmt.Errorf("assign process %d to job: %w", p.Pid, err)
	}
	return nil
}
