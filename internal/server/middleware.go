package server

import (
	"net/http"
	"strconv"

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
