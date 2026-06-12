package main

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeDeps builds preflightDeps backed by simple closures and records the exit
// code (or -1 if exit was never called) plus whether shutdown/openURL fired.
type preflightResult struct {
	exitCode      int // -1 = exit not called
	shutdownFired bool
	openFired     bool
	proceed       bool
}

func runWith(t *testing.T, noOpen, headless bool, fetch func(int, time.Duration) (*remoteInstance, error),
	ourID string, portFree bool) preflightResult {
	t.Helper()
	res := preflightResult{exitCode: -1}
	deps := preflightDeps{
		fetchVersion:  fetch,
		ourInstanceID: ourID,
		postShutdown:  func(int) error { res.shutdownFired = true; return nil },
		waitPortFree:  func(int, time.Duration) bool { return portFree },
		openURL:       func(string) error { res.openFired = true; return nil },
		exit:          func(code int) { res.exitCode = code },
	}
	res.proceed = runPreflight(4773, noOpen, headless, deps, testLogger())
	return res
}

// TestPreflightNoInstance: probe returns (nil,nil) -> proceed, no exit.
func TestPreflightNoInstance(t *testing.T) {
	res := runWith(t, false, false,
		func(int, time.Duration) (*remoteInstance, error) { return nil, nil },
		"our-id", true)
	if !res.proceed {
		t.Fatal("want proceed=true when no instance is running")
	}
	if res.exitCode != -1 {
		t.Fatalf("exit called with %d, want no exit", res.exitCode)
	}
	if res.shutdownFired {
		t.Error("shutdown should not fire when no instance is running")
	}
}

// TestPreflightSameIDReopen: identical instance_id -> reopen UI, exit 0, no daemon.
func TestPreflightSameIDReopen(t *testing.T) {
	res := runWith(t, false, false,
		func(int, time.Duration) (*remoteInstance, error) {
			return &remoteInstance{InstanceID: "same-id", Pid: 100}, nil
		},
		"same-id", true)
	if res.proceed {
		t.Fatal("want proceed=false on a same-id reopen")
	}
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.exitCode)
	}
	if !res.openFired {
		t.Error("want the UI to be reopened on same-id (not headless, not no-open)")
	}
	if res.shutdownFired {
		t.Error("shutdown must NOT fire on a same-id reopen")
	}
}

// TestPreflightSameIDHeadlessNoOpen: headless reopen exits 0 without opening a browser.
func TestPreflightSameIDHeadless(t *testing.T) {
	res := runWith(t, true, true,
		func(int, time.Duration) (*remoteInstance, error) {
			return &remoteInstance{InstanceID: "same-id", Pid: 100}, nil
		},
		"same-id", true)
	if res.proceed {
		t.Fatal("want proceed=false on a same-id reopen")
	}
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.exitCode)
	}
	if res.openFired {
		t.Error("must NOT open a browser in headless reopen")
	}
}

// TestPreflightDifferentIDTakeover: different instance_id -> shutdown + port frees -> proceed.
func TestPreflightDifferentIDTakeover(t *testing.T) {
	res := runWith(t, false, false,
		func(int, time.Duration) (*remoteInstance, error) {
			return &remoteInstance{InstanceID: "old-build", Pid: 100}, nil
		},
		"new-build", true)
	if !res.proceed {
		t.Fatal("want proceed=true after a successful takeover")
	}
	if !res.shutdownFired {
		t.Error("shutdown must fire on a different-id takeover")
	}
	if res.exitCode != -1 {
		t.Fatalf("exit called with %d, want no exit on a successful takeover", res.exitCode)
	}
}

// TestPreflightTakeoverPortNeverFrees: different id but the port never frees -> exit 1.
func TestPreflightTakeoverPortNeverFrees(t *testing.T) {
	res := runWith(t, false, false,
		func(int, time.Duration) (*remoteInstance, error) {
			return &remoteInstance{InstanceID: "old-build", Pid: 100}, nil
		},
		"new-build", false)
	if res.proceed {
		t.Fatal("want proceed=false when the port never frees")
	}
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.exitCode)
	}
}

// TestPreflightProbeError: a reachable-but-unparseable occupant -> exit 1, don't clobber.
func TestPreflightProbeError(t *testing.T) {
	res := runWith(t, false, false,
		func(int, time.Duration) (*remoteInstance, error) {
			return nil, io.ErrUnexpectedEOF
		},
		"our-id", true)
	if res.proceed {
		t.Fatal("want proceed=false on a probe error")
	}
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", res.exitCode)
	}
	if res.shutdownFired {
		t.Error("must NOT send shutdown to an unidentified occupant")
	}
}
