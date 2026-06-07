package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// episodeToDetail converts a store.Episode + its outputs into an EpisodeDetail DTO.
func episodeToDetail(ep store.Episode, outputs []store.EncodedOutput) EpisodeDetail {
	outs := make([]OutputSummary, len(outputs))
	for i, o := range outputs {
		outs[i] = OutputSummary{
			ID:           o.ID,
			UUID:         o.Uuid,
			Resolution:   o.Resolution,
			Status:       o.Status,
			EncodedPath:  o.EncodedPath,
			EncodedSize:  o.EncodedSize,
			ErrorMessage: o.ErrorMessage,
			EncodedAt:    o.EncodedAt,
		}
	}
	return EpisodeDetail{
		ID:           ep.ID,
		UUID:         ep.Uuid,
		SeriesID:     ep.SeriesID,
		Title:        ep.Title,
		EpisodeNo:    ep.EpisodeNo,
		Status:       ep.Status,
		Resolution:   ep.Resolution,
		ReleaseGroup: ep.ReleaseGroup,
		Subtype:      ep.Subtype,
		Uncensored:   ep.Uncensored == 1,
		Bluray:       ep.Bluray == 1,
		SourceSize:   ep.SourceSize,
		ProfileID:    ep.ProfileID,
		ErrorMessage: ep.ErrorMessage,
		RetryCount:   ep.RetryCount,
		PublishedAt:  ep.PublishedAt,
		DownloadedAt: ep.DownloadedAt,
		EncodedAt:    ep.EncodedAt,
		Outputs:      outs,
		AddedAt:      ep.AddedAt,
		ModifiedAt:   ep.ModifiedAt,
	}
}

// torrentToResult maps a source.AnimeTorrent to a TorrentSearchResult DTO.
func torrentToResult(t *source.AnimeTorrent) TorrentSearchResult {
	if t == nil {
		return TorrentSearchResult{}
	}
	return TorrentSearchResult{
		Provider:      t.Provider,
		Name:          t.Name,
		Magnet:        t.Magnet,
		Link:          t.Link,
		InfoHash:      t.InfoHash,
		Date:          t.Date,
		Size:          t.Size,
		Seeders:       t.Seeders,
		Resolution:    t.Resolution,
		ReleaseGroup:  t.ReleaseGroup,
		EpisodeNumber: t.EpisodeNumber,
		IsBatch:       t.IsBatch,
		IsBestRelease: t.IsBestRelease,
		Confirmed:     t.Confirmed,
	}
}

func (h *Handler) handleListEpisodes(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	episodes, err := h.store.Read().ListEpisodesBySeries(r.Context(), id)
	if err != nil {
		h.logger.Error("list episodes", "series_id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to list episodes")
		return
	}
	details := make([]EpisodeDetail, 0, len(episodes))
	for _, ep := range episodes {
		outputs, _ := h.store.Read().ListEncodedOutputsByEpisode(r.Context(), ep.ID)
		details = append(details, episodeToDetail(ep, outputs))
	}
	WriteJSON(w, http.StatusOK, details)
}

// handleScanEpisodes runs SmartSearch on all registered providers for a series
// and returns the candidate torrents without enqueuing them.
func (h *Handler) handleScanEpisodes(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if h.registry == nil {
		WriteError(w, http.StatusServiceUnavailable, "provider registry not available")
		return
	}

	ctx := r.Context()
	series, err := h.store.Read().GetSeries(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "series not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get series")
		return
	}

	media := source.Media{
		RomajiTitle: series.Title,
	}
	if series.AnilistID != nil {
		media.ID = int(*series.AnilistID)
	}
	if series.EnglishTitle != nil {
		media.EnglishTitle = series.EnglishTitle
	}
	if series.RomajiTitle != nil {
		media.RomajiTitle = *series.RomajiTitle
	}
	if series.EpisodeCount != nil {
		media.EpisodeCount = int(*series.EpisodeCount)
	}

	opts := source.SmartSearchOptions{
		Media:        media,
		BestReleases: true,
	}

	var results []TorrentSearchResult
	for _, pid := range h.registry.List() {
		p, _ := h.registry.Get(pid)
		torrents, err := p.SmartSearch(ctx, opts)
		if err != nil {
			h.logger.Warn("scan: provider error", "provider", pid, "series_id", id, "err", err)
			continue
		}
		for _, t := range torrents {
			results = append(results, torrentToResult(t))
		}
	}
	WriteJSON(w, http.StatusOK, results)
}

// handleBulkEncode enqueues a set of episodes for download+encode by setting
// their status to 'queued'. The download and encode queues pick them up on their
// next scan tick.
func (h *Handler) handleBulkEncode(w http.ResponseWriter, r *http.Request) {
	var req BulkEncodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.EpisodeIDs) == 0 {
		WriteError(w, http.StatusBadRequest, "episode_ids required")
		return
	}

	ctx := r.Context()
	enqueued := 0
	for _, eid := range req.EpisodeIDs {
		if _, err := h.store.Read().GetEpisode(ctx, eid); errors.Is(err, sql.ErrNoRows) {
			continue // skip missing
		} else if err != nil {
			h.logger.Error("bulk encode: get episode", "id", eid, "err", err)
			continue
		}

		if err := h.store.Write().SetEpisodeStatus(ctx, store.SetEpisodeStatusParams{
			ID:     eid,
			Status: "queued",
		}); err != nil {
			h.logger.Error("bulk encode: set queued", "id", eid, "err", err)
			continue
		}
		enqueued++
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"enqueued": enqueued,
		"total":    len(req.EpisodeIDs),
	})
}

func (h *Handler) handleEncodeEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	h.enqueueEpisode(w, r.Context(), id)
}

func (h *Handler) handleRetryEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ep, err := h.store.Read().GetEpisode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "episode not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get episode")
		return
	}
	if ep.Status != "error" {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("episode status is %q, not error", ep.Status))
		return
	}
	if err := h.store.Write().IncrementEpisodeRetry(ctx, id); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to increment retry count")
		return
	}
	h.enqueueEpisode(w, ctx, id)
}

func (h *Handler) enqueueEpisode(w http.ResponseWriter, ctx context.Context, id int64) {
	if _, err := h.store.Read().GetEpisode(ctx, id); errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "episode not found")
		return
	}
	if err := h.store.Write().SetEpisodeStatus(ctx, store.SetEpisodeStatusParams{
		ID:     id,
		Status: "queued",
	}); err != nil {
		h.logger.Error("enqueue episode", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to enqueue episode")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "queued"})
}

func (h *Handler) handleDeleteEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if _, err := h.store.Read().GetEpisode(r.Context(), id); errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "episode not found")
		return
	}
	if err := h.store.Write().DeleteEpisode(r.Context(), id); err != nil {
		h.logger.Error("delete episode", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to delete episode")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
