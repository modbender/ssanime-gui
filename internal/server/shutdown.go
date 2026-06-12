package server

import (
	"net/http"
)

// handleShutdown lets a newer build take over the port from an older daemon
// already running in the background. It responds 204 immediately, flushes, and
// THEN fires the graceful-shutdown callback asynchronously: calling the
// daemon's srv.Shutdown synchronously here would block on this very in-flight
// request and deadlock. The callback fires at most once (sync.Once) so repeated
// or racing POSTs can't tear down twice.
//
// The route lives under the same localGuard-protected /api group as every other
// endpoint, so a cross-origin page or a rebound Host can't trigger it.
func (h *Handler) handleShutdown(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	if h.onShutdownRequest == nil {
		return
	}
	h.shutdownOnce.Do(func() {
		go h.onShutdownRequest()
	})
}
