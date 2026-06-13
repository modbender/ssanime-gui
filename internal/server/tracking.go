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

	"github.com/modbender/ssanime-gui/internal/anilist"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// Watch-status values: the AniList-style state that solely drives polling.
// 'completed' is derived (finished airing + all episodes archived), never stored.
const (
	watchStatusWatching = "watching"
	watchStatusOnHold   = "on_hold"
	watchStatusDropped  = "dropped"
)

// validWatchStatuses is the set accepted by POST /api/series/{id}/status.
var validWatchStatuses = map[string]struct{}{
	watchStatusWatching: {},
	watchStatusOnHold:   {},
	watchStatusDropped:  {},
}

// handleGetTracked groups subscribed series into the home/Downloads buckets:
// in_progress (Active), completed, paused, dropped — bucketed by watch_status and
// the derived status. Series actively downloading or encoding are floated to the
// head of in_progress so the home's "Currently downloading" row leads with live work.
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
		switch strings.ToLower(row.WatchStatus) {
		case watchStatusOnHold:
			resp.Paused = append(resp.Paused, p)
		case watchStatusDropped:
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

// activePipelineStatuses are the episode statuses that count as live pipeline work
// for the Activity page ordering (a series with any such episode floats to the top).
var activePipelineStatuses = map[string]struct{}{
	"queued":      {},
	"downloading": {},
	"downloaded":  {},
	"encoding":    {},
	"encoded":     {},
}

// handleActivity returns every in-library series (subscribed OR has episodes) with
// its full episode record for the Activity page, so manually-downloaded
// unsubscribed series appear too. Ordering: series with any active pipeline episode
// first, then by most-recent episode activity (max episode modified_at); episodes
// within a series are newest-first.
func (h *Handler) handleActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.store.Read().ListSeriesWithProgress(ctx)
	if err != nil {
		h.logger.Error("activity: list series", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to list series")
		return
	}

	type ranked struct {
		s          ActivitySeries
		active     bool
		lastActive int64 // max episode modified_at, for the secondary sort
	}
	ranks := make([]ranked, 0, len(rows))

	for _, row := range rows {
		if row.Subscribed != 1 && row.EpisodeTotal == 0 {
			continue
		}
		eps, err := h.store.Read().ListEpisodesBySeries(ctx, row.ID)
		if err != nil {
			h.logger.Warn("activity: list episodes", "series_id", row.ID, "err", err)
			continue
		}
		details := make([]EpisodeDetail, 0, len(eps))
		var anyActive bool
		var lastActive int64
		for _, ep := range eps {
			if _, ok := activePipelineStatuses[ep.Status]; ok {
				anyActive = true
			}
			if ep.ModifiedAt > lastActive {
				lastActive = ep.ModifiedAt
			}
			outputs, _ := h.store.Read().ListEncodedOutputsByEpisode(ctx, ep.ID)
			details = append(details, episodeToDetail(ep, row.Title, outputs))
		}
		// Episodes newest-first (most-recently-modified first, tie-break by id desc).
		sort.SliceStable(details, func(i, j int) bool {
			if details[i].ModifiedAt != details[j].ModifiedAt {
				return details[i].ModifiedAt > details[j].ModifiedAt
			}
			return details[i].ID > details[j].ID
		})
		ranks = append(ranks, ranked{
			s:          ActivitySeries{SeriesProgress: rowToProgress(row), Episodes: details},
			active:     anyActive,
			lastActive: lastActive,
		})
	}

	// Active-series first; then by most-recent episode activity (desc).
	sort.SliceStable(ranks, func(i, j int) bool {
		if ranks[i].active != ranks[j].active {
			return ranks[i].active
		}
		return ranks[i].lastActive > ranks[j].lastActive
	})

	out := ActivityResponse{Series: make([]ActivitySeries, 0, len(ranks))}
	for _, rk := range ranks {
		out.Series = append(out.Series, rk.s)
	}
	WriteJSON(w, http.StatusOK, out)
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

// ensureSeries returns the series row for an AniList id, creating it from AniList
// media when absent. The created row is NOT subscribed (subscribed = 0) and gets
// no feed — it is just the metadata row that backs episodes and the AniList-keyed
// selective-download path. Subscribe layers subscription on top. AniList being
// unreachable is tolerated: the row is created with a placeholder title and the
// metadata refresher fills it in later. created reports whether a new row was made.
func (h *Handler) ensureSeries(ctx context.Context, anilistID int64) (store.Series, bool, error) {
	existing, err := h.store.Read().GetSeriesByAnilistID(ctx, &anilistID)
	switch {
	case err == nil:
		return existing, false, nil
	case errors.Is(err, sql.ErrNoRows):
		now := time.Now().Unix()
		params := store.CreateSeriesParams{
			Uuid:         mustUUID(),
			SeasonNumber: 1,
			Subscribed:   0,
			Favorite:     0,
		}
		if h.anilist != nil {
			if m, e := h.anilist.GetMedia(ctx, int(anilistID)); e == nil {
				applyMediaToCreate(&params, m)
				params.MetadataRefreshedAt = &now
			} else {
				h.logger.Warn("ensure series: anilist fetch failed (proceeding without metadata)", "anilist_id", anilistID, "err", e)
			}
		}
		if params.Title == "" {
			params.AnilistID = &anilistID
			params.Title = fmt.Sprintf("AniList #%d", anilistID)
		}
		s, e := h.store.Write().CreateSeries(ctx, params)
		if e != nil {
			return store.Series{}, false, e
		}
		return s, true, nil
	default:
		return store.Series{}, false, err
	}
}

// handleTrackSeries is the single "Subscribe & track" path. It ensures the series
// row exists (creating it from AniList if new), then layers subscription on top:
// subscribed = 1, watch_status = watching, plus an enabled auto-feed so the poller
// auto-fetches new episodes. Idempotent: re-subscribing an existing series returns
// 200; a fresh series returns 201.
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

	series, created, err := h.ensureSeries(ctx, req.AnilistID)
	if err != nil {
		h.logger.Error("track: ensure series", "anilist_id", req.AnilistID, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to create series")
		return
	}

	// Layer subscription on top: subscribe + land on the polled watch status.
	if series.Subscribed != 1 {
		if e := h.store.Write().SetSeriesSubscribed(ctx, store.SetSeriesSubscribedParams{ID: series.ID, Subscribed: 1}); e != nil {
			h.logger.Error("track: subscribe", "id", series.ID, "err", e)
			WriteError(w, http.StatusInternalServerError, "failed to subscribe series")
			return
		}
	}
	if e := h.store.Write().SetSeriesWatchStatus(ctx, store.SetSeriesWatchStatusParams{ID: series.ID, WatchStatus: watchStatusWatching}); e != nil {
		h.logger.Error("track: set watching", "id", series.ID, "err", e)
		WriteError(w, http.StatusInternalServerError, "failed to subscribe series")
		return
	}

	feedID, err := h.ensureFeed(ctx, series.ID)
	if err != nil {
		h.logger.Error("track: ensure feed", "series_id", series.ID, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to create feed")
		return
	}

	progress, err := h.seriesProgress(ctx, series.ID)
	if err != nil {
		h.logger.Error("track: load progress", "series_id", series.ID, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to load series")
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	WriteJSON(w, status, TrackResponse{Series: progress, SeriesID: series.ID, FeedID: feedID})
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

// handleSetSeriesStatus is the generalized watch-status setter: it accepts a body
// {"status":"watching"|"on_hold"|"dropped"} and writes it. 'watching' is the only
// status the poller acts on; 'on_hold'/'dropped' stay tracked but are never polled.
// 'completed' is derived and rejected here. No status change ever deletes files.
func (h *Handler) handleSetSeriesStatus(w http.ResponseWriter, r *http.Request) {
	var req SetStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if _, ok := validWatchStatuses[status]; !ok {
		WriteError(w, http.StatusBadRequest, "status must be one of watching, on_hold, dropped")
		return
	}
	h.setWatchStatus(w, r, status)
}

// setWatchStatus is the shared body: validate the series exists, write the watch
// status, return the refreshed SeriesProgress.
func (h *Handler) setWatchStatus(w http.ResponseWriter, r *http.Request, status string) {
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
	if err := h.store.Write().SetSeriesWatchStatus(ctx, store.SetSeriesWatchStatusParams{ID: id, WatchStatus: status}); err != nil {
		h.logger.Error("set watch_status", "id", id, "status", status, "err", err)
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

// handleUnsubscribeSeries stops automation for a series without throwing away its
// download history: it clears subscribed and disables every feed (keeping the
// poller double-gate intact), leaving watch_status as-is. Episodes are never
// touched. Only when the series has zero episodes — nothing left to keep — is the
// row deleted (cascade), holding the "exists iff subscribed OR has episodes"
// invariant. It broadcasts series.updated with deleted:true only when the row was
// actually removed. Returns 200 with {deleted, series_id}.
func (h *Handler) handleUnsubscribeSeries(w http.ResponseWriter, r *http.Request) {
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

	if err := h.store.Write().SetSeriesSubscribed(ctx, store.SetSeriesSubscribedParams{ID: id, Subscribed: 0}); err != nil {
		h.logger.Error("unsubscribe: clear subscribed", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to unsubscribe series")
		return
	}
	// Disable every feed so the poller double-gate (subscribed AND feed.enabled)
	// stays consistent; re-subscribe re-enables via ensureFeed.
	if feeds, err := h.store.Read().ListFeedsBySeries(ctx, id); err == nil {
		for _, f := range feeds {
			if f.Enabled != 0 {
				if e := h.store.Write().SetFeedEnabled(ctx, store.SetFeedEnabledParams{ID: f.ID, Enabled: 0}); e != nil {
					h.logger.Warn("unsubscribe: disable feed", "feed_id", f.ID, "err", e)
				}
			}
		}
	} else {
		h.logger.Warn("unsubscribe: list feeds", "id", id, "err", err)
	}

	// Garbage-collect a now-orphaned row: unsubscribed AND no episodes to keep.
	deleted := false
	if count, err := h.store.Read().CountEpisodesBySeries(ctx, id); err == nil && count == 0 {
		if e := h.store.Write().DeleteSeries(ctx, id); e != nil {
			h.logger.Error("unsubscribe: delete empty series", "id", id, "err", e)
			WriteError(w, http.StatusInternalServerError, "failed to unsubscribe series")
			return
		}
		deleted = true
	}

	h.hub.Broadcast(events.TypeSeriesUpdated, map[string]any{
		"series_id": id,
		"deleted":   deleted,
	})
	WriteJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "series_id": id})
}

// handleAnilistAvailable runs an on-demand source search by AniList id and returns
// the source-available episodes not yet downloaded, for the per-episode "download"
// UI. It works with NO pre-existing DB row: if a series row exists for the id its
// local episode numbers are excluded; otherwise the exclusion set is empty. It
// never creates a series row.
func (h *Handler) handleAnilistAvailable(w http.ResponseWriter, r *http.Request) {
	anilistID, ok := parseAnilistID(w, r)
	if !ok {
		return
	}
	if h.registry == nil {
		WriteError(w, http.StatusServiceUnavailable, "provider registry not available")
		return
	}
	ctx := r.Context()

	media, ok := h.searchMediaForAnilist(w, ctx, int64(anilistID))
	if !ok {
		return
	}

	// Episode numbers already present locally (any status) are excluded so the
	// list shows only genuinely-new source availability. No row -> nothing to exclude.
	have := map[int]struct{}{}
	if series, err := h.store.Read().GetSeriesByAnilistID(ctx, i64ptr(int64(anilistID))); err == nil {
		if eps, e := h.store.Read().ListEpisodesBySeries(ctx, series.ID); e == nil {
			for _, ep := range eps {
				if ep.EpisodeNo != nil {
					have[int(*ep.EpisodeNo)] = struct{}{}
				}
			}
		}
	}

	episodes, warnings := h.searchAvailable(ctx, media, have)
	WriteJSON(w, http.StatusOK, AvailableResponse{Episodes: episodes, Warnings: warnings})
}

// handleAnilistDownload downloads one source-found episode by AniList id. It
// ensures the series row exists (creating it WITHOUT subscribing), finds-or-creates
// the (series_id, number) episode, and drives it into the pipeline as 'queued'. It
// never mutates subscription or watch_status — a manual grab is independent of
// subscription. Idempotent: a second call for the same number re-enqueues the
// existing row. Returns 200 with {series_id, episode_id}.
func (h *Handler) handleAnilistDownload(w http.ResponseWriter, r *http.Request) {
	anilistID, ok := parseAnilistID(w, r)
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
	series, _, err := h.ensureSeries(ctx, int64(anilistID))
	if err != nil {
		h.logger.Error("anilist download: ensure series", "anilist_id", anilistID, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to create series")
		return
	}

	ep, err := h.enqueueAvailableEpisode(ctx, series, req)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]int64{"series_id": series.ID, "episode_id": ep.ID})
}

// searchMediaForAnilist resolves the source.Media used to drive SmartSearch for an
// AniList id, preferring a cached series row and falling back to a live AniList
// fetch (so a never-subscribed id still searches). On total failure it writes the
// error response and returns ok=false.
func (h *Handler) searchMediaForAnilist(w http.ResponseWriter, ctx context.Context, anilistID int64) (source.Media, bool) {
	if series, err := h.store.Read().GetSeriesByAnilistID(ctx, &anilistID); err == nil {
		return mediaFromSeries(series), true
	}
	if h.anilist != nil {
		if m, err := h.anilist.GetMedia(ctx, int(anilistID)); err == nil {
			return mediaFromMedia(m), true
		} else {
			h.logger.Info("available: anilist fetch failed", "anilist_id", anilistID, "err", err)
		}
	}
	WriteError(w, http.StatusServiceUnavailable, "AniList unavailable; cannot resolve series to search")
	return source.Media{}, false
}

// searchAvailable runs the registry SmartSearch for one media and returns the best
// release per episode number (excluding numbers in have), plus a warning per failed
// provider. Shared by the AniList-keyed available + download paths.
func (h *Handler) searchAvailable(ctx context.Context, media source.Media, have map[int]struct{}) ([]AvailableEpisode, []string) {
	opts := source.SmartSearchOptions{Media: media, BestReleases: true}
	best := map[int]*source.AnimeTorrent{}
	var warnings []string
	for _, pid := range h.registry.List() {
		p, _ := h.registry.Get(pid)
		torrents, err := p.SmartSearch(ctx, opts)
		if err != nil {
			h.logger.Warn("available: provider error", "provider", pid, "err", err)
			warnings = append(warnings, fmt.Sprintf("%s: %s", pid, err))
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
	return episodes, warnings
}

// enqueueAvailableEpisode finds-or-creates the (series, number) episode for a
// chosen source release and drives it into the pipeline as 'queued', broadcasting
// episode.status. It does NOT touch subscription/watch state. Idempotent: an
// existing row for the number is re-enqueued rather than duplicated.
func (h *Handler) enqueueAvailableEpisode(ctx context.Context, series store.Series, req DownloadAvailableRequest) (store.Episode, error) {
	num := int64(req.Number)
	existing, err := h.store.Read().ListEpisodesBySeries(ctx, series.ID)
	if err != nil {
		h.logger.Error("download available: list episodes", "series_id", series.ID, "err", err)
		return store.Episode{}, errors.New("failed to load episodes")
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

	if !found {
		arg := store.CreateEpisodeParams{
			Uuid:       mustUUID(),
			SeriesID:   series.ID,
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
			h.logger.Error("download available: create episode", "series_id", series.ID, "number", req.Number, "err", err)
			return store.Episode{}, errors.New("failed to create episode")
		}
	}

	if err := h.store.Write().SetEpisodeStatus(ctx, store.SetEpisodeStatusParams{
		ID:     ep.ID,
		Status: "queued",
	}); err != nil {
		h.logger.Error("download available: set queued", "id", ep.ID, "err", err)
		return store.Episode{}, errors.New("failed to enqueue episode")
	}
	h.hub.Broadcast(events.TypeEpisodeStatus, map[string]any{
		"episode_id": ep.ID,
		"series_id":  series.ID,
		"status":     "queued",
	})
	return ep, nil
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
// (which carries the archive counts) for its id. Used by the track/status
// responses so they return the same shape the grids read.
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

// mediaFromMedia builds the source.Media used to drive SmartSearch directly from
// a freshly-fetched AniList Media (the never-in-DB path), mirroring the column
// mapping mediaFromSeries does from a cached row.
func mediaFromMedia(m anilist.Media) source.Media {
	out := source.Media{
		ID:           m.ID,
		IDMal:        m.IDMal,
		RomajiTitle:  m.RomajiTitle,
		Status:       m.Status,
		Format:       m.Format,
		EpisodeCount: -1,
	}
	if m.EnglishTitle != "" {
		v := m.EnglishTitle
		out.EnglishTitle = &v
	}
	if m.EpisodeCount > 0 {
		out.EpisodeCount = m.EpisodeCount
	}
	out.Synonyms = append([]string{}, m.Synonyms...)
	if m.RomajiTitle == "" && m.EnglishTitle != "" {
		out.RomajiTitle = m.EnglishTitle
	}
	return out
}

// i64ptr returns a pointer to an int64 literal, for the *int64 query args.
func i64ptr(v int64) *int64 { return &v }

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
