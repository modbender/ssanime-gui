// Package metadata keeps each subscribed, still-airing series' AniList metadata
// fresh in the local DB so the poller's "stop polling completed series" rule
// reacts when a show flips RELEASING -> FINISHED upstream. AniList's API is
// globally throttled to ~30 req/min, so the refresher treats AniList as a
// cache-first enrichment layer: it batches hard (up to 50 ids per request),
// refreshes rarely, and on any rate-limit or network error it serves the
// existing DB data and tries again next tick — it never fails a user operation,
// blocks shutdown, or crashes the daemon.
package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/modbender/ssanime-gui/internal/anilist"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

const (
	// defaultInterval is how often the refresher wakes to look for stale series.
	defaultInterval = 3 * time.Hour
	// defaultStaleness is how old a series' metadata must be before it is eligible
	// for a refresh — well above the interval so a series isn't re-fetched every tick.
	defaultStaleness = 24 * time.Hour
	// defaultLimit caps how many series one pass refreshes, bounding the per-pass
	// request count (one batch request per 50 series) against the rate limit.
	defaultLimit = 50
	// firstPassDelay holds the first pass off until shortly after boot so startup
	// (migrations, seeding, binary provisioning) isn't competing for the rate limit.
	firstPassDelay = 90 * time.Second
)

// Store is the subset of the store API the refresher needs (kept narrow for tests).
type Store interface {
	Read() *store.Queries
	Write() *store.Queries
}

// AniList is the subset of the anilist client the refresher needs, so tests can
// swap in a fake that returns canned media (or a rate-limit error).
type AniList interface {
	GetMediaBatch(ctx context.Context, ids []int) (map[int]anilist.Media, error)
	GetMedia(ctx context.Context, id int) (anilist.Media, error)
}

// Refresher periodically refreshes stale AniList metadata for subscribed,
// non-finished series. It mirrors the poller's lifecycle (Start/Stop, an
// injectable clock, a top-level recover per pass).
type Refresher struct {
	store     Store
	anilist   AniList
	hub       *events.Hub
	logger    *slog.Logger
	interval  time.Duration
	staleness time.Duration
	limit     int64

	now func() time.Time // injectable clock for tests

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// Option configures a Refresher.
type Option func(*Refresher)

// WithInterval overrides the scheduler tick.
func WithInterval(d time.Duration) Option {
	return func(r *Refresher) {
		if d > 0 {
			r.interval = d
		}
	}
}

// WithStaleness overrides how old metadata must be to be eligible for refresh.
func WithStaleness(d time.Duration) Option {
	return func(r *Refresher) {
		if d > 0 {
			r.staleness = d
		}
	}
}

// WithLimit overrides how many series a single pass refreshes.
func WithLimit(n int64) Option {
	return func(r *Refresher) {
		if n > 0 {
			r.limit = n
		}
	}
}

// WithClock injects a clock (tests).
func WithClock(now func() time.Time) Option {
	return func(r *Refresher) {
		if now != nil {
			r.now = now
		}
	}
}

// New builds a Refresher.
func New(st Store, al AniList, hub *events.Hub, logger *slog.Logger, opts ...Option) *Refresher {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Refresher{
		store:     st,
		anilist:   al,
		hub:       hub,
		logger:    logger,
		interval:  defaultInterval,
		staleness: defaultStaleness,
		limit:     defaultLimit,
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Start launches the refresh loop in a goroutine. Safe to call once; Stop ends it.
func (r *Refresher) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	r.started = true
	go r.loop(ctx)
}

// Stop ends the refresh loop and waits for it to exit. Idempotent.
func (r *Refresher) Stop() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.started = false
	r.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (r *Refresher) loop(ctx context.Context) {
	defer close(r.done)

	// Hold the first pass off until shortly after boot, but abort promptly if the
	// daemon is already shutting down.
	select {
	case <-ctx.Done():
		return
	case <-time.After(firstPassDelay):
	}
	r.RefreshDue(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.RefreshDue(ctx)
		}
	}
}

// RefreshDue runs one refresh pass over the stale-series batch. Exposed so tests
// and a manual trigger can drive a single pass. A top-level recover isolates a
// panic to this pass (the loop and daemon survive). On a batch error (rate limit
// or network) it logs and returns without stamping metadata_refreshed_at, so the
// same series are retried next tick.
func (r *Refresher) RefreshDue(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("metadata: recovered from panic", "panic", rec)
		}
	}()

	cutoff := r.now().Add(-r.staleness).Unix()
	series, err := r.store.Read().ListSeriesForMetadataRefresh(ctx, store.ListSeriesForMetadataRefreshParams{
		MetadataRefreshedAt: &cutoff,
		Limit:               r.limit,
	})
	if err != nil {
		r.logger.Error("metadata: list stale series", "err", err)
		return
	}
	if len(series) == 0 {
		return
	}

	ids := make([]int, 0, len(series))
	for _, s := range series {
		if s.AnilistID != nil {
			ids = append(ids, int(*s.AnilistID))
		}
	}

	media, err := r.anilist.GetMediaBatch(ctx, ids)
	if err != nil {
		// Rate-limit / network error: serve existing DB data, leave rows untouched,
		// retry next tick. This is expected under AniList's throttle, so info level.
		r.logger.Info("metadata: batch fetch failed; keeping existing metadata", "count", len(ids), "err", err)
		return
	}

	now := r.now().Unix()
	var updated int
	for _, s := range series {
		if ctx.Err() != nil {
			return
		}
		if s.AnilistID == nil {
			continue
		}
		m, ok := media[int(*s.AnilistID)]
		if !ok {
			continue
		}
		if err := r.store.Write().UpdateSeriesMetadata(ctx, updateParams(m, s.ID, now)); err != nil {
			r.logger.Warn("metadata: update series failed", "series", s.ID, "err", err)
			continue
		}
		updated++
		r.broadcastUpdated(s.ID, m)
	}
	r.logger.Info("metadata: refresh pass complete", "stale", len(series), "updated", updated)
}

// RefreshSeries refreshes a single series by row id (the manual endpoint). It
// fetches the one media live and writes it through. A series with no anilist_id
// is a clear, non-retryable error; a rate-limit/network error is returned so the
// handler can map it to 503 with the existing metadata kept. Never panics.
func (r *Refresher) RefreshSeries(ctx context.Context, id int64) (store.Series, error) {
	s, err := r.store.Read().GetSeries(ctx, id)
	if err != nil {
		return store.Series{}, err
	}
	if s.AnilistID == nil {
		return store.Series{}, ErrNoAnilistID
	}

	m, err := r.anilist.GetMedia(ctx, int(*s.AnilistID))
	if err != nil {
		return store.Series{}, fmt.Errorf("anilist fetch: %w", err)
	}

	now := r.now().Unix()
	if err := r.store.Write().UpdateSeriesMetadata(ctx, updateParams(m, s.ID, now)); err != nil {
		return store.Series{}, fmt.Errorf("update series metadata: %w", err)
	}
	updated, err := r.store.Read().GetSeries(ctx, id)
	if err != nil {
		return store.Series{}, err
	}
	r.broadcastUpdated(id, m)
	return updated, nil
}

// ErrNoAnilistID is returned when a refresh is requested for a series that has no
// AniList id to refresh from.
var ErrNoAnilistID = errors.New("series has no anilist_id to refresh from")

func (r *Refresher) broadcastUpdated(seriesID int64, m anilist.Media) {
	if r.hub == nil {
		return
	}
	r.hub.Broadcast(events.TypeSeriesUpdated, map[string]any{
		"series_id":     seriesID,
		"anilist_id":    m.ID,
		"airing_status": m.Status,
	})
}

// updateParams maps a fetched Media to the UpdateSeriesMetadata params via the
// shared anilist mapper, so the field list lives in exactly one place. The
// preserve-on-empty columns are passed as plain (possibly empty) strings so the
// query's COALESCE(NULLIF(?, ”), col) keeps the existing value on a blank.
func updateParams(m anilist.Media, seriesID, now int64) store.UpdateSeriesMetadataParams {
	f := anilist.MediaToSeriesFields(m)
	return store.UpdateSeriesMetadataParams{
		Status:         f.Status,
		AiringStatus:   f.AiringStatus,
		EpisodeCount:   f.EpisodeCount,
		Format:         f.Format,
		Season:         f.Season,
		SeasonYear:     f.SeasonYear,
		CoverImageUrl:  deref(f.CoverImage),
		BannerImageUrl: deref(f.BannerImage),
		CoverColor:     deref(f.CoverColor),
		RomajiTitle:    deref(f.RomajiTitle),
		EnglishTitle:   deref(f.EnglishTitle),
		Synonyms:       deref(f.Synonyms),
		Now:            &now,
		ID:             seriesID,
	}
}

// deref returns the empty string for a nil *string, else its value. The empty
// string drives NULLIF(?, ”) -> NULL so COALESCE preserves the existing column.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
