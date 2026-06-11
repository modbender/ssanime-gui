package server

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
)

// parseID extracts and parses the {id} URL parameter. Returns false and writes
// a 400 error if the param is missing or not a positive integer.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	if raw == "" {
		WriteError(w, http.StatusBadRequest, "missing id")
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// parseAnilistID extracts the {id} URL parameter as an int (an AniList id, not a
// series row id). Returns false and a 400 if missing or not a positive integer.
func parseAnilistID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, ok := parseID(w, r)
	if !ok {
		return 0, false
	}
	return int(id), true
}

// boolToInt64 converts Go bool to the SQLite integer sqlc uses for boolean cols.
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// RingBuffer is a bounded in-memory circular log buffer. Safe for concurrent use.
type RingBuffer struct {
	mu   sync.RWMutex
	buf  []string
	cap  int
	head int
	size int
}

// NewRingBuffer creates a RingBuffer that holds at most n lines.
func NewRingBuffer(n int) *RingBuffer {
	return &RingBuffer{buf: make([]string, n), cap: n}
}

// Write appends a log line, evicting the oldest when full.
func (rb *RingBuffer) Write(line string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.buf[rb.head] = line
	rb.head = (rb.head + 1) % rb.cap
	if rb.size < rb.cap {
		rb.size++
	}
}

// Lines returns up to limit recent lines (newest last). limit=0 means all.
func (rb *RingBuffer) Lines(limit int) []string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	n := rb.size
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		// walk from oldest-within-window forward
		idx := (rb.head - rb.size + rb.cap + (rb.size - n) + i) % rb.cap
		out[i] = rb.buf[idx]
	}
	return out
}
