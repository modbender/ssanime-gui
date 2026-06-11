// Package discovery maintains a server-side cache of AniList "discovery" feeds
// (trending, seasonal, all-time popular, genre rows) that power the home page.
// AniList is globally throttled (~30 req/min), so the home must never hit it per
// page-load: this service refreshes a small fixed set of feeds on an hourly loop
// and serves the cached slices to readers. On a rate-limit or network error it
// keeps the previously-cached slice for that feed and retries next tick
// (serve-stale), exactly like the metadata refresher — it never fails a reader,
// blocks shutdown, or crashes the daemon.
package discovery

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/modbender/ssanime-gui/internal/anilist"
)

const (
	// defaultInterval is how often every feed is re-fetched.
	defaultInterval = time.Hour
	// firstPassDelay holds the first refresh off briefly after boot so the rows
	// populate fast without competing with startup work.
	firstPassDelay = 5 * time.Second
	// fetchSpacing paces the sequential per-feed requests within one pass, keeping
	// the burst well under AniList's ~30 req/min limit (~5-7 feeds => ~1.5s).
	fetchSpacing = 250 * time.Millisecond
)

// AniList is the subset of the anilist client the service needs, so tests can
// swap in a fake that returns canned media (or a rate-limit error).
type AniList interface {
	ListByFeed(ctx context.Context, spec anilist.FeedSpec) ([]anilist.Media, error)
}

// FeedKey is the stable string identity of a discovery feed (the wire `key`).
type FeedKey string

// Feed pairs a stable key + display title with the AniList query spec. The
// static feedSpecs slice below is the ONE place to add or remove a home row.
type Feed struct {
	Key   FeedKey
	Title string
	Spec  anilist.FeedSpec
	// Seasonal marks the feed whose Season/SeasonYear are filled live (the current
	// airing season) at each fetch, so it tracks the calendar without a hardcoded date.
	Seasonal bool
}

// feedSpecs is the fixed set of discovery feeds shown on the home page, in the
// approved order. Add a row here and it appears in the cache and /api/discovery
// automatically.
var feedSpecs = []Feed{
	{Key: "trending", Title: "Trending Now", Spec: anilist.FeedSpec{Sort: anilist.SortTrending}},
	{Key: "seasonal", Title: "Popular This Season", Spec: anilist.FeedSpec{Sort: anilist.SortPopularity}, Seasonal: true},
	{Key: "popular_all_time", Title: "All-Time Popular", Spec: anilist.FeedSpec{Sort: anilist.SortPopularity}},
	{Key: "genre_action", Title: "Action", Spec: anilist.FeedSpec{Sort: anilist.SortPopularity, Genre: "Action"}},
	{Key: "genre_romance", Title: "Romance", Spec: anilist.FeedSpec{Sort: anilist.SortPopularity, Genre: "Romance"}},
}

// Feeds returns the static feed definitions (key + title) in display order, so
// the server can render rows even before the cache warms.
func Feeds() []Feed {
	out := make([]Feed, len(feedSpecs))
	copy(out, feedSpecs)
	return out
}

// entry is one cached feed slice plus when it was last successfully fetched.
type entry struct {
	media     []anilist.Media
	fetchedAt time.Time
}

// Service refreshes the discovery feeds on a loop and serves the cache. It
// mirrors the metadata refresher's lifecycle (Start/Stop, injectable clock, a
// top-level recover per pass).
type Service struct {
	anilist  AniList
	logger   *slog.Logger
	interval time.Duration

	now func() time.Time // injectable clock for tests

	mu    sync.RWMutex
	cache map[FeedKey]entry

	startMu sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// Option configures a Service.
type Option func(*Service)

// WithInterval overrides the refresh tick.
func WithInterval(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.interval = d
		}
	}
}

// WithClock injects a clock (tests).
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// New builds a discovery Service.
func New(al AniList, logger *slog.Logger, opts ...Option) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{
		anilist:  al,
		logger:   logger,
		interval: defaultInterval,
		now:      time.Now,
		cache:    make(map[FeedKey]entry),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start launches the refresh loop in a goroutine. Safe to call once; Stop ends it.
func (s *Service) Start() {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if s.started {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	s.started = true
	go s.loop(ctx)
}

// Stop ends the refresh loop and waits for it to exit. Idempotent.
func (s *Service) Stop() {
	s.startMu.Lock()
	cancel, done := s.cancel, s.done
	s.started = false
	s.startMu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (s *Service) loop(ctx context.Context) {
	defer close(s.done)

	// Hold the first pass off briefly so rows populate soon after boot, but abort
	// promptly if the daemon is already shutting down.
	select {
	case <-ctx.Done():
		return
	case <-time.After(firstPassDelay):
	}
	s.RefreshAll(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RefreshAll(ctx)
		}
	}
}

// RefreshAll fetches every feed once, sequentially with spacing. Exposed so tests
// and a manual trigger can drive a single pass. A top-level recover isolates a
// panic to this pass (the loop and daemon survive). Per-feed errors (rate limit
// or network) keep the previously-cached slice and are retried next tick.
func (s *Service) RefreshAll(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("discovery: recovered from panic", "panic", rec)
		}
	}()

	for i, f := range feedSpecs {
		if ctx.Err() != nil {
			return
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(fetchSpacing):
			}
		}
		spec := f.Spec
		if f.Seasonal {
			season, year := anilist.CurrentSeason(s.now())
			spec.Season = season
			spec.SeasonYear = year
		}
		media, err := s.anilist.ListByFeed(ctx, spec)
		if err != nil {
			// Serve-stale: keep the existing slice (if any), retry next tick. This
			// is expected under AniList's throttle, so info level.
			s.logger.Info("discovery: feed fetch failed; keeping cached slice",
				"feed", f.Key, "spec", anilist.DescribeFeed(spec), "err", err)
			continue
		}
		s.mu.Lock()
		s.cache[f.Key] = entry{media: media, fetchedAt: s.now()}
		s.mu.Unlock()
	}
}

// Snapshot returns a copy of every cached feed slice keyed by feed key. Readers
// never trigger a live fetch — only the loop does. A feed not yet fetched is
// absent from the map (the server emits an empty row for it).
func (s *Service) Snapshot() map[FeedKey][]anilist.Media {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[FeedKey][]anilist.Media, len(s.cache))
	for k, e := range s.cache {
		cp := make([]anilist.Media, len(e.media))
		copy(cp, e.media)
		out[k] = cp
	}
	return out
}

// Feed returns the cached slice for one key and when it was last fetched. The
// boolean reports whether the feed has been fetched at least once. Never triggers
// a live fetch.
func (s *Service) Feed(key FeedKey) ([]anilist.Media, time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.cache[key]
	if !ok {
		return nil, time.Time{}, false
	}
	cp := make([]anilist.Media, len(e.media))
	copy(cp, e.media)
	return cp, e.fetchedAt, true
}
