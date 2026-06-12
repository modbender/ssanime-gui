package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// handleEvents streams the events hub to one client over Server-Sent Events. It
// subscribes to the hub, writes each Event as an "event:/data:" frame, flushes
// after every frame, and exits when the client disconnects, the subscriber is
// dropped for being slow, or the request context is cancelled.
func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering
	w.WriteHeader(http.StatusOK)

	sub := h.hub.Subscribe()
	defer sub.Close()

	// Initial comment line opens the stream so clients connect immediately.
	fmt.Fprint(w, ":ok\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Dropped():
			return
		case ev, open := <-sub.Events():
			if !open {
				return
			}
			payload, err := json.Marshal(ev.Data)
			if err != nil {
				h.logger.Error("server: marshal SSE event", "type", ev.Type, "err", err)
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
