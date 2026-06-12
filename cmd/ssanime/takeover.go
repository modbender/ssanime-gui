package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/cli/browser"

	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/version"
)

// probeTimeout bounds the GET /api/version probe of an already-running instance.
const probeTimeout = 800 * time.Millisecond

// takeoverWait bounds how long we poll for the old daemon's port to free after
// asking it to shut down.
const takeoverWait = 10 * time.Second

// remoteInstance is the subset of /api/version the preflight needs. The endpoint
// wraps it in the standard {data,error} envelope, so we decode that.
type remoteInstance struct {
	InstanceID string `json:"instance_id"`
	Pid        int    `json:"pid"`
}

type versionEnvelope struct {
	Data  *remoteInstance `json:"data"`
	Error string          `json:"error"`
}

// takeoverOrReopen runs the single-instance preflight BEFORE the caller binds the
// port. It returns true if the caller should proceed to start a daemon, and false
// if it has already handled the situation (reopened an identical instance, or
// failed to take over) and the process should exit.
//
// Decisions:
//   - no instance reachable        -> proceed (return true)
//   - same instance_id (reopen)    -> open the UI if appropriate, exit 0 (false)
//   - different instance_id (upgrade) -> POST /api/shutdown, wait for the port to
//     free, then proceed (true); if it never frees, exit 1 (false)
func takeoverOrReopen(cfg *config.Config, logger *slog.Logger, noOpen, headless bool) bool {
	deps := preflightDeps{
		fetchVersion:  fetchRemoteVersion,
		postShutdown:  postShutdown,
		waitPortFree:  waitPortFree,
		ourInstanceID: version.InstanceID(),
		openURL:       browser.OpenURL,
		exit:          os.Exit,
	}
	return runPreflight(cfg.Port, noOpen, headless, deps, logger)
}

// preflightDeps injects the side-effecting operations so runPreflight's decision
// logic is unit-testable without real sockets or process exits.
type preflightDeps struct {
	// fetchVersion probes the running instance. It returns (nil, nil) when no
	// instance is reachable (connection refused / timeout) and a non-nil
	// *remoteInstance when one answered.
	fetchVersion func(port int, timeout time.Duration) (*remoteInstance, error)
	// postShutdown asks the running instance to shut down.
	postShutdown func(port int) error
	// waitPortFree blocks until the port stops accepting connections or timeout.
	// It returns true once the port is free, false on timeout.
	waitPortFree func(port int, timeout time.Duration) bool
	// ourInstanceID is this process's identity.
	ourInstanceID string
	// openURL opens the existing UI on a same-identity reopen.
	openURL func(string) error
	// exit terminates the process (os.Exit in production).
	exit func(int)
}

// runPreflight is the pure decision core: it consults deps and never touches the
// network or process state directly. Returns true to proceed with startup.
func runPreflight(port int, noOpen, headless bool, deps preflightDeps, logger *slog.Logger) bool {
	remote, err := deps.fetchVersion(port, probeTimeout)
	if err != nil {
		// A reachable instance that answered malformed/garbage: treat as a foreign
		// occupant we can't identify. Don't tear down something we don't understand;
		// don't try to bind on top of it either.
		logger.Error("preflight: probe failed, port may be held by an unknown process", "err", err)
		deps.exit(1)
		return false
	}
	if remote == nil {
		// Nothing listening (or not our API): start normally.
		return true
	}

	if remote.InstanceID == deps.ourInstanceID {
		// Identical build already running: this launch is redundant. Reopen its UI
		// rather than starting a second daemon, preserving its in-flight downloads.
		logger.Info("another instance already running, reopened its UI",
			"pid", remote.Pid, "instance_id", remote.InstanceID)
		if !headless && !noOpen {
			url := fmt.Sprintf("http://127.0.0.1:%d/", port)
			if err := deps.openURL(url); err != nil {
				logger.Warn("preflight: open browser", "url", url, "err", err)
			}
		}
		deps.exit(0)
		return false
	}

	// A different build holds the port (new version or rebuilt dev binary): take
	// over. Ask the old daemon to shut down, then wait for it to release the port.
	logger.Info("different instance holds the port, taking over",
		"their_id", remote.InstanceID, "our_id", deps.ourInstanceID, "their_pid", remote.Pid)
	if err := deps.postShutdown(port); err != nil {
		logger.Error("preflight: shutdown request failed", "err", err)
		deps.exit(1)
		return false
	}
	if !deps.waitPortFree(port, takeoverWait) {
		logger.Error("preflight: old instance did not release the port in time", "timeout", takeoverWait)
		deps.exit(1)
		return false
	}
	logger.Info("old instance released the port, proceeding to start")
	return true
}

// loopbackClient is a same-host, non-browser HTTP client. It satisfies localGuard
// by sending a loopback Host header and NO Origin header (a cross-origin Origin is
// what the guard rejects on state-changing requests).
func loopbackClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// fetchRemoteVersion probes GET /api/version. A connection error (nothing
// listening) yields (nil, nil) — the normal "no instance" path. A 200 with a
// parseable body yields the remote identity. A reachable-but-unparseable response
// yields an error so the caller can refuse to clobber an unknown occupant.
func fetchRemoteVersion(port int, timeout time.Duration) (*remoteInstance, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/api/version", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// localGuard checks the Host header; 127.0.0.1 is loopback. No Origin header is
	// set (GET is not state-changing anyway), so the guard passes us through.
	req.Host = fmt.Sprintf("127.0.0.1:%d", port)

	resp, err := loopbackClient(timeout).Do(req)
	if err != nil {
		// Connection refused / timeout / DNS: no instance answering. Not an error
		// from the preflight's perspective.
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("version probe: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("version probe: read body: %w", err)
	}
	var env versionEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("version probe: decode: %w", err)
	}
	if env.Data == nil || env.Data.InstanceID == "" {
		return nil, fmt.Errorf("version probe: missing instance_id")
	}
	return env.Data, nil
}

// postShutdown asks the running daemon to shut down via POST /api/shutdown. The
// request satisfies localGuard the same way: loopback Host, no cross-origin
// Origin header.
func postShutdown(port int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/api/shutdown", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Host = fmt.Sprintf("127.0.0.1:%d", port)

	resp, err := loopbackClient(5 * time.Second).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("shutdown request: status %d", resp.StatusCode)
	}
	return nil
}

// waitPortFree polls tcp/127.0.0.1:<port> until a dial fails (port released) or
// the timeout elapses. Returns true once the port is free.
func waitPortFree(port int, timeout time.Duration) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return true
		}
		conn.Close()
		time.Sleep(150 * time.Millisecond)
	}
	// One final check after the loop in case the last sleep straddled the deadline.
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return true
	}
	conn.Close()
	return false
}
