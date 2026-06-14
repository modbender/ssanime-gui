package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// fakeProvider is a source.Provider that records the episode numbers it was
// SmartSearched for and returns a canned per-episode release (or an error). Only
// the methods searchAvailable exercises carry behaviour; the rest are stubs.
type fakeProvider struct {
	id string
	// searchFn yields the releases (or error) for one SmartSearch call.
	searchFn func(opts source.SmartSearchOptions) ([]*source.AnimeTorrent, error)

	mu      sync.Mutex
	queried []int // episode numbers seen, in arrival order
}

func (f *fakeProvider) ID() string { return f.id }

func (f *fakeProvider) SmartSearch(_ context.Context, opts source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
	f.mu.Lock()
	f.queried = append(f.queried, opts.EpisodeNumber)
	f.mu.Unlock()
	return f.searchFn(opts)
}

func (f *fakeProvider) queriedSet() map[int]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[int]bool, len(f.queried))
	for _, n := range f.queried {
		out[n] = true
	}
	return out
}

func (f *fakeProvider) Search(context.Context, source.SearchOptions) ([]*source.AnimeTorrent, error) {
	return nil, nil
}
func (f *fakeProvider) GetLatest(context.Context) ([]*source.AnimeTorrent, error) { return nil, nil }
func (f *fakeProvider) GetTorrentMagnetLink(_ context.Context, t *source.AnimeTorrent) (string, error) {
	return t.Magnet, nil
}
func (f *fakeProvider) GetTorrentInfoHash(_ context.Context, t *source.AnimeTorrent) (string, error) {
	return t.InfoHash, nil
}
func (f *fakeProvider) GetSettings() source.Settings { return source.Settings{CanSmartSearch: true} }

// fakeResolver is an AnizipFetcher whose GetIDs returns a fixed episode-number
// map, so searchAvailable resolves a known episode set without a network call.
type fakeResolver struct {
	episodes map[int]anizip.EpisodeIDs
	err      error
}

func (f fakeResolver) GetEpisodes(context.Context, int) ([]anizip.Episode, error) { return nil, nil }
func (f fakeResolver) GetIDs(_ context.Context, anilistID int) (anizip.IDs, error) {
	if f.err != nil {
		return anizip.IDs{}, f.err
	}
	return anizip.IDs{AnilistID: anilistID, Episodes: f.episodes}, nil
}

// newAvailableHandler builds a Handler wired with a registry and ani.zip resolver
// for the in-package searchAvailable tests, plus the store for endpoint tests.
func newAvailableHandler(t *testing.T, reg *source.Registry, anizip AnizipFetcher) (*Handler, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: filepath.Join(dir, "avail.db"), Port: config.DefaultPort}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)
	return &Handler{store: st, hub: hub, logger: slog.Default(), registry: reg, anizip: anizip}, st
}

// epReleases builds a single canned release for the queried episode with a given
// seeder count, so per-episode best-by-seeders bucketing is observable.
func epRelease(name string, ep, seeders int) *source.AnimeTorrent {
	return &source.AnimeTorrent{
		Name:          name,
		Magnet:        fmt.Sprintf("magnet:?xt=urn:btih:%s", name),
		EpisodeNumber: ep,
		Seeders:       seeders,
		Resolution:    "1080p",
		// Confirmed bypasses SelectBest's title-match filter; these canned names
		// don't carry the media title, and best-by-seeders is what the tests assert.
		Confirmed: true,
	}
}

// TestSearchAvailablePerEpisode verifies searchAvailable now issues one SmartSearch
// per ani.zip episode number per provider, buckets the best (highest-seeded) release
// per episode, excludes already-have numbers, and reports nothing when sources work.
func TestSearchAvailablePerEpisode(t *testing.T) {
	reg := source.NewRegistry()
	// Two providers; B returns a higher-seeded release so it wins per episode.
	pa := &fakeProvider{id: "a", searchFn: func(o source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
		return []*source.AnimeTorrent{epRelease("a-ep", o.EpisodeNumber, 5)}, nil
	}}
	pb := &fakeProvider{id: "b", searchFn: func(o source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
		return []*source.AnimeTorrent{epRelease("b-ep", o.EpisodeNumber, 50)}, nil
	}}
	reg.Register(pa)
	reg.Register(pb)

	res := fakeResolver{episodes: map[int]anizip.EpisodeIDs{
		1: {AnidbEid: 100}, 2: {AnidbEid: 101}, 3: {AnidbEid: 102},
	}}
	h, _ := newAvailableHandler(t, reg, res)

	media := source.Media{ID: 182205, RomajiTitle: "Test Show"}
	have := map[int]struct{}{2: {}} // already have ep 2 → excluded

	eps, warnings := h.searchAvailable(context.Background(), media, have)

	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none (all providers succeeded)", warnings)
	}
	// Each provider must have been queried for each NON-have episode (1 and 3).
	for _, p := range []*fakeProvider{pa, pb} {
		q := p.queriedSet()
		if !q[1] || !q[3] {
			t.Errorf("provider %s queried set = %v, want episodes 1 and 3", p.id, q)
		}
		if q[2] {
			t.Errorf("provider %s queried already-have episode 2", p.id)
		}
	}
	// Result: episodes 1 and 3, each won by provider b (higher seeders).
	if len(eps) != 2 {
		t.Fatalf("episodes = %+v, want 2 (1 and 3)", eps)
	}
	if eps[0].Number != 1 || eps[1].Number != 3 {
		t.Errorf("episode numbers = [%d,%d], want [1,3]", eps[0].Number, eps[1].Number)
	}
	for _, e := range eps {
		if e.Title != "b-ep" {
			t.Errorf("episode %d title = %q, want b-ep (higher-seeded provider wins)", e.Number, e.Title)
		}
	}
}

// TestSearchAvailableWarningDedup verifies a provider that rejects identically for
// every episode (the "No anidbEid provided" symptom) yields exactly ONE warning,
// not one per episode, while a healthy provider still contributes its results.
func TestSearchAvailableWarningDedup(t *testing.T) {
	reg := source.NewRegistry()
	var badCalls int32
	bad := &fakeProvider{id: "animetosho", searchFn: func(source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
		atomic.AddInt32(&badCalls, 1)
		return nil, errors.New("No anidbEid provided")
	}}
	good := &fakeProvider{id: "good", searchFn: func(o source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
		return []*source.AnimeTorrent{epRelease("g", o.EpisodeNumber, 10)}, nil
	}}
	reg.Register(bad)
	reg.Register(good)

	res := fakeResolver{episodes: map[int]anizip.EpisodeIDs{1: {}, 2: {}, 3: {}}}
	h, _ := newAvailableHandler(t, reg, res)

	eps, warnings := h.searchAvailable(context.Background(), source.Media{ID: 555}, nil)

	// The bad provider was queried once per episode (3 calls) but produces ONE warning.
	if c := atomic.LoadInt32(&badCalls); c != 3 {
		t.Errorf("bad provider call count = %d, want 3 (once per episode)", c)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one (deduped per provider)", warnings)
	}
	if warnings[0] != "animetosho: No anidbEid provided" {
		t.Errorf("warning = %q, want the single deduped provider message", warnings[0])
	}
	// The healthy provider still yields all three episodes.
	if len(eps) != 3 {
		t.Errorf("episodes = %+v, want 3 from the healthy provider", eps)
	}
}

// TestSearchAvailableAllProvidersFail verifies the all-sources-unreachable collapse
// fires when every provider fails every episode.
func TestSearchAvailableAllProvidersFail(t *testing.T) {
	reg := source.NewRegistry()
	for _, id := range []string{"a", "b"} {
		reg.Register(&fakeProvider{id: id, searchFn: func(source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
			return nil, errors.New("dial tcp: connection refused")
		}})
	}
	res := fakeResolver{episodes: map[int]anizip.EpisodeIDs{1: {}, 2: {}}}
	h, _ := newAvailableHandler(t, reg, res)

	eps, warnings := h.searchAvailable(context.Background(), source.Media{ID: 9}, nil)
	if len(eps) != 0 {
		t.Errorf("episodes = %+v, want none", eps)
	}
	if len(warnings) != 1 || warnings[0] != "All installed sources are unreachable — check Extensions." {
		t.Errorf("warnings = %v, want the single all-unreachable message", warnings)
	}
}

// TestSearchAvailableNoEpisodeMap verifies that when ani.zip has no episode map and
// media.EpisodeCount is unknown, searchAvailable returns no episodes plus the
// clear "couldn't determine episodes" warning rather than a number-0 query.
func TestSearchAvailableNoEpisodeMap(t *testing.T) {
	reg := source.NewRegistry()
	p := &fakeProvider{id: "a", searchFn: func(source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
		t.Errorf("provider must not be queried when no episode set is resolvable")
		return nil, nil
	}}
	reg.Register(p)

	res := fakeResolver{episodes: nil} // no ani.zip coverage
	h, _ := newAvailableHandler(t, reg, res)

	media := source.Media{ID: 12345, EpisodeCount: 0} // EpisodeCount unknown too
	eps, warnings := h.searchAvailable(context.Background(), media, nil)
	if len(eps) != 0 {
		t.Errorf("episodes = %+v, want none", eps)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one explaining the missing mapping", warnings)
	}
}

// TestSearchAvailableEpisodeCountFallback verifies that when ani.zip has no map,
// searchAvailable falls back to media.EpisodeCount for the episode set.
func TestSearchAvailableEpisodeCountFallback(t *testing.T) {
	reg := source.NewRegistry()
	p := &fakeProvider{id: "a", searchFn: func(o source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
		return []*source.AnimeTorrent{epRelease("x", o.EpisodeNumber, 3)}, nil
	}}
	reg.Register(p)

	res := fakeResolver{episodes: nil}
	h, _ := newAvailableHandler(t, reg, res)

	eps, warnings := h.searchAvailable(context.Background(), source.Media{ID: 7, EpisodeCount: 2}, nil)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(eps) != 2 || eps[0].Number != 1 || eps[1].Number != 2 {
		t.Errorf("episodes = %+v, want numbers [1,2] from EpisodeCount fallback", eps)
	}
}

// TestSearchAvailableTrustedFlagAndGroup verifies the per-episode pick prefers a
// trusted release (flagging it trusted=true with its group) and still offers the
// best non-trusted release (trusted=false) when no trusted release exists.
func TestSearchAvailableTrustedFlagAndGroup(t *testing.T) {
	reg := source.NewRegistry()
	// Ep 1: a trusted SubsPlease release + an untrusted one (higher seeders).
	// Ep 2: only an untrusted release. Names carry the media title so the title
	// filter keeps them; Confirmed:false so the trusted flag drives the pick.
	p := &fakeProvider{id: "a", searchFn: func(o source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
		switch o.EpisodeNumber {
		case 1:
			return []*source.AnimeTorrent{
				{Name: "[Random] Test Show - 01 (1080p)", ReleaseGroup: "Random",
					Magnet: "magnet:?xt=urn:btih:r1", EpisodeNumber: 1, Seeders: 900, Resolution: "1080p"},
				{Name: "[SubsPlease] Test Show - 01 (1080p)", ReleaseGroup: "SubsPlease",
					Magnet: "magnet:?xt=urn:btih:sp1", EpisodeNumber: 1, Seeders: 100, Resolution: "1080p"},
			}, nil
		default:
			return []*source.AnimeTorrent{
				{Name: "[Random] Test Show - 02 (1080p)", ReleaseGroup: "Random",
					Magnet: "magnet:?xt=urn:btih:r2", EpisodeNumber: 2, Seeders: 700, Resolution: "1080p"},
			}, nil
		}
	}}
	reg.Register(p)

	res := fakeResolver{episodes: map[int]anizip.EpisodeIDs{1: {}, 2: {}}}
	h, _ := newAvailableHandler(t, reg, res)

	media := source.Media{ID: 777, RomajiTitle: "Test Show"}
	eps, warnings := h.searchAvailable(context.Background(), media, nil)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(eps) != 2 {
		t.Fatalf("episodes = %+v, want 2", eps)
	}
	byNum := map[int]AvailableEpisode{}
	for _, e := range eps {
		byNum[e.Number] = e
	}
	if e := byNum[1]; !e.Trusted || e.ReleaseGroup != "SubsPlease" {
		t.Errorf("ep1 = {trusted:%v group:%q}, want {true SubsPlease} despite lower seeders", e.Trusted, e.ReleaseGroup)
	}
	if e := byNum[2]; e.Trusted || e.ReleaseGroup != "Random" {
		t.Errorf("ep2 = {trusted:%v group:%q}, want {false Random}", e.Trusted, e.ReleaseGroup)
	}
}

// TestGetSeriesByAnilist verifies GET /api/series/by-anilist/{id} returns the full
// SeriesDetail (200) for a tracked anilist id and 404 for an untracked one.
func TestGetSeriesByAnilist(t *testing.T) {
	srv, st := newTrackingServer(t)
	s := addTrackedSeries(t, st, "By Anilist", 182205)

	rec := getJSON(t, srv, "/api/series/by-anilist/182205")
	if rec.Code != http.StatusOK {
		t.Fatalf("by-anilist: status=%d body=%s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[SeriesDetail](t, rec)
	if resp.Data == nil {
		t.Fatalf("no data: %s", rec.Body.String())
	}
	if resp.Data.ID != s.ID {
		t.Errorf("id = %d, want local series id %d", resp.Data.ID, s.ID)
	}
	if resp.Data.AnilistID == nil || *resp.Data.AnilistID != 182205 {
		t.Errorf("anilist_id = %v, want 182205", resp.Data.AnilistID)
	}
	if resp.Data.Episodes == nil {
		t.Errorf("episodes should be [] not null")
	}

	// Untracked anilist id → 404.
	rec404 := getJSON(t, srv, "/api/series/by-anilist/999999")
	if rec404.Code != http.StatusNotFound {
		t.Errorf("untracked by-anilist: status=%d, want 404", rec404.Code)
	}
}
