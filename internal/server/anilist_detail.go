package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/modbender/ssanime-gui/internal/anilist"
	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/store"
)

// detailCacheTTL is how long a cached detail payload is served before a refetch.
const detailCacheTTL = 24 * time.Hour

// AnilistDetailFetcher fetches the full AniList Media detail for one id. The
// concrete *anilist.Client satisfies it; the narrow interface lets tests inject
// a fake (including a rate-limit error).
type AnilistDetailFetcher interface {
	GetDetail(ctx context.Context, id int) (anilist.MediaDetail, error)
}

// AnizipFetcher fetches per-episode metadata from ani.zip for one AniList id.
// The concrete *anizip.Client satisfies it; ani.zip is best-effort, so an error
// here degrades the payload rather than failing the request.
type AnizipFetcher interface {
	GetEpisodes(ctx context.Context, anilistID int) ([]anizip.Episode, error)
}

// handleAnilistDetail serves the merged AniList + ani.zip series-detail payload
// for one AniList id, resolving both tracked and untracked series. Serving a
// fresh cache row (< 24h) costs zero upstream calls. On a stale/missing row it
// fetches AniList detail and ani.zip in parallel, merges, upserts, and serves.
//
// Failure posture mirrors the discovery cache: ani.zip failing alone degrades to
// an AniList-only payload (episodes lose thumbnails/overviews); AniList failing
// serves the stale row if one exists; only when there is neither a cache row nor
// a successful AniList fetch does it return an error.
func (h *Handler) handleAnilistDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAnilistID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	// Fresh cache hit: serve verbatim.
	cached, cachedOK := h.readDetailCache(ctx, id)
	if cachedOK && time.Since(time.Unix(cached.FetchedAt, 0)) < detailCacheTTL {
		writeRawDetail(w, cached.Payload)
		return
	}

	if h.anilistDetail == nil {
		// No fetcher wired (e.g. in a minimal test server): serve a stale row if
		// present, else error.
		if cachedOK {
			writeRawDetail(w, cached.Payload)
			return
		}
		WriteError(w, http.StatusServiceUnavailable, "detail metadata is unavailable")
		return
	}

	detail, episodes, alErr := h.fetchDetail(ctx, id)
	if alErr != nil {
		// AniList failed: serve a stale row if we have one, else surface the error.
		if cachedOK {
			h.logger.Info("anilist detail: upstream failed; serving stale cache", "anilist_id", id, "err", alErr)
			writeRawDetail(w, cached.Payload)
			return
		}
		h.logger.Info("anilist detail: upstream unavailable, no cache", "anilist_id", id, "err", alErr)
		WriteError(w, http.StatusServiceUnavailable, "AniList unavailable or rate-limited")
		return
	}

	payload := mergeDetail(id, detail, episodes)
	raw, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("anilist detail: marshal payload", "anilist_id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to build detail payload")
		return
	}
	h.upsertDetailCache(ctx, id, raw)

	WriteJSON(w, http.StatusOK, payload)
}

// fetchDetail runs the AniList detail and ani.zip episode fetches in parallel.
// The AniList fetch is load-bearing (its error fails the group); the ani.zip
// fetch is best-effort (its error is swallowed, yielding nil episodes).
func (h *Handler) fetchDetail(ctx context.Context, id int) (anilist.MediaDetail, []anizip.Episode, error) {
	var (
		detail   anilist.MediaDetail
		episodes []anizip.Episode
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		d, err := h.anilistDetail.GetDetail(gctx, id)
		if err != nil {
			return err
		}
		detail = d
		return nil
	})
	g.Go(func() error {
		if h.anizip == nil {
			return nil
		}
		eps, err := h.anizip.GetEpisodes(gctx, id)
		if err != nil {
			// Best-effort: log and degrade, never fail the group.
			h.logger.Info("anilist detail: ani.zip fetch failed; degrading to AniList-only", "anilist_id", id, "err", err)
			return nil
		}
		episodes = eps
		return nil
	})
	if err := g.Wait(); err != nil {
		return anilist.MediaDetail{}, nil, err
	}
	return detail, episodes, nil
}

// mergeDetail builds the frozen AnilistDetail payload from the AniList detail and
// the ani.zip episodes. Episodes merge ani.zip-primary, with AniList's
// streamingEpisodes filling thumbnails/titles by 1-based index where ani.zip is
// absent or sparse; the result is sorted by number.
func mergeDetail(id int, d anilist.MediaDetail, anizipEps []anizip.Episode) AnilistDetail {
	out := AnilistDetail{
		AnilistID:       id,
		TitleEnglish:    d.EnglishTitle,
		TitleRomaji:     d.RomajiTitle,
		CoverImage:      d.CoverImage,
		CoverColor:      d.CoverColor,
		BannerImage:     d.BannerImage,
		Format:          d.Format,
		AiringStatus:    d.Status,
		Description:     d.Description,
		Genres:          orEmptyStrings(d.Genres),
		AverageScore:    d.AverageScore,
		Studio:          d.Studio,
		SourceMaterial:  d.Source,
		Season:          d.Season,
		SeasonYear:      d.SeasonYear,
		DurationMin:     d.Duration,
		EpisodeCount:    d.EpisodeCount,
		Episodes:        []DetailEpisode{},
		Relations:       []RelatedMediaCard{},
		Recommendations: []RelatedMediaCard{},
	}
	if d.NextAiring != nil {
		out.NextAiring = &NextAiring{Episode: d.NextAiring.Episode, AiringAt: d.NextAiring.AiringAt}
	}
	if d.Trailer != nil {
		out.Trailer = &DetailTrailer{Site: d.Trailer.Site, VideoID: d.Trailer.VideoID, Thumbnail: d.Trailer.Thumbnail}
	}
	out.Episodes = mergeEpisodes(anizipEps, d.StreamingEpisodes)
	for _, rel := range d.Relations {
		out.Relations = append(out.Relations, relatedToCard(rel))
	}
	for _, rec := range d.Recommendations {
		out.Recommendations = append(out.Recommendations, relatedToCard(rec))
	}
	return out
}

// mergeEpisodes builds the per-episode list keyed by number. ani.zip is primary;
// AniList streamingEpisodes (which carry no number) fill in by 1-based position
// to supply a thumbnail/title for episodes ani.zip lacks. Episodes are sorted by
// number.
func mergeEpisodes(anizipEps []anizip.Episode, streaming []anilist.StreamingEpisode) []DetailEpisode {
	byNum := make(map[int]DetailEpisode)
	order := make([]int, 0, len(anizipEps))
	for _, e := range anizipEps {
		if _, seen := byNum[e.Number]; !seen {
			order = append(order, e.Number)
		}
		byNum[e.Number] = DetailEpisode{
			Number:     e.Number,
			Title:      e.Title,
			Thumbnail:  e.Thumbnail,
			AirDate:    e.AirDate,
			Overview:   e.Overview,
			RuntimeMin: e.RuntimeMin,
		}
	}
	// streamingEpisodes have no episode number; map them positionally (1-based).
	for i, se := range streaming {
		num := i + 1
		ep, exists := byNum[num]
		if !exists {
			ep = DetailEpisode{Number: num}
			order = append(order, num)
		}
		if ep.Thumbnail == "" {
			ep.Thumbnail = se.Thumbnail
		}
		if ep.Title == "" {
			ep.Title = se.Title
		}
		byNum[num] = ep
	}
	out := make([]DetailEpisode, 0, len(byNum))
	for _, num := range order {
		out = append(out, byNum[num])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

func relatedToCard(r anilist.RelatedMedia) RelatedMediaCard {
	return RelatedMediaCard{
		AnilistID:    r.AnilistID,
		RelationType: r.RelationType,
		TitleEnglish: r.EnglishTitle,
		TitleRomaji:  r.RomajiTitle,
		CoverImage:   r.CoverImage,
		CoverColor:   r.CoverColor,
		Format:       r.Format,
		Status:       r.Status,
	}
}

// readDetailCache returns the cache row for an AniList id, if present.
func (h *Handler) readDetailCache(ctx context.Context, id int) (store.AnilistDetailCache, bool) {
	row, err := h.store.Read().GetAnilistDetailCache(ctx, int64(id))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			h.logger.Error("anilist detail: read cache", "anilist_id", id, "err", err)
		}
		return store.AnilistDetailCache{}, false
	}
	return row, true
}

// upsertDetailCache writes the merged payload to the durable cache, stamping the
// fetch time. A write error is logged but non-fatal — the response was already
// served from the live fetch.
func (h *Handler) upsertDetailCache(ctx context.Context, id int, payload []byte) {
	err := h.store.Write().UpsertAnilistDetailCache(ctx, store.UpsertAnilistDetailCacheParams{
		AnilistID: int64(id),
		Payload:   string(payload),
		FetchedAt: time.Now().Unix(),
	})
	if err != nil {
		h.logger.Error("anilist detail: upsert cache", "anilist_id", id, "err", err)
	}
}

// writeRawDetail serves an already-marshalled detail payload inside the success
// envelope without re-encoding the cached JSON.
func writeRawDetail(w http.ResponseWriter, payload string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"data":` + payload + `,"error":""}`))
}

// orEmptyStrings returns a non-nil slice so genres serialises as [] not null.
func orEmptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
