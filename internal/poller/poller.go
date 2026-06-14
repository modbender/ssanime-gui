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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modbender/ssanime-gui/internal/anizip"
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

// EpisodeResolver resolves a series' per-episode metadata (air dates) from
// ani.zip, so the poller queries only episodes that have actually aired and sit
// above the backfill watermark. *anizip.Client satisfies it. Nullable: when nil
// the poller falls back to the series' EpisodeCount.
type EpisodeResolver interface {
	GetEpisodes(ctx context.Context, anilistID int) ([]anizip.Episode, error)
}

// Poller polls due feeds and enqueues new episodes.
type Poller struct {
	store    Store
	registry *source.Registry
	hub      *events.Hub
	logger   *slog.Logger
	interval time.Duration
	resolver EpisodeResolver // ani.zip air-date resolver; nil falls back to EpisodeCount

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

// WithResolver injects the ani.zip episode resolver that gates auto-download to
// aired episodes. Without it the poller falls back to the series' EpisodeCount.
func WithResolver(r EpisodeResolver) Option {
	return func(p *Poller) {
		if r != nil {
			p.resolver = r
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
	// Load the user-configured trusted-group allowlist once per cycle (not per
	// feed/episode) and pass it down to selection.
	trustedGroups := p.trustedReleaseGroups(ctx)
	for _, feed := range feeds {
		if ctx.Err() != nil {
			return
		}
		if err := p.pollFeed(ctx, feed, trustedGroups); err != nil {
			p.logger.Warn("poller: feed failed", "feed", feed.ID, "url", feed.Url, "err", err)
		}
	}
}

// trustedReleaseGroups reads the user-configured trusted-group allowlist from
// settings. A non-nil slice (empty = "no trust filter") overrides the package
// default; on read failure it returns nil so selection falls back to the default.
func (p *Poller) trustedReleaseGroups(ctx context.Context) []string {
	set, err := p.store.Read().GetSettings(ctx)
	if err != nil {
		p.logger.Warn("poller: read settings for trusted groups", "err", err)
		return nil
	}
	return decodeTrustedGroups(set.TrustedReleaseGroups)
}

// decodeTrustedGroups parses the settings JSON-array trusted_release_groups value.
// Blank/invalid → the package default; an explicit '[]' → an empty (non-nil) slice,
// the "no trust filter" signal.
func decodeTrustedGroups(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return append([]string(nil), source.TrustedReleaseGroups...)
	}
	var groups []string
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return append([]string(nil), source.TrustedReleaseGroups...)
	}
	if groups == nil {
		groups = []string{}
	}
	return groups
}

// pollFeed fetches one feed, dedupes against its seen_cache, enqueues new
// episodes, updates last_checked_at + seen_cache, and emits feed.checked.
func (p *Poller) pollFeed(ctx context.Context, feed store.Feed, trustedGroups []string) error {
	// Gate: the feed must map to a registered provider. The per-episode search
	// below drives the actual provider calls through the registry, so the
	// returned provider itself is unused here.
	if _, err := p.providerFor(feed); err != nil {
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

	have := p.haveEpisodes(ctx, series)
	nums, airDates := p.episodesToQuery(ctx, series, have)

	now := p.now().Unix()
	if len(nums) == 0 {
		// Nothing new to chase (not yet aired, or all aired episodes already in
		// library). Still stamp last_checked and broadcast so the UI sees a pass.
		if err := p.store.Write().MarkFeedChecked(ctx, store.MarkFeedCheckedParams{
			Now: &now, SeenCache: feed.SeenCache, ID: feed.ID,
		}); err != nil {
			return fmt.Errorf("mark feed checked: %w", err)
		}
		p.hub.Broadcast(events.TypeFeedChecked, map[string]any{
			"feed_id": feed.ID, "series_id": series.ID, "found": 0, "created": 0, "at": now,
		})
		return nil
	}

	media := mediaFromSeries(series)
	base := source.SmartSearchOptions{
		Query:      feedQuery(feed, series),
		Resolution: resolutionFilter(feed),
	}
	candidates, warnings := source.SearchEpisodes(ctx, p.registry, media, nums, base)
	for _, msg := range warnings {
		p.logger.Warn("poller: source search", "feed", feed.ID, "series", series.ID, "detail", msg)
	}

	res := resolutionFilter(feed)
	lockGroup := ""
	if series.LockedReleaseGroup != nil {
		lockGroup = strings.TrimSpace(*series.LockedReleaseGroup)
	}
	seen := loadSeenCache(feed.SeenCache)
	var found, created int
	for _, ep := range nums {
		group := candidates[ep]
		if len(group) == 0 {
			continue
		}
		found++
		var best *source.AnimeTorrent
		if lockGroup != "" {
			// Stage 1: locked group, trusted-only.
			if b, err := source.SelectBest(media, group, source.SelectOptions{
				Group: lockGroup, RequireTrustedGroup: true, Resolution: res, Episode: ep, TrustedGroups: trustedGroups,
			}); err == nil {
				best = b
			}
			// Stage 2: locked group missing AND past air+24h → fall back to any trusted group.
			if best == nil {
				if ad, ok := airDates[ep]; ok && !p.now().Before(ad.Add(24*time.Hour)) {
					if b, err := source.SelectBest(media, group, source.SelectOptions{
						RequireTrustedGroup: true, Resolution: res, Episode: ep, TrustedGroups: trustedGroups,
					}); err == nil {
						best = b
					}
				}
				// else: still within 24h of air, or no known air date → skip, wait for next poll.
			}
		} else {
			// First episode (no lock yet): trusted-only; enqueue sets the lock.
			if b, err := source.SelectBest(media, group, source.SelectOptions{
				RequireTrustedGroup: true, Resolution: res, Episode: ep, TrustedGroups: trustedGroups,
			}); err == nil {
				best = b
			}
		}
		if best == nil {
			continue
		}
		key := dedupeKey(best)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := p.enqueue(ctx, series, best); err != nil {
			p.logger.Warn("poller: enqueue failed", "series", series.ID, "name", best.Name, "err", err)
			// Keep it out of seen so a transient failure retries next pass.
			delete(seen, key)
			continue
		}
		created++
	}

	cacheJSON := dumpSeenCache(seen)
	if err := p.store.Write().MarkFeedChecked(ctx, store.MarkFeedCheckedParams{
		Now: &now, SeenCache: &cacheJSON, ID: feed.ID,
	}); err != nil {
		return fmt.Errorf("mark feed checked: %w", err)
	}

	p.hub.Broadcast(events.TypeFeedChecked, map[string]any{
		"feed_id":   feed.ID,
		"series_id": series.ID,
		"found":     found,
		"created":   created,
		"at":        now,
	})
	p.logger.Info("poller: feed checked", "feed", feed.ID, "series", series.ID,
		"found", found, "created", created)
	return nil
}

// haveEpisodes returns the set of episode numbers already present for a series
// (any status), so the poller never re-queries an episode already in the library.
func (p *Poller) haveEpisodes(ctx context.Context, series store.Series) map[int]struct{} {
	have := map[int]struct{}{}
	eps, err := p.store.Read().ListEpisodesBySeries(ctx, series.ID)
	if err != nil {
		p.logger.Warn("poller: list episodes", "series", series.ID, "err", err)
		return have
	}
	for _, e := range eps {
		if e.EpisodeNo != nil {
			have[int(*e.EpisodeNo)] = struct{}{}
		}
	}
	return have
}

// episodesToQuery resolves the genuinely-new episode numbers to auto-download:
// those above the backfill watermark, not already in the library, and (when the
// ani.zip resolver is wired) already aired. Without a resolver or AniList id it
// falls back to the series' EpisodeCount — bounded by the watermark and have-set
// but without an air-date gate. Returns sorted unique numbers (usually 0-2).
func (p *Poller) episodesToQuery(ctx context.Context, series store.Series, have map[int]struct{}) ([]int, map[int]time.Time) {
	floor := 0
	if series.BackfillFromEpisode != nil {
		floor = int(*series.BackfillFromEpisode)
	}

	airDates := map[int]time.Time{}
	var out []int
	if p.resolver != nil && series.AnilistID != nil {
		if eps, err := p.resolver.GetEpisodes(ctx, int(*series.AnilistID)); err == nil {
			now := p.now()
			for _, e := range eps {
				ad := strings.TrimSpace(e.AirDate)
				if ad == "" {
					continue // unknown air date: treat as not yet aired
				}
				airTime, perr := time.Parse("2006-01-02", ad)
				if perr != nil || airTime.After(now) {
					continue
				}
				if e.Number <= floor {
					continue
				}
				if _, ok := have[e.Number]; ok {
					continue
				}
				out = append(out, e.Number)
				airDates[e.Number] = airTime
			}
		} else {
			p.logger.Info("poller: anizip resolve failed", "series", series.ID, "err", err)
		}
	}

	// Fallback: resolver nil/erroring, no AniList id, or zero usable aired episodes.
	if len(out) == 0 && series.EpisodeCount != nil && *series.EpisodeCount > 0 {
		for n := 1; n <= int(*series.EpisodeCount); n++ {
			if n <= floor {
				continue
			}
			if _, ok := have[n]; ok {
				continue
			}
			out = append(out, n)
		}
	}

	sort.Ints(out)
	return dedupeInts(out), airDates
}

// dedupeInts removes adjacent duplicates from a sorted slice in place.
func dedupeInts(in []int) []int {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, n := range in[1:] {
		if n != out[len(out)-1] {
			out = append(out, n)
		}
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
	// Lock the series to this release's group on the first downloaded episode, so
	// subsequent episodes prefer the same group. Never overwrite an existing lock.
	if group := strings.TrimSpace(t.ReleaseGroup); group != "" &&
		(series.LockedReleaseGroup == nil || strings.TrimSpace(*series.LockedReleaseGroup) == "") {
		if e := p.store.Write().UpdateSeriesLockedGroup(ctx, store.UpdateSeriesLockedGroupParams{
			LockedReleaseGroup: &group, ID: series.ID,
		}); e != nil {
			p.logger.Warn("poller: lock release group", "series", series.ID, "group", group, "err", e)
		}
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
