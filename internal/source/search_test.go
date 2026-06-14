package source

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeSearchProvider is a Provider that records the episode numbers it was
// SmartSearched for and returns canned releases (or an error). Only SmartSearch
// carries behaviour; the rest are stubs.
type fakeSearchProvider struct {
	id       string
	searchFn func(opts SmartSearchOptions) ([]*AnimeTorrent, error)

	mu      sync.Mutex
	queried []int
}

func (f *fakeSearchProvider) ID() string { return f.id }
func (f *fakeSearchProvider) SmartSearch(_ context.Context, opts SmartSearchOptions) ([]*AnimeTorrent, error) {
	f.mu.Lock()
	f.queried = append(f.queried, opts.EpisodeNumber)
	f.mu.Unlock()
	return f.searchFn(opts)
}
func (f *fakeSearchProvider) Search(context.Context, SearchOptions) ([]*AnimeTorrent, error) {
	return nil, nil
}
func (f *fakeSearchProvider) GetLatest(context.Context) ([]*AnimeTorrent, error) { return nil, nil }
func (f *fakeSearchProvider) GetTorrentMagnetLink(_ context.Context, t *AnimeTorrent) (string, error) {
	return t.Magnet, nil
}
func (f *fakeSearchProvider) GetTorrentInfoHash(_ context.Context, t *AnimeTorrent) (string, error) {
	return t.InfoHash, nil
}
func (f *fakeSearchProvider) GetSettings() Settings { return Settings{CanSmartSearch: true} }

func release(name string, ep, seeders int) *AnimeTorrent {
	return &AnimeTorrent{Name: name, EpisodeNumber: ep, Seeders: seeders, Resolution: "1080p"}
}

// TestSearchEpisodesPerEpisodeBucketing verifies each queried episode is searched
// on every provider and the returned releases land under the queried number.
func TestSearchEpisodesPerEpisodeBucketing(t *testing.T) {
	reg := NewRegistry()
	pa := &fakeSearchProvider{id: "a", searchFn: func(o SmartSearchOptions) ([]*AnimeTorrent, error) {
		return []*AnimeTorrent{release("a-ep", o.EpisodeNumber, 5)}, nil
	}}
	pb := &fakeSearchProvider{id: "b", searchFn: func(o SmartSearchOptions) ([]*AnimeTorrent, error) {
		return []*AnimeTorrent{release("b-ep", o.EpisodeNumber, 9)}, nil
	}}
	reg.Register(pa)
	reg.Register(pb)

	cands, warnings := SearchEpisodes(context.Background(), reg, Media{ID: 1}, []int{1, 2, 3}, SmartSearchOptions{BestReleases: true})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	for _, n := range []int{1, 2, 3} {
		if len(cands[n]) != 2 {
			t.Errorf("bucket %d = %d candidates, want 2 (one per provider)", n, len(cands[n]))
		}
		for _, c := range cands[n] {
			if c.EpisodeNumber != n {
				t.Errorf("bucket %d holds a torrent for episode %d", n, c.EpisodeNumber)
			}
		}
	}
}

// TestSearchEpisodesCrossEpisodeBucketing verifies a torrent whose parsed episode
// differs from the queried number is bucketed under its own (batch/multi) number.
func TestSearchEpisodesCrossEpisodeBucketing(t *testing.T) {
	reg := NewRegistry()
	p := &fakeSearchProvider{id: "a", searchFn: func(o SmartSearchOptions) ([]*AnimeTorrent, error) {
		// Queried for ep 1, the provider returns a release parsed as episode 5.
		return []*AnimeTorrent{release("multi", 5, 3)}, nil
	}}
	reg.Register(p)

	cands, _ := SearchEpisodes(context.Background(), reg, Media{ID: 1}, []int{1}, SmartSearchOptions{})
	if len(cands[1]) != 0 {
		t.Errorf("bucket 1 = %d candidates, want 0 (the ep-5 hit must not bucket to 1)", len(cands[1]))
	}
	if len(cands[5]) != 1 {
		t.Fatalf("bucket 5 = %d candidates, want 1", len(cands[5]))
	}
}

// TestSearchEpisodesWarningDedup verifies a provider erroring identically for
// every queried cell yields exactly one "<id>: <msg>" warning.
func TestSearchEpisodesWarningDedup(t *testing.T) {
	reg := NewRegistry()
	bad := &fakeSearchProvider{id: "animetosho", searchFn: func(SmartSearchOptions) ([]*AnimeTorrent, error) {
		return nil, errors.New("No anidbEid provided")
	}}
	good := &fakeSearchProvider{id: "good", searchFn: func(o SmartSearchOptions) ([]*AnimeTorrent, error) {
		return []*AnimeTorrent{release("g", o.EpisodeNumber, 10)}, nil
	}}
	reg.Register(bad)
	reg.Register(good)

	cands, warnings := SearchEpisodes(context.Background(), reg, Media{ID: 1}, []int{1, 2, 3}, SmartSearchOptions{})
	if len(warnings) != 1 || warnings[0] != "animetosho: No anidbEid provided" {
		t.Fatalf("warnings = %v, want exactly the one deduped provider message", warnings)
	}
	for _, n := range []int{1, 2, 3} {
		if len(cands[n]) != 1 {
			t.Errorf("bucket %d = %d, want 1 from the healthy provider", n, len(cands[n]))
		}
	}
}

// TestSearchEpisodesAllProvidersFail verifies the all-unreachable collapse fires
// when every provider fails every cell.
func TestSearchEpisodesAllProvidersFail(t *testing.T) {
	reg := NewRegistry()
	for _, id := range []string{"a", "b"} {
		reg.Register(&fakeSearchProvider{id: id, searchFn: func(SmartSearchOptions) ([]*AnimeTorrent, error) {
			return nil, errors.New("dial tcp: connection refused")
		}})
	}
	cands, warnings := SearchEpisodes(context.Background(), reg, Media{ID: 1}, []int{1, 2}, SmartSearchOptions{})
	if len(cands) != 0 {
		t.Errorf("candidates = %v, want none", cands)
	}
	if len(warnings) != 1 || warnings[0] != "All installed sources are unreachable — check Extensions." {
		t.Errorf("warnings = %v, want the single all-unreachable message", warnings)
	}
}

// TestSearchEpisodesEmptyRegistry verifies an empty registry returns no candidates
// and no warnings (nothing to search).
func TestSearchEpisodesEmptyRegistry(t *testing.T) {
	cands, warnings := SearchEpisodes(context.Background(), NewRegistry(), Media{ID: 1}, []int{1}, SmartSearchOptions{})
	if len(cands) != 0 || len(warnings) != 0 {
		t.Errorf("empty registry returned cands=%v warnings=%v, want empty", cands, warnings)
	}
}
