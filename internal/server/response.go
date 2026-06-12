package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Response is the uniform JSON envelope every REST endpoint returns. The
// frontend reads {data, error} the same way for every call: Data is the payload
// on success, Error is a non-empty message on failure. Exactly one is set.
type Response[T any] struct {
	Data  *T     `json:"data"`
	Error string `json:"error"`
}

// WriteJSON writes data as a successful Response[T] with the given status code.
func WriteJSON[T any](w http.ResponseWriter, status int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(Response[T]{Data: &data}); err != nil {
		slog.Default().Error("server: encode response", "err", err)
	}
}

// WriteError writes an error Response with the given status code and message.
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(Response[struct{}]{Error: msg}); err != nil {
		slog.Default().Error("server: encode error response", "err", err)
	}
}
