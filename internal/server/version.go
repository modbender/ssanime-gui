package server

import (
	"net/http"
	"os"

	"github.com/modbender/ssanime-gui/internal/version"
)

// VersionResponse reports the build-time version + commit injected via -ldflags,
// plus the running instance's identity. The raw version string is returned
// unchanged (any leading "v" is the frontend's to strip for display).
//
// InstanceID distinguishes this exact build (down to a rebuilt dev binary) so a
// launching process can decide between "reopen the same instance" and "take over
// a different build"; Pid is informational.
type VersionResponse struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	InstanceID string `json:"instance_id"`
	Pid        int    `json:"pid"`
}

func (h *Handler) handleVersion(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, VersionResponse{
		Version:    version.Version,
		Commit:     version.Commit,
		InstanceID: version.InstanceID(),
		Pid:        os.Getpid(),
	})
}
