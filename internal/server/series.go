package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modbender/ssanime-gui/internal/anilist"
	"github.com/modbender/ssanime-gui/internal/metadata"
	"github.com/modbender/ssanime-gui/internal/store"
)

// applyMediaToCreate fills a CreateSeriesParams from fetched AniList media via
// the shared mapper, so the Media -> series-column field list lives in exactly
// one place (also used by the background metadata refresher).
func applyMediaToCreate(p *store.CreateSeriesParams, m anilist.Media) {
	f := anilist.MediaToSeriesFields(m)
	p.Title = f.Title
	p.AnilistID = f.AnilistID
	p.MalID = f.MalID
	p.RomajiTitle = f.RomajiTitle
	p.EnglishTitle = f.EnglishTitle
	p.Format = f.Format
	p.Status = f.Status
	p.AiringStatus = f.AiringStatus
	p.EpisodeCount = f.EpisodeCount
	p.Synonyms = f.Synonyms
	p.CoverImageUrl = f.CoverImage
	p.BannerImageUrl = f.BannerImage
	p.CoverColor = f.CoverColor
	p.Season = f.Season
	p.SeasonYear = f.SeasonYear
}

// derivedStatus computes the UI status string from AniList airing_status and
// the archive counts. No extra DB query needed — ListSeriesWithProgress joins counts.
func derivedStatus(airingStatus *string, episodeCount *int64, episodeTotal, episodeArchived int64) string {
	as := ""
	if airingStatus != nil {
		as = strings.ToUpper(*airingStatus)
	}
	switch as {
	case "NOT_YET_RELEASED":
		return "not_aired"
	case "CANCELLED":
		return "cancelled"
	case "FINISHED", "HIATUS":
		if episodeCount != nil && *episodeCount > 0 && episodeArchived >= *episodeCount {
			return "completed"
		}
		return "incomplete"
	case "RELEASING":
		if episodeCount != nil && *episodeCount > 0 && episodeArchived >= *episodeCount {
			return "up_to_date"
		}
		if episodeArchived < episodeTotal {
			return "airing"
		}
		return "up_to_date"
	default:
		if episodeCount != nil && *episodeCount > 0 && episodeArchived >= *episodeCount {
			return "completed"
		}
		return "airing"
	}
}

// rowToProgress builds a SeriesProgress wire row from a ListSeriesWithProgress
// row. derivedStatus computes the automatic status; a manual user_status
// override is carried alongside so the frontend can render the right badge and
// bucket the series. Used by the Library grid, the Downloads grouping, and the
// tracked-home endpoint so the mapping lives in one place.
func rowToProgress(row store.ListSeriesWithProgressRow) SeriesProgress {
	src := toInt64(row.SourceBytesTotal)
	enc := toInt64(row.EncodedBytesTotal)
	ds := derivedStatus(row.AiringStatus, row.EpisodeCount, row.EpisodeTotal, row.EpisodeArchived)
	return SeriesProgress{
		ID:                row.ID,
		UUID:              row.Uuid,
		Title:             row.Title,
		FeedTitle:         row.FeedTitle,
		SeasonNumber:      row.SeasonNumber,
		Subscribed:        row.Subscribed == 1,
		Favorite:          row.Favorite == 1,
		AiringStatus:      row.AiringStatus,
		DerivedStatus:     ds,
		UserStatus:        row.UserStatus,
		PosterPath:        row.PosterPath,
		CoverImageURL:     row.CoverImageUrl,
		BannerImageURL:    row.BannerImageUrl,
		CoverColor:        row.CoverColor,
		AnilistID:         row.AnilistID,
		RomajiTitle:       row.RomajiTitle,
		EnglishTitle:      row.EnglishTitle,
		Format:            row.Format,
		EpisodeCount:      row.EpisodeCount,
		EpisodeTotal:      row.EpisodeTotal,
		EpisodeArchived:   row.EpisodeArchived,
		SourceBytesTotal:  src,
		EncodedBytesTotal: enc,
		SpaceSavedBytes:   src - enc,
		AddedAt:           row.AddedAt,
		ModifiedAt:        row.ModifiedAt,
	}
}

// toInt64 safely converts an interface{} returned by sqlc aggregate columns.
func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case []byte:
		var n int64
		fmt.Sscanf(string(x), "%d", &n)
		return n
	}
	return 0
}

func (h *Handler) handleListSeries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filterSubscribed := q.Get("subscribed") == "true"
	filterFavorite := q.Get("favorite") == "true"
	filterStatus := q.Get("status")
	filterQ := strings.ToLower(q.Get("q"))

	rows, err := h.store.Read().ListSeriesWithProgress(r.Context())
	if err != nil {
		h.logger.Error("list series", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to list series")
		return
	}

	out := make([]SeriesProgress, 0, len(rows))
	for _, row := range rows {
		if filterSubscribed && row.Subscribed != 1 {
			continue
		}
		if filterFavorite && row.Favorite != 1 {
			continue
		}
		p := rowToProgress(row)
		if filterStatus != "" && p.DerivedStatus != filterStatus {
			continue
		}
		if filterQ != "" && !strings.Contains(strings.ToLower(row.Title), filterQ) {
			continue
		}
		out = append(out, p)
	}
	WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) handleGetSeries(w http.ResponseWriter, r *http.Request) {
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
		h.logger.Error("get series", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to get series")
		return
	}

	episodes, err := h.store.Read().ListEpisodesBySeries(r.Context(), id)
	if err != nil {
		h.logger.Error("list episodes", "series_id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to list episodes")
		return
	}

	details := make([]EpisodeDetail, 0, len(episodes))
	var archived, total int64
	for _, ep := range episodes {
		total++
		if ep.Status == "archived" {
			archived++
		}
		outputs, _ := h.store.Read().ListEncodedOutputsByEpisode(r.Context(), ep.ID)
		details = append(details, episodeToDetail(ep, series.Title, outputs))
	}
	ds := derivedStatus(series.AiringStatus, series.EpisodeCount, total, archived)

	WriteJSON(w, http.StatusOK, SeriesDetail{
		ID:               series.ID,
		UUID:             series.Uuid,
		Title:            series.Title,
		FeedTitle:        series.FeedTitle,
		AltTitles:        series.AltTitles,
		SeasonNumber:     series.SeasonNumber,
		Subscribed:       series.Subscribed == 1,
		Favorite:         series.Favorite == 1,
		AiringStatus:     series.AiringStatus,
		DerivedStatus:    ds,
		UserStatus:       series.UserStatus,
		PosterPath:       series.PosterPath,
		CoverImageURL:    series.CoverImageUrl,
		BannerImageURL:   series.BannerImageUrl,
		CoverColor:       series.CoverColor,
		AnilistID:        series.AnilistID,
		RomajiTitle:      series.RomajiTitle,
		EnglishTitle:     series.EnglishTitle,
		Format:           series.Format,
		EpisodeCount:     series.EpisodeCount,
		DefaultProfileID: series.DefaultProfileID,
		Episodes:         details,
		AddedAt:          series.AddedAt,
		ModifiedAt:       series.ModifiedAt,
	})
}

func (h *Handler) handleCreateSeries(w http.ResponseWriter, r *http.Request) {
	var req CreateSeriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.AnilistID == nil && (req.Title == nil || *req.Title == "") {
		WriteError(w, http.StatusBadRequest, "anilist_id or title required")
		return
	}

	ctx := r.Context()
	now := time.Now().Unix()

	// Duplicate check for AniList ID.
	if req.AnilistID != nil {
		if existing, err := h.store.Read().GetSeriesByAnilistID(ctx, req.AnilistID); err == nil {
			WriteError(w, http.StatusConflict, fmt.Sprintf("series already exists: id=%d", existing.ID))
			return
		}
	}

	params := store.CreateSeriesParams{
		Uuid:         mustUUID(),
		SeasonNumber: 1,
		Subscribed:   0,
		Favorite:     0,
	}
	if req.SeasonNumber != nil {
		params.SeasonNumber = *req.SeasonNumber
	}
	if req.ProfileID != nil {
		params.DefaultProfileID = req.ProfileID
	}

	if req.AnilistID != nil && h.anilist != nil {
		m, err := h.anilist.GetMedia(ctx, int(*req.AnilistID))
		if err == nil {
			applyMediaToCreate(&params, m)
			params.MetadataRefreshedAt = &now
		} else {
			h.logger.Warn("anilist fetch failed (proceeding without metadata)", "anilist_id", *req.AnilistID, "err", err)
			params.AnilistID = req.AnilistID
			params.Title = fmt.Sprintf("AniList #%d", *req.AnilistID)
		}
	} else if req.Title != nil {
		params.Title = *req.Title
		if h.anilist != nil {
			if list, err := h.anilist.SearchMedia(ctx, *req.Title); err == nil && len(list) > 0 {
				title := params.Title
				applyMediaToCreate(&params, list[0])
				params.Title = title // a title-search add keeps the user's typed title
				params.MetadataRefreshedAt = &now
			}
		}
	}

	if params.Title == "" {
		WriteError(w, http.StatusBadRequest, "could not determine series title")
		return
	}

	series, err := h.store.Write().CreateSeries(ctx, params)
	if err != nil {
		h.logger.Error("create series", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to create series")
		return
	}
	WriteJSON(w, http.StatusCreated, series)
}

func (h *Handler) handlePatchSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req PatchSeriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
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

	if req.Subscribed != nil {
		if err := h.store.Write().SetSeriesSubscribed(ctx, store.SetSeriesSubscribedParams{
			ID:         id,
			Subscribed: boolToInt64(*req.Subscribed),
		}); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to update subscribed")
			return
		}
		series.Subscribed = boolToInt64(*req.Subscribed)
	}
	if req.Favorite != nil {
		if err := h.store.Write().SetSeriesFavorite(ctx, store.SetSeriesFavoriteParams{
			ID:       id,
			Favorite: boolToInt64(*req.Favorite),
		}); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to update favorite")
			return
		}
		series.Favorite = boolToInt64(*req.Favorite)
	}
	if req.AiringStatus != nil {
		if err := h.store.Write().SetSeriesAiringStatus(ctx, store.SetSeriesAiringStatusParams{
			ID:           id,
			AiringStatus: req.AiringStatus,
		}); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to update airing_status")
			return
		}
	}
	if req.SeasonNumber != nil || req.DefaultProfileID != nil {
		p := store.UpdateSeriesParams{
			ID:               series.ID,
			Title:            series.Title,
			FeedTitle:        series.FeedTitle,
			AltTitles:        series.AltTitles,
			SeasonNumber:     series.SeasonNumber,
			Subscribed:       series.Subscribed,
			Favorite:         series.Favorite,
			AiringStatus:     series.AiringStatus,
			PosterPath:       series.PosterPath,
			PosterPortrait:   series.PosterPortrait,
			DefaultProfileID: series.DefaultProfileID,
			AnilistID:        series.AnilistID,
			MalID:            series.MalID,
			RomajiTitle:      series.RomajiTitle,
			EnglishTitle:     series.EnglishTitle,
			Format:           series.Format,
			Status:           series.Status,
			EpisodeCount:     series.EpisodeCount,
			Synonyms:         series.Synonyms,
			CoverImageUrl:    series.CoverImageUrl,
			BannerImageUrl:   series.BannerImageUrl,
			CoverColor:       series.CoverColor,
			Season:           series.Season,
			SeasonYear:       series.SeasonYear,
		}
		if req.SeasonNumber != nil {
			p.SeasonNumber = *req.SeasonNumber
		}
		if req.DefaultProfileID != nil {
			p.DefaultProfileID = req.DefaultProfileID
		}
		updated, err := h.store.Write().UpdateSeries(ctx, p)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to update series")
			return
		}
		WriteJSON(w, http.StatusOK, updated)
		return
	}

	// Re-fetch after partial updates.
	series, _ = h.store.Read().GetSeries(ctx, id)
	WriteJSON(w, http.StatusOK, series)
}

// handleRefreshSeries refreshes one series' AniList metadata on demand. On
// success it returns the updated row. AniList being rate-limited or unreachable
// is not a user error — it returns 503 with the existing metadata kept, never a
// 500. A series with no anilist_id is a 422 (nothing to refresh from).
func (h *Handler) handleRefreshSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if h.refresher == nil {
		WriteError(w, http.StatusServiceUnavailable, "metadata refresh is unavailable")
		return
	}

	updated, err := h.refresher.RefreshSeries(r.Context(), id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		WriteError(w, http.StatusNotFound, "series not found")
		return
	case errors.Is(err, metadata.ErrNoAnilistID):
		WriteError(w, http.StatusUnprocessableEntity, "series has no anilist_id to refresh from")
		return
	case err != nil:
		h.logger.Info("refresh series: upstream unavailable", "id", id, "err", err)
		WriteError(w, http.StatusServiceUnavailable, "AniList unavailable or rate-limited; existing metadata kept")
		return
	}
	// Bust the durable detail cache too, so the Refresh button refreshes both
	// metadata layers (the series row and the AniList+ani.zip detail payload).
	if updated.AnilistID != nil {
		if derr := h.store.Write().DeleteAnilistDetailCache(r.Context(), *updated.AnilistID); derr != nil {
			h.logger.Error("refresh series: bust detail cache", "anilist_id", *updated.AnilistID, "err", derr)
		}
	}
	WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) handleDeleteSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if _, err := h.store.Read().GetSeries(r.Context(), id); errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "series not found")
		return
	}
	if err := h.store.Write().DeleteSeries(r.Context(), id); err != nil {
		h.logger.Error("delete series", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to delete series")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
