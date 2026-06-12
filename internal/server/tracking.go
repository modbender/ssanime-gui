package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// User-status override values. NULL (absent) means fully automatic.
const (
	userStatusPaused  = "paused"
	userStatusDropped = "dropped"
)

// handleGetTracked groups tracked series into the home/Downloads buckets:
// in_progress (Active), completed, paused, dropped. A manual user_status override
// wins the bucket; otherwise the derived status decides. Series actively
// downloading or encoding are floated to the head of in_progress so the home's
// "Currently downloading" row leads with live work.
func (h *Handler) handleGetTracked(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.store.Read().ListSeriesWithProgress(ctx)
	if err != nil {
		h.logger.Error("tracked: list series", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to list series")
		return
	}

	active := h.activeSeriesIDs(ctx)

	resp := TrackedResponse{
		InProgress: []SeriesProgress{},
		Completed:  []SeriesProgress{},
		Paused:     []SeriesProgress{},
		Dropped:    []SeriesProgress{},
	}
	// inProgress holds (progress, isActivelyWorking) so we can float live work up.
	type ranked struct {
		p      SeriesProgress
		active bool
	}
	var inProgress []ranked

	for _, row := range rows {
		if row.Subscribed != 1 {
			continue
		}
		p := rowToProgress(row)
		switch us := strings.ToLower(strPtrVal(row.UserStatus)); us {
		case userStatusPaused:
			resp.Paused = append(resp.Paused, p)
		case userStatusDropped:
			resp.Dropped = append(resp.Dropped, p)
		default:
			if p.DerivedStatus == "completed" {
				resp.Completed = append(resp.Completed, p)
			} else {
				_, isActive := active[row.ID]
				inProgress = append(inProgress, ranked{p: p, active: isActive})
			}
		}
	}

	// Stable sort: actively-working series first, preserving title order within.
	sort.SliceStable(inProgress, func(i, j int) bool {
		return inProgress[i].active && !inProgress[j].active
	})
	for _, r := range inProgress {
		resp.InProgress = append(resp.InProgress, r.p)
	}

	WriteJSON(w, http.StatusOK, resp)
}

// activeSeriesIDs returns the set of series ids with an episode currently
// downloading or encoding, so they can be floated up in the in_progress bucket.
func (h *Handler) activeSeriesIDs(ctx context.Context) map[int64]struct{} {
	out := map[int64]struct{}{}
	for _, status := range []string{"downloading", "encoding"} {
		eps, err := h.store.Read().ListEpisodesByStatus(ctx, status)
		if err != nil {
			h.logger.Warn("tracked: list episodes by status", "status", status, "err", err)
			continue
		}
		for _, e := range eps {
			out[e.SeriesID] = struct{}{}
		}
	}
	return out
}

// handleTrackSeries is the single "Download & track" creation path. It creates
// (or upgrades) the series subscribed, auto-creates its feed if missing, clears
// any manual status override, and lets the running poller take over. Idempotent:
// re-tracking an existing series upgrades it and returns 200; a fresh series
// returns 201. AniList being unreachable is tolerated — the series is created
// with available data and the metadata refresher fills in later.
func (h *Handler) handleTrackSeries(w http.ResponseWriter, r *http.Request) {
	var req TrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.AnilistID <= 0 {
		WriteError(w, http.StatusBadRequest, "anilist_id required")
		return
	}

	ctx := r.Context()
	now := time.Now().Unix()
	anilistID := req.AnilistID

	created := false
	existing, err := h.store.Read().GetSeriesByAnilistID(ctx, &anilistID)
	switch {
	case err == nil:
		// Re-track: upgrade to subscribed + clear any manual override.
		if existing.Subscribed != 1 {
			if e := h.store.Write().SetSeriesSubscribed(ctx, store.SetSeriesSubscribedParams{ID: existing.ID, Subscribed: 1}); e != nil {
				h.logger.Error("track: subscribe existing", "id", existing.ID, "err", e)
				WriteError(w, http.StatusInternalServerError, "failed to subscribe series")
				return
			}
		}
		if e := h.store.Write().SetSeriesUserStatus(ctx, store.SetSeriesUserStatusParams{ID: existing.ID, UserStatus: nil}); e != nil {
			h.logger.Error("track: clear user_status", "id", existing.ID, "err", e)
			WriteError(w, http.StatusInternalServerError, "failed to re-engage series")
			return
		}
	case errors.Is(err, sql.ErrNoRows):
		// Fresh create: subscribed from the start.
		params := store.CreateSeriesParams{
			Uuid:         mustUUID(),
			SeasonNumber: 1,
			Subscribed:   1,
			Favorite:     0,
		}
		if h.anilist != nil {
			if m, e := h.anilist.GetMedia(ctx, int(anilistID)); e == nil {
				applyMediaToCreate(&params, m)
				params.MetadataRefreshedAt = &now
			} else {
				h.logger.Warn("track: anilist fetch failed (proceeding without metadata)", "anilist_id", anilistID, "err", e)
				params.AnilistID = &anilistID
				params.Title = fmt.Sprintf("AniList #%d", anilistID)
			}
		} else {
			params.AnilistID = &anilistID
			params.Title = fmt.Sprintf("AniList #%d", anilistID)
		}
		if params.Title == "" {
			params.AnilistID = &anilistID
			params.Title = fmt.Sprintf("AniList #%d", anilistID)
		}
		s, e := h.store.Write().CreateSeries(ctx, params)
		if e != nil {
			h.logger.Error("track: create series", "err", e)
			WriteError(w, http.StatusInternalServerError, "failed to create series")
			return
		}
		existing = s
		created = true
	default:
		h.logger.Error("track: lookup series", "anilist_id", anilistID, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to look up series")
		return
	}

	feedID, err := h.ensureFeed(ctx, existing.ID)
	if err != nil {
		h.logger.Error("track: ensure feed", "series_id", existing.ID, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to create feed")
		return
	}

	progress, err := h.seriesProgress(ctx, existing.ID)
	if err != nil {
		h.logger.Error("track: load progress", "series_id", existing.ID, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to load series")
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	WriteJSON(w, status, TrackResponse{Series: progress, SeriesID: existing.ID, FeedID: feedID})
}

// ensureFeed returns the id of an existing feed for the series, or creates a
// default one (the piece auto-track adds over plain create-series). The default
// feed has no structured URL — the poller drives SmartSearch from series
// metadata — so a sentinel url keeps the NOT NULL column satisfied.
func (h *Handler) ensureFeed(ctx context.Context, seriesID int64) (int64, error) {
	feeds, err := h.store.Read().ListFeedsBySeries(ctx, seriesID)
	if err != nil {
		return 0, err
	}
	if len(feeds) > 0 {
		// Ensure the first feed is enabled so a previously-disabled series re-polls.
		if feeds[0].Enabled != 1 {
			_ = h.store.Write().SetFeedEnabled(ctx, store.SetFeedEnabledParams{ID: feeds[0].ID, Enabled: 1})
		}
		return feeds[0].ID, nil
	}
	site := ""
	if h.registry != nil {
		if ids := h.registry.List(); len(ids) > 0 {
			site = ids[0]
		}
	}
	feed, err := h.store.Write().CreateFeed(ctx, store.CreateFeedParams{
		Uuid:            mustUUID(),
		SeriesID:        seriesID,
		Type:            "scrape",
		Site:            &site,
		Url:             fmt.Sprintf("ssanime://auto/%s/%d", site, seriesID),
		IntervalSeconds: 3600,
		Enabled:         1,
	})
	if err != nil {
		return 0, err
	}
	return feed.ID, nil
}

// handlePauseSeries / handleDropSeries / handleResumeSeries set or clear the
// manual user_status override. Pause/Drop make the feed dormant via the poller
// gate (user_status IS NULL); Resume re-engages full automation. No status change
// ever deletes files.
func (h *Handler) handlePauseSeries(w http.ResponseWriter, r *http.Request) {
	h.setUserStatus(w, r, strPtr(userStatusPaused))
}

func (h *Handler) handleDropSeries(w http.ResponseWriter, r *http.Request) {
	h.setUserStatus(w, r, strPtr(userStatusDropped))
}

func (h *Handler) handleResumeSeries(w http.ResponseWriter, r *http.Request) {
	h.setUserStatus(w, r, nil)
}

// setUserStatus is the shared body for pause/drop/resume: validate the series
// exists, write the override, return the refreshed SeriesProgress.
func (h *Handler) setUserStatus(w http.ResponseWriter, r *http.Request, status *string) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if _, err := h.store.Read().GetSeries(ctx, id); errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "series not found")
		return
	} else if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get series")
		return
	}
	if err := h.store.Write().SetSeriesUserStatus(ctx, store.SetSeriesUserStatusParams{ID: id, UserStatus: status}); err != nil {
		h.logger.Error("set user_status", "id", id, "status", strPtrVal(status), "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	progress, err := h.seriesProgress(ctx, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to load series")
		return
	}
	WriteJSON(w, http.StatusOK, SeriesStatusResponse{Series: progress})
}

// handleAvailableEpisodes runs an on-demand source search for a series NOW
// (independent of status — works for Paused/Dropped too) and returns the
// source-available episodes that are not yet downloaded, for the per-episode
// "download" UI.
func (h *Handler) handleAvailableEpisodes(w http.ResponseWriter, r *http.Request) {
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

	// Episode numbers already present locally (any status) are excluded so the
	// list shows only genuinely-new source availability.
	have := map[int]struct{}{}
	if eps, e := h.store.Read().ListEpisodesBySeries(ctx, id); e == nil {
		for _, ep := range eps {
			if ep.EpisodeNo != nil {
				have[int(*ep.EpisodeNo)] = struct{}{}
			}
		}
	}

	opts := source.SmartSearchOptions{Media: mediaFromSeries(series), BestReleases: true}

	// Keep the best release per episode number across all providers.
	best := map[int]*source.AnimeTorrent{}
	for _, pid := range h.registry.List() {
		p, _ := h.registry.Get(pid)
		torrents, err := p.SmartSearch(ctx, opts)
		if err != nil {
			h.logger.Warn("available: provider error", "provider", pid, "series_id", id, "err", err)
			continue
		}
		for _, t := range torrents {
			if t.EpisodeNumber <= 0 {
				continue // skip batches / unknown episode for the per-episode list
			}
			if _, ok := have[t.EpisodeNumber]; ok {
				continue
			}
			cur, ok := best[t.EpisodeNumber]
			if !ok || t.Seeders > cur.Seeders {
				best[t.EpisodeNumber] = t
			}
		}
	}

	episodes := make([]AvailableEpisode, 0, len(best))
	for num, t := range best {
		ep := AvailableEpisode{
			Number:     num,
			Title:      t.Name,
			SourceURL:  availableSourceURL(t),
			Resolution: t.Resolution,
		}
		if t.Size > 0 {
			sz := t.Size
			ep.Size = &sz
		}
		episodes = append(episodes, ep)
	}
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].Number < episodes[j].Number })

	WriteJSON(w, http.StatusOK, AvailableResponse{Episodes: episodes})
}

// handleDownloadAvailable downloads one source-found episode (from the
// /available list) that may not yet have an episodes row. It finds-or-creates
// the (series_id, number) episode, drives it into the pipeline as 'queued', and
// re-engages a paused/dropped series to Active. Idempotent: a second call for the
// same episode re-enqueues the existing row rather than duplicating it.
func (h *Handler) handleDownloadAvailable(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req DownloadAvailableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.SourceURL) == "" {
		WriteError(w, http.StatusBadRequest, "source_url required")
		return
	}
	if req.Number <= 0 {
		WriteError(w, http.StatusBadRequest, "number must be positive")
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

	// Find-or-create the episode for this number. ListEpisodesBySeries is the
	// only by-number lookup available, so scan it for a matching episode_no.
	num := int64(req.Number)
	existing, err := h.store.Read().ListEpisodesBySeries(ctx, id)
	if err != nil {
		h.logger.Error("download available: list episodes", "series_id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to load episodes")
		return
	}
	var ep store.Episode
	found := false
	for _, e := range existing {
		if e.EpisodeNo != nil && *e.EpisodeNo == num {
			ep = e
			found = true
			break
		}
	}

	created := false
	if !found {
		arg := store.CreateEpisodeParams{
			Uuid:       mustUUID(),
			SeriesID:   id,
			SourceKind: "torrent",
			EpisodeNo:  &num,
			Status:     "queued",
			ProfileID:  series.DefaultProfileID,
		}
		// A magnet URI goes in magnet; any other link goes in source_url —
		// mirrors how the poller's enqueue maps a release's link vs magnet.
		src := strings.TrimSpace(req.SourceURL)
		if strings.HasPrefix(src, "magnet:") {
			arg.Magnet = &src
		} else {
			arg.SourceUrl = &src
		}
		if res := parseResolution(req.Resolution); res > 0 {
			arg.Resolution = &res
		}
		ep, err = h.store.Write().CreateEpisode(ctx, arg)
		if err != nil {
			h.logger.Error("download available: create episode", "series_id", id, "number", req.Number, "err", err)
			WriteError(w, http.StatusInternalServerError, "failed to create episode")
			return
		}
		created = true
	}

	// Drive it into the pipeline exactly as the manual-enqueue path does: set
	// queued, broadcast episode.status, and re-engage the series to Active.
	if err := h.store.Write().SetEpisodeStatus(ctx, store.SetEpisodeStatusParams{
		ID:     ep.ID,
		Status: "queued",
	}); err != nil {
		h.logger.Error("download available: set queued", "id", ep.ID, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to enqueue episode")
		return
	}
	h.hub.Broadcast(events.TypeEpisodeStatus, map[string]any{
		"episode_id": ep.ID,
		"series_id":  id,
		"status":     "queued",
	})
	h.reengageSeries(ctx, id)

	// Re-read so the DTO reflects the queued status + any persisted columns.
	fresh, err := h.store.Read().GetEpisode(ctx, ep.ID)
	if err != nil {
		h.logger.Error("download available: reload episode", "id", ep.ID, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to load episode")
		return
	}
	outputs, _ := h.store.Read().ListEncodedOutputsByEpisode(ctx, fresh.ID)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	WriteJSON(w, status, episodeToDetail(fresh, outputs))
}

// parseResolution reads the leading run of digits from a resolution string
// ("1080p", "1080") into an int for episodes.resolution; 0 when none are present.
func parseResolution(res string) int64 {
	start := -1
	for i, c := range res {
		if c >= '0' && c <= '9' {
			start = i
			break
		}
	}
	if start < 0 {
		return 0
	}
	end := start
	for end < len(res) && res[end] >= '0' && res[end] <= '9' {
		end++
	}
	n, err := strconv.ParseInt(res[start:end], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// availableSourceURL prefers the magnet link, falling back to the torrent page.
func availableSourceURL(t *source.AnimeTorrent) string {
	if t.Magnet != "" {
		return t.Magnet
	}
	return t.Link
}

// seriesProgress loads one series' SeriesProgress by scanning the progress list
// (which carries the archive counts + user_status) for its id. Used by the
// track/pause/drop/resume responses so they return the same shape the grids read.
func (h *Handler) seriesProgress(ctx context.Context, id int64) (SeriesProgress, error) {
	rows, err := h.store.Read().ListSeriesWithProgress(ctx)
	if err != nil {
		return SeriesProgress{}, err
	}
	for _, row := range rows {
		if row.ID == id {
			return rowToProgress(row), nil
		}
	}
	return SeriesProgress{}, sql.ErrNoRows
}

// mediaFromSeries builds the source.Media used to drive SmartSearch from a
// series row's cached AniList metadata. Mirrors the poller's helper of the same
// intent (kept local to avoid a cross-package import).
func mediaFromSeries(s store.Series) source.Media {
	m := source.Media{
		ID:           derefInt64(s.AnilistID),
		RomajiTitle:  strPtrVal(s.RomajiTitle),
		EpisodeCount: -1,
	}
	if m.RomajiTitle == "" {
		m.RomajiTitle = s.Title
	}
	if s.MalID != nil {
		v := int(*s.MalID)
		m.IDMal = &v
	}
	if s.EnglishTitle != nil {
		m.EnglishTitle = s.EnglishTitle
	}
	if s.Status != nil {
		m.Status = *s.Status
	}
	if s.Format != nil {
		m.Format = *s.Format
	}
	if s.EpisodeCount != nil {
		m.EpisodeCount = int(*s.EpisodeCount)
	}
	m.Synonyms = parseSeriesSynonyms(s.Synonyms)
	if s.Title != "" {
		m.Synonyms = append(m.Synonyms, s.Title)
	}
	return m
}

// parseSeriesSynonyms reads the series.synonyms JSON array column.
func parseSeriesSynonyms(raw *string) []string {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	var syn []string
	if err := json.Unmarshal([]byte(*raw), &syn); err != nil {
		return nil
	}
	return syn
}

func strPtr(s string) *string { return &s }

func strPtrVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt64(i *int64) int {
	if i == nil {
		return 0
	}
	return int(*i)
}
