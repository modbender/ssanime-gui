package server

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/events"
)

// newShutdownServer builds a server whose OnShutdownRequest increments the
// returned counter, so tests can assert it fired exactly once.
func newShutdownServer(t *testing.T) (http.Handler, *atomic.Int32) {
	t.Helper()
	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)
	var calls atomic.Int32
	srv := New(nil, hub, nil, Config{
		OnShutdownRequest: func() { calls.Add(1) },
	})
	return srv, &calls
}

// TestShutdownReturns204AndFiresOnce asserts POST /api/shutdown answers 204 and
// invokes OnShutdownRequest exactly once even across repeated requests.
func TestShutdownReturns204AndFiresOnce(t *testing.T) {
	srv, calls := newShutdownServer(t)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
		req.Host = loopbackHost
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("request %d: status = %d, want 204", i, rec.Code)
		}
	}

	// The callback fires in a goroutine; give it a moment to run.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("OnShutdownRequest fired %d times, want exactly 1", got)
	}
}

// TestShutdownLocalGuarded asserts the endpoint is under localGuard: a
// non-loopback Host is rejected with 403 and must NOT fire the callback.
func TestShutdownRejectsForeignHost(t *testing.T) {
	srv, calls := newShutdownServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	req.Host = "anime-tools.example.com" // DNS-rebind authority
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign host: status = %d, want 403", rec.Code)
	}

	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("OnShutdownRequest fired %d times on a rejected request, want 0", got)
	}
}

// TestShutdownRejectsCrossOrigin asserts a cross-origin Origin (CSRF) is rejected.
func TestShutdownRejectsCrossOrigin(t *testing.T) {
	srv, calls := newShutdownServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	req.Host = loopbackHost
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin: status = %d, want 403", rec.Code)
	}

	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("OnShutdownRequest fired %d times on a rejected request, want 0", got)
	}
}

// TestShutdownNilCallbackSafe asserts the handler still 204s when no callback is
// wired (e.g. a server built without OnShutdownRequest).
func TestShutdownNilCallbackSafe(t *testing.T) {
	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)
	srv := New(nil, hub, nil, Config{})

	req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	req.Host = loopbackHost
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}
