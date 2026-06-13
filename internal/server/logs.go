package server

import (
	"net/http"
	"strconv"
)

func (h *Handler) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			limit = n
		}
	}
	lines := []string{}
	if h.logs != nil {
		if got := h.logs.Lines(limit); got != nil {
			lines = got
		}
	}
	WriteJSON(w, http.StatusOK, LogsResponse{Lines: lines})
}
