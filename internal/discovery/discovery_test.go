package discovery

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/anilist"
)

// fakeAniList returns canned media per spec, or a fixed error, recording how many
// times each feed key was fetched so a test can assert serve-stale behavior.
type fakeAniList struct {
	mu      sync.Mutex
	byKey   map[string][]anilist.Media
	err     error
	calls   int
	callLog []string
}

func (f *fakeAniList) ListByFeed(_ context.Context, spec anilist.FeedSpec) ([]anilist.Media, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.callLog = append(f.callLog, anilist.DescribeFeed(spec))
	if f.err != nil {
		return nil, f.err
	}
	// Key by sort+genre so trending vs popular vs genre rows are distinguishable.
	key := string(spec.Sort)
	if spec.Genre != "" {
		key = "g:" + spec.Genre
	}
	return f.byKey[key], nil
}

func newService(t *testing.T, al AniList) *Service {
	t.Helper()
	return New(al, slog.Default())
}

// fakeLogos records every id it was asked to resolve and returns a per-id logo,
// or a fixed error, so a test can assert which items were enriched and that a
// failing lookup degrades to "".
type fakeLogos struct {
	mu     sync.Mutex
	asked  []int
	byID   map[int]string
	err    error
	errIDs map[int]bool // ids that error (when err is nil, per-id)
}

func (f *fakeLogos) GetClearLogo(_ context.Context, id int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, id)
	if f.err != nil {
		return "", f.err
	}
	if f.errIDs[id] {
		return "", errors.New("anizip: simulated lookup failure")
	}
	return f.byID[id], nil
}

func (f *fakeLogos) askedIDs() map[int]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[int]bool, len(f.asked))
	for _, id := range f.asked {
		out[id] = true
	}
	return out
}

// TestRefreshAllPopulatesCache verifies a successful pass caches every feed and a
// reader sees them without triggering a fetch.
func TestRefreshAllPopulatesCache(t *testing.T) {
	fake := &fakeAniList{byKey: map[string][]anilist.Media{
		string(anilist.SortTrending):   {{ID: 1, RomajiTitle: "Trending One"}},
		string(anilist.SortPopularity): {{ID: 2, RomajiTitle: "Popular One"}},
		"g:Action":                     {{ID: 3, RomajiTitle: "Action One"}},
		"g:Romance":                    {{ID: 4, RomajiTitle: "Romance One"}},
	}}
	svc := newService(t, fake)

	svc.RefreshAll(context.Background())

	snap := svc.Snapshot()
	if len(snap["trending"]) != 1 || snap["trending"][0].ID != 1 {
		t.Fatalf("trending not cached: %+v", snap["trending"])
	}
	// seasonal + popular_all_time both come from the popularity key.
	if len(snap["seasonal"]) != 1 || len(snap["popular_all_time"]) != 1 {
		t.Fatalf("popularity feeds not cached: seasonal=%+v popular=%+v", snap["seasonal"], snap["popular_all_time"])
	}
	if len(snap["genre_action"]) != 1 || snap["genre_action"][0].ID != 3 {
		t.Fatalf("action genre not cached: %+v", snap["genre_action"])
	}

	callsAfterRefresh := fake.calls
	_ = svc.Snapshot()
	if _, _, _ = svc.Feed("trending"); fake.calls != callsAfterRefresh {
		t.Fatalf("a reader triggered a live fetch: calls went %d -> %d", callsAfterRefresh, fake.calls)
	}
}

// TestServeStaleOnError verifies that when AniList errors on a later refresh, the
// previously-cached slices are retained (serve-stale), not blanked.
func TestServeStaleOnError(t *testing.T) {
	fake := &fakeAniList{byKey: map[string][]anilist.Media{
		string(anilist.SortTrending):   {{ID: 1, RomajiTitle: "Trending One"}},
		string(anilist.SortPopularity): {{ID: 2, RomajiTitle: "Popular One"}},
		"g:Action":                     {{ID: 3}},
		"g:Romance":                    {{ID: 4}},
	}}
	svc := newService(t, fake)

	// First pass succeeds and warms the cache.
	svc.RefreshAll(context.Background())
	before, fetchedBefore, ok := svc.Feed("trending")
	if !ok || len(before) != 1 {
		t.Fatalf("expected trending warmed, got ok=%v len=%d", ok, len(before))
	}

	// Second pass: AniList is rate-limited. The cache must be preserved unchanged.
	fake.err = errors.New("anilist: rate limited (429)")
	svc.RefreshAll(context.Background())

	after, fetchedAfter, ok := svc.Feed("trending")
	if !ok || len(after) != 1 || after[0].ID != 1 {
		t.Fatalf("serve-stale failed: trending=%+v ok=%v", after, ok)
	}
	if !fetchedAfter.Equal(fetchedBefore) {
		t.Fatalf("fetchedAt should be unchanged on error: before=%v after=%v", fetchedBefore, fetchedAfter)
	}
}

// TestColdCacheEmpty verifies a never-refreshed service returns an empty snapshot
// (so the server emits empty rows and the frontend skeletons/hides them) rather
// than nil-panicking.
func TestColdCacheEmpty(t *testing.T) {
	svc := newService(t, &fakeAniList{})
	snap := svc.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("cold cache should be empty, got %d entries", len(snap))
	}
	if _, _, ok := svc.Feed("trending"); ok {
		t.Fatalf("cold cache Feed should report not-fetched")
	}
}

// TestSeasonalFeedResolvesSeason verifies the seasonal feed fills a live
// season/year from the injected clock before querying.
func TestSeasonalFeedResolvesSeason(t *testing.T) {
	fake := &fakeAniList{byKey: map[string][]anilist.Media{
		string(anilist.SortPopularity): {{ID: 9}},
		string(anilist.SortTrending):   {{ID: 8}},
		"g:Action":                     {{ID: 7}},
		"g:Romance":                    {{ID: 6}},
	}}
	// Pin the clock to a known July (SUMMER) so we can assert resolution ran.
	clock := func() time.Time { return time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC) }
	svc := New(fake, slog.Default(), WithClock(clock))

	svc.RefreshAll(context.Background())

	fake.mu.Lock()
	defer fake.mu.Unlock()
	var sawSeasonal bool
	for _, label := range fake.callLog {
		if label == "season:SUMMER2026" {
			sawSeasonal = true
		}
	}
	if !sawSeasonal {
		t.Fatalf("seasonal feed did not resolve to SUMMER2026; calls=%v", fake.callLog)
	}
}

// TestStopBeforeStartIsSafe verifies Stop on a never-started service is a no-op.
func TestStopBeforeStartIsSafe(t *testing.T) {
	svc := newService(t, &fakeAniList{})
	svc.Stop() // must not panic or block
}

// trendingMedia builds n trending items with ids 1..n so cap/scope assertions
// have a stable shape; the other feeds get a single item each.
func feedsWithTrending(trending []anilist.Media) *fakeAniList {
	return &fakeAniList{byKey: map[string][]anilist.Media{
		string(anilist.SortTrending):   trending,
		string(anilist.SortPopularity): {{ID: 1001, RomajiTitle: "Popular One"}},
		"g:Action":                     {{ID: 1002}},
		"g:Romance":                    {{ID: 1003}},
	}}
}

// TestEnrichHeroLogosPopulatesTrendingTopN verifies the hero feed's leading
// items (capped at heroEnrichCap) get a ClearLogoURL that survives into the
// snapshot, while items past the cap and non-hero feeds stay empty.
func TestEnrichHeroLogosPopulatesTrendingTopN(t *testing.T) {
	// 15 trending items: ids 1..15. Cap is 12, so 13..15 must not be enriched.
	trending := make([]anilist.Media, 0, 15)
	logos := &fakeLogos{byID: map[int]string{}}
	for i := 1; i <= 15; i++ {
		trending = append(trending, anilist.Media{ID: i})
		logos.byID[i] = "https://artworks.thetvdb.com/logo/" + itoa(i) + ".png"
	}
	svc := New(feedsWithTrending(trending), slog.Default(), WithLogoFetcher(logos))

	svc.RefreshAll(context.Background())

	snap := svc.Snapshot()
	got := snap["trending"]
	if len(got) != 15 {
		t.Fatalf("trending len = %d, want 15", len(got))
	}
	for i := 0; i < heroEnrichCap; i++ {
		if got[i].ClearLogoURL == "" {
			t.Errorf("item %d (id=%d) should be enriched", i, got[i].ID)
		}
	}
	for i := heroEnrichCap; i < len(got); i++ {
		if got[i].ClearLogoURL != "" {
			t.Errorf("item %d (id=%d) past cap should be empty, got %q", i, got[i].ID, got[i].ClearLogoURL)
		}
	}

	// Only the bounded hero set was looked up: ids 1..12, never 13..15 or other feeds.
	asked := logos.askedIDs()
	if len(asked) != heroEnrichCap {
		t.Fatalf("looked up %d ids, want exactly %d (the bounded hero set)", len(asked), heroEnrichCap)
	}
	for id := 13; id <= 15; id++ {
		if asked[id] {
			t.Errorf("id %d past cap should not be looked up", id)
		}
	}
	if asked[1001] || asked[1002] || asked[1003] {
		t.Error("non-hero feed items must not be enriched")
	}
}

// TestEnrichHeroLogosBestEffort verifies a failing or empty lookup leaves
// ClearLogoURL "" and never breaks the feed: the slice is still cached intact.
func TestEnrichHeroLogosBestEffort(t *testing.T) {
	trending := []anilist.Media{{ID: 1}, {ID: 2}, {ID: 3}}
	logos := &fakeLogos{
		byID:   map[int]string{1: "https://artworks.thetvdb.com/logo/1.png"},
		errIDs: map[int]bool{2: true}, // id 2 errors; id 3 returns "" (no mapping)
	}
	svc := New(feedsWithTrending(trending), slog.Default(), WithLogoFetcher(logos))

	svc.RefreshAll(context.Background())

	got := svc.Snapshot()["trending"]
	if len(got) != 3 {
		t.Fatalf("feed broken by enrichment: len = %d, want 3", len(got))
	}
	if got[0].ClearLogoURL == "" {
		t.Error("id 1 should be enriched")
	}
	if got[1].ClearLogoURL != "" {
		t.Errorf("id 2 errored; want empty, got %q", got[1].ClearLogoURL)
	}
	if got[2].ClearLogoURL != "" {
		t.Errorf("id 3 had no logo; want empty, got %q", got[2].ClearLogoURL)
	}
}

// TestEnrichDisabledWithoutFetcher verifies that without a LogoFetcher the
// refresh still works and every item keeps an empty ClearLogoURL.
func TestEnrichDisabledWithoutFetcher(t *testing.T) {
	trending := []anilist.Media{{ID: 1}, {ID: 2}}
	svc := newService(t, feedsWithTrending(trending))

	svc.RefreshAll(context.Background())

	for _, m := range svc.Snapshot()["trending"] {
		if m.ClearLogoURL != "" {
			t.Errorf("id %d enriched without a fetcher: %q", m.ID, m.ClearLogoURL)
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
