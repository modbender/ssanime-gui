package server

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/modbender/ssanime-gui/internal/animedb"
	"github.com/modbender/ssanime-gui/internal/source"
)

// animeSearchLimit caps offline search results, matching what the add-series
// dropdown can usefully show.
const animeSearchLimit = 25

// cspImageHosts is the set of image hosts the CSP img-src directive allows.
// A search-result CoverImage is only emitted when its host is in this set; any
// other host (manami pictures are mostly cdn.myanimelist.net) is dropped to ""
// so the card shows a placeholder rather than a CSP-blocked broken image. The
// real cover arrives when the series is added (the by-id AniList fetch in
// handleCreateSeries). Keep in sync with the img-src list in security.go.
var cspImageHosts = map[string]bool{
	"s4.anilist.co": true,
	"img.anili.st":  true,
}

// cspSafeImage returns rawURL if its host is CSP-allowed, otherwise "".
func cspSafeImage(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || !cspImageHosts[u.Hostname()] {
		return ""
	}
	return rawURL
}

// handleSearchAnilist answers the add-series search. It serves from the offline
// animedb index (zero AniList calls) when that index is ready, and falls back to
// the live AniList API during first-boot warmup while the dataset is still
// downloading. Both branches return the identical AnilistSearchResult shape.
func (h *Handler) handleSearchAnilist(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		WriteError(w, http.StatusBadRequest, "q required")
		return
	}

	if h.animedb != nil && h.animedb.Ready() {
		WriteJSON(w, http.StatusOK, offlineSearchResults(h.animedb.Search(q, animeSearchLimit)))
		return
	}

	// Warmup fallback: the offline index isn't loaded yet (initial download in
	// flight) — answer from AniList so search still works on first boot.
	if h.anilist == nil {
		WriteError(w, http.StatusServiceUnavailable, "anilist client not available")
		return
	}
	media, err := h.anilist.SearchMedia(r.Context(), q)
	if err != nil {
		h.logger.Warn("anilist search failed", "q", q, "err", err)
		WriteError(w, http.StatusBadGateway, "anilist search failed: "+err.Error())
		return
	}
	results := make([]AnilistSearchResult, 0, len(media))
	for _, m := range media {
		results = append(results, AnilistSearchResult{
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
	WriteJSON(w, http.StatusOK, results)
}

// offlineSearchResults maps animedb hits into the AnilistSearchResult wire
// shape. The offline dataset has no separate english title (Title → RomajiTitle,
// EnglishTitle left ""), no MAL id, and no reliable is_adult flag, so those stay
// zero-valued. CoverImage is filtered to CSP-safe hosts only.
func offlineSearchResults(hits []animedb.Result) []AnilistSearchResult {
	out := make([]AnilistSearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, AnilistSearchResult{
			ID:           h.AniListID,
			RomajiTitle:  h.Title,
			Format:       h.Type,
			Status:       h.Status,
			EpisodeCount: h.Episodes,
			CoverImage:   cspSafeImage(h.Picture),
			Season:       h.Season,
			SeasonYear:   h.Year,
			Synonyms:     h.Synonyms,
		})
	}
	return out
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
