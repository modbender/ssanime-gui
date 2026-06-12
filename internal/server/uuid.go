package server

import "github.com/google/uuid"

// mustUUID returns a new random UUID string.
func mustUUID() string { return uuid.NewString() }
