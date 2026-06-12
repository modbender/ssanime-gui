// Package poller drives feed polling. On an interval it asks the store for feeds
// that are due AND whose series is subscribed and whose derived status permits
// polling (the store's ListFeedsDueForPoll enforces the durable half of the
// app-flow rule; the poller enforces the computed completed/up-to-date half).
// For each due feed it fetches via the right provider, parses with habari,
// dedupes against the feed's seen_cache, and creates episode rows (status
// "queued") for genuinely-new releases. It never starts downloads — that is
// Phase 4's job; the poller only enqueues.
package poller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// queuedStatus is the status a freshly-enqueued episode row gets.
const queuedStatus = "queued"

// errProviderNotRegistered marks a feed whose provider id has no registered
// provider (or whose provider id is empty). The poller skips such feeds quietly
// rather than error-marking them, so a series tracked before any source is
// installed waits silently for one to arrive.
var errProviderNotRegistered = errors.New("poller: feed provider not registered")

// defaultInterval is how often the poller wakes to look for due feeds. Each feed
// also has its own interval_seconds; this is just the scheduler tick.
const defaultInterval = 60 * time.Second

// Store is the subset of the store API the poller needs (kept narrow for tests).
type Store interface {
	Read() *store.Queries
	Write() *store.Queries
}

// Poller polls due feeds and enqueues new episodes.
type Poller struct {
	store    Store
	registry *source.Registry
	hub      *events.Hub
	logger   *slog.Logger
	interval time.Duration

	now func() time.Time // injectable clock for tests

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// Option configures a Poller.
type Option func(*Poller)

// WithInterval overrides the scheduler tick.
func WithInterval(d time.Duration) Option {
	return func(p *Poller) {
		if d > 0 {
			p.interval = d
		}
	}
}

// WithClock injects a clock (tests).
func WithClock(now func() time.Time) Option {
	return func(p *Poller) {
		if now != nil {
			p.now = now
		}
	}
}

// New builds a poller.
func New(st Store, registry *source.Registry, hub *events.Hub, logger *slog.Logger, opts ...Option) *Poller {
	if logger == nil {
		logger = slog.Default()
	}
	p := &Poller{
		store:    st,
		registry: registry,
		hub:      hub,
		logger:   logger,
		interval: defaultInterval,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Start launches the polling loop in a goroutine. Safe to call once; Stop ends it.
func (p *Poller) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	p.started = true
	go p.loop(ctx)
}

// Stop ends the polling loop and waits for it to exit. Idempotent.
func (p *Poller) Stop() {
	p.mu.Lock()
	cancel, done := p.cancel, p.done
	p.started = false
	p.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (p *Poller) loop(ctx context.Context) {
	defer close(p.done)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	// Run one pass immediately so a fresh boot doesn't wait a full interval.
	p.PollDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.PollDue(ctx)
		}
	}
}

// PollDue runs one polling pass over every due feed. Exposed so tests and a
// manual "check now" trigger can drive a single pass without the loop.
func (p *Poller) PollDue(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("poller: recovered from panic", "panic", r)
		}
	}()

	now := p.now().Unix()
	feeds, err := p.store.Read().ListFeedsDueForPoll(ctx, &now)
	if err != nil {
		p.logger.Error("poller: list due feeds", "err", err)
		return
	}
	for _, feed := range feeds {
		if ctx.Err() != nil {
			return
		}
		if err := p.pollFeed(ctx, feed); err != nil {
			p.logger.Warn("poller: feed failed", "feed", feed.ID, "url", feed.Url, "err", err)
		}
	}
}

// pollFeed fetches one feed, dedupes against its seen_cache, enqueues new
// episodes, updates last_checked_at + seen_cache, and emits feed.checked.
func (p *Poller) pollFeed(ctx context.Context, feed store.Feed) error {
	provider, err := p.providerFor(feed)
	if err != nil {
		if errors.Is(err, errProviderNotRegistered) {
			p.logger.Debug("poller: skipping feed with unregistered provider",
				"feed", feed.ID, "site", deref(feed.Site))
			return nil
		}
		return p.markError(ctx, feed, err)
	}

	series, err := p.store.Read().GetSeries(ctx, feed.SeriesID)
	if err != nil {
		return p.markError(ctx, feed, fmt.Errorf("load series: %w", err))
	}

	// Derived-status check beyond what the SQL filter already did: stop polling a
	// FINISHED series whose every aired episode is already archived (completed).
	if p.isCompleted(ctx, series) {
		p.logger.Debug("poller: skipping completed series", "series", series.ID)
		now := p.now().Unix()
		return p.store.Write().MarkFeedChecked(ctx, store.MarkFeedCheckedParams{
			Now: &now, SeenCache: feed.SeenCache, ID: feed.ID,
		})
	}

	results, err := p.fetch(ctx, provider, feed, series)
	if err != nil {
		return p.markError(ctx, feed, err)
	}

	seen := loadSeenCache(feed.SeenCache)
	var created int
	for _, t := range results {
		key := dedupeKey(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := p.enqueue(ctx, series, t); err != nil {
			p.logger.Warn("poller: enqueue failed", "series", series.ID, "name", t.Name, "err", err)
			// Keep it out of seen so a transient failure retries next pass.
			delete(seen, key)
			continue
		}
		created++
	}

	now := p.now().Unix()
	cacheJSON := dumpSeenCache(seen)
	if err := p.store.Write().MarkFeedChecked(ctx, store.MarkFeedCheckedParams{
		Now: &now, SeenCache: &cacheJSON, ID: feed.ID,
	}); err != nil {
		return fmt.Errorf("mark feed checked: %w", err)
	}

	p.hub.Broadcast(events.TypeFeedChecked, map[string]any{
		"feed_id":   feed.ID,
		"series_id": series.ID,
		"found":     len(results),
		"created":   created,
		"at":        now,
	})
	p.logger.Info("poller: feed checked", "feed", feed.ID, "series", series.ID,
		"found", len(results), "created", created)
	return nil
}

// fetch runs the right provider call for the feed kind. A structured feed (a feed
// URL we can hand straight to the provider) uses SmartSearch driven by the
// series metadata; a provider-search feed does the same — both normalize to the
// same result shape, then autoselect narrows to the best original release.
func (p *Poller) fetch(ctx context.Context, provider source.Provider, feed store.Feed, series store.Series) ([]*source.AnimeTorrent, error) {
	media := mediaFromSeries(series)
	opts := source.SmartSearchOptions{
		Media:      media,
		Query:      feedQuery(feed, series),
		Resolution: resolutionFilter(feed),
	}
	results, err := provider.SmartSearch(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("smart search (%s): %w", provider.ID(), err)
	}

	// Narrow to the best original release per episode using autoselect: this is
	// the "prefer trusted group + target resolution" rule. We keep one best
	// release per distinct episode number so a weekly feed enqueues exactly the
	// new episode(s), not every group's copy.
	return p.bestPerEpisode(media, feed, results), nil
}

// bestPerEpisode groups results by episode and runs SelectBest within each group,
// returning one winning release per episode (and one per batch).
func (p *Poller) bestPerEpisode(media source.Media, feed store.Feed, results []*source.AnimeTorrent) []*source.AnimeTorrent {
	res := resolutionFilter(feed)
	byEpisode := map[int][]*source.AnimeTorrent{}
	for _, t := range results {
		byEpisode[t.EpisodeNumber] = append(byEpisode[t.EpisodeNumber], t)
	}
	var out []*source.AnimeTorrent
	for ep, group := range byEpisode {
		opts := source.SelectOptions{Resolution: res, PreferBatch: ep < 0}
		if ep > 0 {
			opts.Episode = ep
		}
		best, err := source.SelectBest(media, group, opts)
		if err != nil {
			continue
		}
		out = append(out, best)
	}
	return out
}

// enqueue creates a queued episode row from a selected release.
func (p *Poller) enqueue(ctx context.Context, series store.Series, t *source.AnimeTorrent) error {
	arg := store.CreateEpisodeParams{
		Uuid:       newUUID(),
		SeriesID:   series.ID,
		SourceKind: "torrent",
		Status:     queuedStatus,
		ProfileID:  series.DefaultProfileID,
		Uncensored: 0,
		Bluray:     0,
	}
	if title := strings.TrimSpace(t.Name); title != "" {
		arg.Title = &title
	}
	if t.EpisodeNumber > 0 {
		ep := int64(t.EpisodeNumber)
		arg.EpisodeNo = &ep
	}
	if t.Link != "" {
		link := t.Link
		arg.SourceUrl = &link
	}
	if t.Magnet != "" {
		mag := t.Magnet
		arg.Magnet = &mag
	}
	if t.ReleaseGroup != "" {
		rg := t.ReleaseGroup
		arg.ReleaseGroup = &rg
	}
	if res := int64(resolutionInt(t.Resolution)); res > 0 {
		arg.Resolution = &res
	}
	if t.Date != "" {
		if ts, err := time.Parse(time.RFC3339, t.Date); err == nil {
			pub := ts.Unix()
			arg.PublishedAt = &pub
		}
	}
	ep, err := p.store.Write().CreateEpisode(ctx, arg)
	if err != nil {
		return err
	}
	p.hub.Broadcast(events.TypeEpisodeStatus, map[string]any{
		"episode_id": ep.ID,
		"series_id":  series.ID,
		"status":     queuedStatus,
	})
	return nil
}

// markError records a feed error and stamps last_checked_at so a broken feed
// doesn't get retried every tick.
func (p *Poller) markError(ctx context.Context, feed store.Feed, cause error) error {
	now := p.now().Unix()
	msg := cause.Error()
	_ = p.store.Write().MarkFeedError(ctx, store.MarkFeedErrorParams{
		Now: &now, ErrorMessage: &msg, ID: feed.ID,
	})
	return cause
}

// providerFor maps a feed to its provider. feeds.site holds the provider id (an
// extension ext_id). A feed with no site, or a site whose provider isn't
// registered, returns errProviderNotRegistered so the caller skips it quietly.
func (p *Poller) providerFor(feed store.Feed) (source.Provider, error) {
	id := ""
	if feed.Site != nil {
		id = strings.TrimSpace(*feed.Site)
	}
	if id == "" {
		return nil, errProviderNotRegistered
	}
	provider, ok := p.registry.Get(id)
	if !ok {
		return nil, errProviderNotRegistered
	}
	return provider, nil
}
