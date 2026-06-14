package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/modbender/ssanime-gui/internal/extension"
)

func (h *Handler) handleListExtensionRepos(w http.ResponseWriter, r *http.Request) {
	if h.extMgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension manager not available")
		return
	}
	repos, err := h.extMgr.ListRepos(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list repos")
		return
	}
	out := make([]ExtensionRepoDTO, 0, len(repos))
	for _, repo := range repos {
		out = append(out, toExtensionRepoDTO(repo))
	}
	WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) handleCreateExtensionRepo(w http.ResponseWriter, r *http.Request) {
	if h.extMgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension manager not available")
		return
	}
	var req CreateExtensionRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.URL == "" {
		WriteError(w, http.StatusBadRequest, "url required")
		return
	}
	if req.Name == "" {
		req.Name = req.URL
	}
	repo, err := h.extMgr.AddRepo(r.Context(), req.Name, req.URL)
	if err != nil {
		h.logger.Error("add extension repo", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to add repo")
		return
	}
	WriteJSON(w, http.StatusCreated, toExtensionRepoDTO(repo))
}

func (h *Handler) handleInstallFromRepo(w http.ResponseWriter, r *http.Request) {
	if h.extMgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension manager not available")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	repo, err := h.store.Read().GetExtensionRepo(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "repo not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get repo")
		return
	}
	if err := h.extMgr.SyncRepo(ctx, repo); err != nil {
		h.logger.Error("sync repo", "id", id, "err", err)
		WriteError(w, http.StatusBadGateway, "sync failed: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

// handlePreviewExtensionRepo fetches a repo index and liveness-checks every
// listed torrent extension without installing anything, so the UI can show
// per-extension green/red status before the user confirms the add. An
// unreachable / invalid / empty index is a 4xx; a single dead extension is just
// usable:false in the list, not an error.
func (h *Handler) handlePreviewExtensionRepo(w http.ResponseWriter, r *http.Request) {
	if h.extMgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension manager not available")
		return
	}
	var req PreviewRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.URL == "" {
		WriteError(w, http.StatusBadRequest, "url required")
		return
	}
	entries, err := h.extMgr.PreviewRepo(r.Context(), req.URL)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := make([]PreviewEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, toPreviewEntryDTO(e))
	}
	WriteJSON(w, http.StatusOK, PreviewRepoResponse{Entries: out})
}

// handleTestExtension runs an installed extension's liveness Test(), persists the
// result to its centralized health record, and returns the outcome.
func (h *Handler) handleTestExtension(w http.ResponseWriter, r *http.Request) {
	if h.extMgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension manager not available")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	healthy, errMsg, err := h.extMgr.TestExtension(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "extension not found")
		return
	}
	if err != nil {
		h.logger.Error("test extension", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to test extension")
		return
	}
	var checkedAt int64
	if row, gerr := h.store.Read().GetExtension(ctx, id); gerr == nil && row.HealthCheckedAt != nil {
		checkedAt = *row.HealthCheckedAt
	}
	WriteJSON(w, http.StatusOK, ExtensionTestResponse{Healthy: healthy, Error: errMsg, CheckedAt: checkedAt})
}

func (h *Handler) handleDeleteExtensionRepo(w http.ResponseWriter, r *http.Request) {
	if h.extMgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension manager not available")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.extMgr.DeleteRepo(r.Context(), id); err != nil {
		h.logger.Error("delete extension repo", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to delete repo")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]int64{"id": id})
}

func (h *Handler) handleListExtensions(w http.ResponseWriter, r *http.Request) {
	exts, err := h.store.Read().ListExtensions(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list extensions")
		return
	}
	out := make([]ExtensionDTO, 0, len(exts))
	for _, ext := range exts {
		out = append(out, toExtensionDTO(ext))
	}
	WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) handleEnableExtension(w http.ResponseWriter, r *http.Request) {
	h.setExtensionEnabled(w, r, true)
}

func (h *Handler) handleDisableExtension(w http.ResponseWriter, r *http.Request) {
	h.setExtensionEnabled(w, r, false)
}

func (h *Handler) setExtensionEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if h.extMgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension manager not available")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var err error
	if enabled {
		err = h.extMgr.EnableExtension(ctx, id)
	} else {
		err = h.extMgr.DisableExtension(ctx, id)
	}
	if err != nil {
		h.logger.Error("set extension enabled", "id", id, "enabled", enabled, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to update extension")
		return
	}
	ext, err := h.store.Read().GetExtension(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "extension not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get extension")
		return
	}
	WriteJSON(w, http.StatusOK, toExtensionDTO(ext))
}

// handleExtensionIcon proxies an extension's remote icon through the daemon so
// the browser loads it from 'self' instead of widening the CSP's img-src. The
// fetch runs on the manager's DoH/SSRF-guarded client because the icon URL is
// attacker-controlled (it comes from a user-added repo).
func (h *Handler) handleExtensionIcon(w http.ResponseWriter, r *http.Request) {
	if h.extMgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension manager not available")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	contentType, body, err := h.extMgr.FetchIcon(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, extension.ErrNoIcon) {
			WriteError(w, http.StatusNotFound, "icon not found")
			return
		}
		h.logger.Error("fetch extension icon", "id", id, "err", err)
		WriteError(w, http.StatusNotFound, "icon not found")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *Handler) handleUninstallExtension(w http.ResponseWriter, r *http.Request) {
	if h.extMgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension manager not available")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.extMgr.UninstallExtension(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			WriteError(w, http.StatusNotFound, "extension not found")
			return
		}
		h.logger.Error("uninstall extension", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to uninstall extension")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]int64{"id": id})
}
