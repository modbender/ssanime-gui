package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modbender/ssanime-gui/internal/store"
)

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
		src := toInt64(row.SourceBytesTotal)
		enc := toInt64(row.EncodedBytesTotal)
		ds := derivedStatus(row.AiringStatus, row.EpisodeCount, row.EpisodeTotal, row.EpisodeArchived)

		if filterSubscribed && row.Subscribed != 1 {
			continue
		}
		if filterFavorite && row.Favorite != 1 {
			continue
		}
		if filterStatus != "" && ds != filterStatus {
			continue
		}
		if filterQ != "" && !strings.Contains(strings.ToLower(row.Title), filterQ) {
			continue
		}

		out = append(out, SeriesProgress{
			ID:                row.ID,
			UUID:              row.Uuid,
			Title:             row.Title,
			FeedTitle:         row.FeedTitle,
			SeasonNumber:      row.SeasonNumber,
			Subscribed:        row.Subscribed == 1,
			Favorite:          row.Favorite == 1,
			AiringStatus:      row.AiringStatus,
			DerivedStatus:     ds,
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
		})
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
		details = append(details, episodeToDetail(ep, outputs))
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
			params.AnilistID = req.AnilistID
			if m.IDMal != nil {
				id := int64(*m.IDMal)
				params.MalID = &id
			}
			params.Title = m.RomajiTitle
			if m.EnglishTitle != "" {
				params.EnglishTitle = &m.EnglishTitle
				params.Title = m.EnglishTitle
			}
			params.RomajiTitle = &m.RomajiTitle
			params.Format = &m.Format
			s := m.Status
			params.AiringStatus = &s
			params.Status = &s
			if m.EpisodeCount > 0 {
				ec := int64(m.EpisodeCount)
				params.EpisodeCount = &ec
			}
			if m.CoverImage != "" {
				params.CoverImageUrl = &m.CoverImage
			}
			if m.BannerImage != "" {
				params.BannerImageUrl = &m.BannerImage
			}
			if m.CoverColor != "" {
				params.CoverColor = &m.CoverColor
			}
			if m.Season != "" {
				params.Season = &m.Season
			}
			if m.SeasonYear != 0 {
				sy := int64(m.SeasonYear)
				params.SeasonYear = &sy
			}
			if len(m.Synonyms) > 0 {
				syn, _ := json.Marshal(m.Synonyms)
				s := string(syn)
				params.Synonyms = &s
			}
		} else {
			h.logger.Warn("anilist fetch failed (proceeding without metadata)", "anilist_id", *req.AnilistID, "err", err)
			params.AnilistID = req.AnilistID
			params.Title = fmt.Sprintf("AniList #%d", *req.AnilistID)
		}
	} else if req.Title != nil {
		params.Title = *req.Title
		if h.anilist != nil {
			if m, err := h.anilist.SearchMedia(ctx, *req.Title); err == nil {
				aid := int64(m.ID)
				params.AnilistID = &aid
				if m.IDMal != nil {
					mid := int64(*m.IDMal)
					params.MalID = &mid
				}
				if m.EnglishTitle != "" {
					params.EnglishTitle = &m.EnglishTitle
				}
				params.RomajiTitle = &m.RomajiTitle
				params.Format = &m.Format
				st := m.Status
				params.AiringStatus = &st
				params.Status = &st
				if m.EpisodeCount > 0 {
					ec := int64(m.EpisodeCount)
					params.EpisodeCount = &ec
				}
				if m.CoverImage != "" {
					params.CoverImageUrl = &m.CoverImage
				}
				if m.BannerImage != "" {
					params.BannerImageUrl = &m.BannerImage
				}
				if m.CoverColor != "" {
					params.CoverColor = &m.CoverColor
				}
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
