package server

import (
	"net/http"
	"strconv"

	"github.com/modbender/ssanime-gui/internal/source"
)

func (h *Handler) handleSearchAnilist(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		WriteError(w, http.StatusBadRequest, "q required")
		return
	}
	if h.anilist == nil {
		WriteError(w, http.StatusServiceUnavailable, "anilist client not available")
		return
	}
	m, err := h.anilist.SearchMedia(r.Context(), q)
	if err != nil {
		h.logger.Warn("anilist search failed", "q", q, "err", err)
		WriteError(w, http.StatusBadGateway, "anilist search failed: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, AnilistSearchResult{
		ID:           m.ID,
		IDMal:        m.IDMal,
		RomajiTitle:  m.RomajiTitle,
		EnglishTitle: m.EnglishTitle,
		Format:       m.Format,
		Status:       m.Status,
		EpisodeCount: m.EpisodeCount,
		CoverImage:   m.CoverImage,
		BannerImage:  m.BannerImage,
		Season:       m.Season,
		SeasonYear:   m.SeasonYear,
		Synonyms:     m.Synonyms,
		IsAdult:      m.IsAdult,
	})
}

func (h *Handler) handleSearchTorrents(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		WriteError(w, http.StatusServiceUnavailable, "provider registry not available")
		return
	}
	q := r.URL.Query()
	seriesIDStr := q.Get("seriesId")
	episodeStr := q.Get("episode")
	providerID := q.Get("provider")

	var media source.Media
	if seriesIDStr != "" {
		if id, err := strconv.ParseInt(seriesIDStr, 10, 64); err == nil && h.store != nil {
			if s, err := h.store.Read().GetSeries(r.Context(), id); err == nil {
				media.RomajiTitle = s.Title
				if s.AnilistID != nil {
					media.ID = int(*s.AnilistID)
				}
				if s.EnglishTitle != nil {
					media.EnglishTitle = s.EnglishTitle
				}
				if s.RomajiTitle != nil {
					media.RomajiTitle = *s.RomajiTitle
				}
				if s.EpisodeCount != nil {
					media.EpisodeCount = int(*s.EpisodeCount)
				}
			}
		}
	}

	opts := source.SmartSearchOptions{
		Media:        media,
		BestReleases: true,
	}
	if episodeStr != "" {
		if ep, err := strconv.Atoi(episodeStr); err == nil {
			opts.EpisodeNumber = ep
		}
	}

	var results []TorrentSearchResult
	run := func(p source.Provider) {
		torrents, err := p.SmartSearch(r.Context(), opts)
		if err != nil {
			h.logger.Warn("torrent search error", "provider", p.ID(), "err", err)
			return
		}
		for _, t := range torrents {
			results = append(results, torrentToResult(t))
		}
	}

	if providerID != "" {
		p, ok := h.registry.Get(providerID)
		if !ok {
			WriteError(w, http.StatusBadRequest, "unknown provider: "+providerID)
			return
		}
		run(p)
	} else {
		for _, pid := range h.registry.List() {
			p, _ := h.registry.Get(pid)
			run(p)
		}
	}

	WriteJSON(w, http.StatusOK, results)
}
