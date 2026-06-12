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
	lines := h.logs.Lines(limit)
	if lines == nil {
		lines = []string{}
	}
	WriteJSON(w, http.StatusOK, LogsResponse{Lines: lines})
}
