package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/modbender/ssanime-gui/internal/store"
)

func (h *Handler) handleListFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := h.store.Read().ListFeeds(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list feeds")
		return
	}
	WriteJSON(w, http.StatusOK, feeds)
}

func (h *Handler) handleCreateFeed(w http.ResponseWriter, r *http.Request) {
	var req CreateFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.URL == "" {
		WriteError(w, http.StatusBadRequest, "url required")
		return
	}
	if req.SeriesID == 0 {
		WriteError(w, http.StatusBadRequest, "series_id required")
		return
	}
	interval := req.IntervalSeconds
	if interval == 0 {
		interval = 3600
	}
	feed, err := h.store.Write().CreateFeed(r.Context(), store.CreateFeedParams{
		Uuid:            mustUUID(),
		SeriesID:        req.SeriesID,
		Type:            req.Type,
		Site:            req.Site,
		Url:             req.URL,
		Quality:         req.Quality,
		Subtype:         req.Subtype,
		Deinterlace:     boolToInt64(req.Deinterlace),
		Uncensored:      boolToInt64(req.Uncensored),
		Bluray:          boolToInt64(req.Bluray),
		TitleRegex:      req.TitleRegex,
		ExtraTags:       req.ExtraTags,
		IntervalSeconds: interval,
		OffsetSeconds:   req.OffsetSeconds,
		SeenCache:       nil,
		Enabled:         boolToInt64(req.Enabled),
	})
	if err != nil {
		h.logger.Error("create feed", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to create feed")
		return
	}
	WriteJSON(w, http.StatusCreated, feed)
}

func (h *Handler) handlePatchFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	existing, err := h.store.Read().GetFeed(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "feed not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get feed")
		return
	}
	var req PatchFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Merge patch: start from existing, override non-nil fields.
	p := store.UpdateFeedParams{
		ID:              id,
		Type:            existing.Type,
		Site:            existing.Site,
		Url:             existing.Url,
		Quality:         existing.Quality,
		Subtype:         existing.Subtype,
		Deinterlace:     existing.Deinterlace,
		Uncensored:      existing.Uncensored,
		Bluray:          existing.Bluray,
		TitleRegex:      existing.TitleRegex,
		ExtraTags:       existing.ExtraTags,
		IntervalSeconds: existing.IntervalSeconds,
		OffsetSeconds:   existing.OffsetSeconds,
		Enabled:         existing.Enabled,
	}
	if req.Type != nil {
		p.Type = *req.Type
	}
	if req.Site != nil {
		p.Site = req.Site
	}
	if req.URL != nil {
		p.Url = *req.URL
	}
	if req.Quality != nil {
		p.Quality = req.Quality
	}
	if req.Subtype != nil {
		p.Subtype = req.Subtype
	}
	if req.Deinterlace != nil {
		p.Deinterlace = boolToInt64(*req.Deinterlace)
	}
	if req.Uncensored != nil {
		p.Uncensored = boolToInt64(*req.Uncensored)
	}
	if req.Bluray != nil {
		p.Bluray = boolToInt64(*req.Bluray)
	}
	if req.TitleRegex != nil {
		p.TitleRegex = req.TitleRegex
	}
	if req.ExtraTags != nil {
		p.ExtraTags = req.ExtraTags
	}
	if req.IntervalSeconds != nil {
		p.IntervalSeconds = *req.IntervalSeconds
	}
	if req.OffsetSeconds != nil {
		p.OffsetSeconds = *req.OffsetSeconds
	}
	if req.Enabled != nil {
		p.Enabled = boolToInt64(*req.Enabled)
	}

	feed, err := h.store.Write().UpdateFeed(ctx, p)
	if err != nil {
		h.logger.Error("update feed", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to update feed")
		return
	}
	WriteJSON(w, http.StatusOK, feed)
}

func (h *Handler) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if _, err := h.store.Read().GetFeed(r.Context(), id); errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "feed not found")
		return
	}
	if err := h.store.Write().DeleteFeed(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to delete feed")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
