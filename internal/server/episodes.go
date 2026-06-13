package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// episodeToDetail converts a store.Episode + its outputs into an EpisodeDetail DTO.
// seriesTitle is the parent series' title, joined in by the caller for the
// drawer's group-by-series view.
func episodeToDetail(ep store.Episode, seriesTitle string, outputs []store.EncodedOutput) EpisodeDetail {
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
		ID:              ep.ID,
		UUID:            ep.Uuid,
		SeriesID:        ep.SeriesID,
		SeriesTitle:     seriesTitle,
		Title:           ep.Title,
		EpisodeNo:       ep.EpisodeNo,
		Status:          ep.Status,
		Resolution:      ep.Resolution,
		ReleaseGroup:    ep.ReleaseGroup,
		Subtype:         ep.Subtype,
		Uncensored:      ep.Uncensored == 1,
		Bluray:          ep.Bluray == 1,
		SourceSize:      ep.SourceSize,
		SourcePath:      ep.SourcePath,
		SourceCleanedAt: ep.SourceCleanedAt,
		ProfileID:       ep.ProfileID,
		ErrorMessage:    ep.ErrorMessage,
		RetryCount:      ep.RetryCount,
		PublishedAt:     ep.PublishedAt,
		DownloadedAt:    ep.DownloadedAt,
		EncodedAt:       ep.EncodedAt,
		Outputs:         outs,
		AddedAt:         ep.AddedAt,
		ModifiedAt:      ep.ModifiedAt,
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
	series, err := h.store.Read().GetSeries(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "series not found")
		return
	}
	if err != nil {
		h.logger.Error("list episodes: get series", "series_id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to list episodes")
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
		details = append(details, episodeToDetail(ep, series.Title, outputs))
	}
	WriteJSON(w, http.StatusOK, details)
}

// handleGetEpisode returns a single episode's detail (paths, cleanup status,
// outputs, series title) for the Activity drawer's detail view. Lighter than
// GET /api/series/{id}.
func (h *Handler) handleGetEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	row, err := h.store.Read().GetEpisodeWithSeries(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "episode not found")
		return
	}
	if err != nil {
		h.logger.Error("get episode", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to get episode")
		return
	}
	outputs, _ := h.store.Read().ListEncodedOutputsByEpisode(r.Context(), id)
	WriteJSON(w, http.StatusOK, episodeToDetail(row.Episode, row.SeriesTitle, outputs))
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

// handleRetryEpisode requeues an errored episode: it is valid only when the
// episode is in 'error'. It clears error_message, increments retry_count, sets
// status 'queued', broadcasts episode.status=queued, and returns
// {"episode": <EpisodeDetail>}. A non-error episode is a 409. Retrying an
// episode never changes the series' subscription or watch status.
func (h *Handler) handleRetryEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ep, err := h.store.Read().GetEpisodeWithSeries(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "episode not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get episode")
		return
	}
	if ep.Episode.Status != "error" {
		WriteError(w, http.StatusConflict, fmt.Sprintf("episode status is %q, not error", ep.Episode.Status))
		return
	}
	if err := h.store.Write().IncrementEpisodeRetry(ctx, id); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to increment retry count")
		return
	}
	if err := h.store.Write().ClearEpisodeError(ctx, id); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to clear error")
		return
	}
	if err := h.store.Write().SetEpisodeStatus(ctx, store.SetEpisodeStatusParams{ID: id, Status: "queued"}); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to requeue episode")
		return
	}
	h.hub.Broadcast(events.TypeEpisodeStatus, map[string]any{
		"episode_id": id,
		"series_id":  ep.Episode.SeriesID,
		"status":     "queued",
	})

	fresh, err := h.store.Read().GetEpisode(ctx, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to load episode")
		return
	}
	outputs, _ := h.store.Read().ListEncodedOutputsByEpisode(ctx, id)
	WriteJSON(w, http.StatusOK, EpisodeRetryResponse{Episode: episodeToDetail(fresh, ep.SeriesTitle, outputs)})
}

func (h *Handler) enqueueEpisode(w http.ResponseWriter, ctx context.Context, id int64) {
	_, err := h.store.Read().GetEpisode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "episode not found")
		return
	}
	if err != nil {
		h.logger.Error("enqueue episode: get", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to enqueue episode")
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
	ctx := r.Context()
	ep, err := h.store.Read().GetEpisode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "episode not found")
		return
	}
	if err != nil {
		h.logger.Error("delete episode: get", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to delete episode")
		return
	}
	if err := h.store.Write().DeleteEpisode(ctx, id); err != nil {
		h.logger.Error("delete episode", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to delete episode")
		return
	}

	// Garbage-collect the parent series if this was its last episode and it is not
	// subscribed — holding the "exists iff subscribed OR has episodes" invariant.
	h.gcSeriesIfOrphaned(ctx, ep.SeriesID)

	WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// gcSeriesIfOrphaned deletes the series row when it is unsubscribed AND has zero
// remaining episodes, broadcasting series.updated{deleted:true}. Best-effort: a
// lookup error leaves the row in place rather than failing the caller.
func (h *Handler) gcSeriesIfOrphaned(ctx context.Context, seriesID int64) {
	series, err := h.store.Read().GetSeries(ctx, seriesID)
	if err != nil {
		return // already gone, or unreadable — nothing to GC
	}
	if series.Subscribed == 1 {
		return
	}
	count, err := h.store.Read().CountEpisodesBySeries(ctx, seriesID)
	if err != nil || count > 0 {
		return
	}
	if err := h.store.Write().DeleteSeries(ctx, seriesID); err != nil {
		h.logger.Error("gc orphaned series", "id", seriesID, "err", err)
		return
	}
	h.hub.Broadcast(events.TypeSeriesUpdated, map[string]any{
		"series_id": seriesID,
		"deleted":   true,
	})
}
