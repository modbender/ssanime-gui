package server

import (
	"net/http"

	"github.com/modbender/ssanime-gui/internal/version"
)

// VersionResponse reports the build-time version + commit injected via -ldflags.
// The raw version string is returned unchanged (any leading "v" is the
// frontend's to strip for display).
type VersionResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

func (h *Handler) handleVersion(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, VersionResponse{
		Version: version.Version,
		Commit:  version.Commit,
	})
}
