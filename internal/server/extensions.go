package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
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
