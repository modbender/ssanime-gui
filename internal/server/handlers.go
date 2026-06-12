package server

import (
	"encoding/json"
	"net/http"

	"github.com/modbender/ssanime-gui/internal/store"
)

// handleHealthz reports liveness.
func (h *Handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handlePing is a trivial round-trip check.
func (h *Handler) handlePing(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"message": "pong"})
}

// handleGetSettings returns the singleton settings row.
func (h *Handler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	set, err := h.store.Read().GetSettings(r.Context())
	if err != nil {
		h.logger.Error("server: get settings", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to read settings")
		return
	}
	WriteJSON(w, http.StatusOK, toSettingsResponse(set))
}

// handlePutSettings replaces the singleton settings row.
func (h *Handler) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req PutSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.DownloadRoot == "" || req.EncodedRoot == "" {
		WriteError(w, http.StatusBadRequest, "download_root and encoded_root required")
		return
	}
	validPolicies := map[string]bool{"delete": true, "keep": true, "move": true}
	if !validPolicies[req.CleanupPolicy] {
		WriteError(w, http.StatusBadRequest, "cleanup_policy must be delete|keep|move")
		return
	}
	set, err := h.store.Write().UpdateSettings(r.Context(), store.UpdateSettingsParams{
		DownloadRoot:        req.DownloadRoot,
		EncodedRoot:         req.EncodedRoot,
		CleanupPolicy:       req.CleanupPolicy,
		ProcessedDir:        req.ProcessedDir,
		NamingTemplate:      req.NamingTemplate,
		DownloadBackend:     req.DownloadBackend,
		DefaultProfileID:    req.DefaultProfileID,
		ConcurrencyDownload: req.ConcurrencyDownload,
		ConcurrencyEncode:   req.ConcurrencyEncode,
		FfmpegPath:          req.FfmpegPath,
		YtdlpPath:           req.YtdlpPath,
		Port:                req.Port,
		DohEnabled:          boolToInt64(req.DohEnabled),
		SetupCompleted:      boolToInt64(req.SetupCompleted),
		ShowNsfw:            boolToInt64(req.ShowNsfw),
	})
	if err != nil {
		h.logger.Error("update settings", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}
	WriteJSON(w, http.StatusOK, toSettingsResponse(set))
}
