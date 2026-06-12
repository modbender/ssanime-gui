package discovery

import (
	"context"
	"errors"
	"log/slog"
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
