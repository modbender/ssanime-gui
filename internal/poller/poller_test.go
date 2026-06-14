package poller

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// fakeResolver is an EpisodeResolver returning a fixed episode list so the poller
// resolves a known aired-episode set without a network call.
type fakeResolver struct {
	eps []anizip.Episode
	err error
}

func (f fakeResolver) GetEpisodes(context.Context, int) ([]anizip.Episode, error) {
	return f.eps, f.err
}

// pastDate / futureDate format an ISO air date relative to now for air-date gating.
func pastDate() string   { return time.Now().AddDate(0, 0, -7).Format("2006-01-02") }
func futureDate() string { return time.Now().AddDate(0, 0, 7).Format("2006-01-02") }

// stubProvider returns a fixed result set, recording how many times it was
// called so a test can assert a completed series is never fetched.
type stubProvider struct {
	id      string
	results []*source.AnimeTorrent
	calls   int
}

func (s *stubProvider) ID() string { return s.id }
func (s *stubProvider) Search(context.Context, source.SearchOptions) ([]*source.AnimeTorrent, error) {
	return s.results, nil
}
func (s *stubProvider) SmartSearch(_ context.Context, opts source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
	s.calls++
	return source.Filter(s.results, opts), nil
}
func (s *stubProvider) GetLatest(context.Context) ([]*source.AnimeTorrent, error) {
	return s.results, nil
}
func (s *stubProvider) GetTorrentMagnetLink(_ context.Context, t *source.AnimeTorrent) (string, error) {
	return t.Magnet, nil
}
func (s *stubProvider) GetTorrentInfoHash(_ context.Context, t *source.AnimeTorrent) (string, error) {
	return t.InfoHash, nil
}
func (s *stubProvider) GetSettings() source.Settings {
	return source.Settings{CanSmartSearch: true, Type: source.ProviderTypeMain}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: filepath.Join(dir, "test.db"), Port: config.DefaultPort}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func strptr(s string) *string { return &s }
func i64ptr(i int64) *int64   { return &i }

func frierenResults() []*source.AnimeTorrent {
	return []*source.AnimeTorrent{
		{Provider: "stub", Name: "[SubsPlease] Sousou no Frieren - 28 (1080p)",
			ReleaseGroup: "SubsPlease", Resolution: "1080p", EpisodeNumber: 28,
			Seeders: 1500, InfoHash: "hash28sp", Magnet: "magnet:?xt=urn:btih:hash28sp"},
		{Provider: "stub", Name: "[Erai-raws] Sousou no Frieren - 28 (1080p)",
			ReleaseGroup: "Erai-raws", Resolution: "1080p", EpisodeNumber: 28,
			Seeders: 2000, InfoHash: "hash28er", Magnet: "magnet:?xt=urn:btih:hash28er"},
		{Provider: "stub", Name: "[SubsPlease] Sousou no Frieren - 27 (1080p)",
			ReleaseGroup: "SubsPlease", Resolution: "1080p", EpisodeNumber: 27,
			Seeders: 1400, InfoHash: "hash27sp", Magnet: "magnet:?xt=urn:btih:hash27sp"},
	}
}

// makeFeed creates a subscribed, still-airing series and a due feed pointing at
// the stub provider.
func makeFeed(t *testing.T, st *store.Store, status string) (store.Series, store.Feed) {
	t.Helper()
	ctx := context.Background()
	series, err := st.Write().CreateSeries(ctx, store.CreateSeriesParams{
		Uuid:         "series-uuid",
		Title:        "Sousou no Frieren",
		RomajiTitle:  strptr("Sousou no Frieren"),
		EnglishTitle: strptr("Frieren: Beyond Journey's End"),
		Subscribed:   1,
		AnilistID:    i64ptr(154587),
		AiringStatus: strptr(status),
		Status:       strptr(status),
		EpisodeCount: i64ptr(28),
		SeasonNumber: 1,
	})
	if err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}
	feed, err := st.Write().CreateFeed(ctx, store.CreateFeedParams{
		Uuid:            "feed-uuid",
		SeriesID:        series.ID,
		Type:            "rss",
		Site:            strptr("stub"),
		Url:             "https://example.test/rss",
		Quality:         i64ptr(1080),
		IntervalSeconds: 3600,
		Enabled:         1,
	})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}
	return series, feed
}

func newPoller(t *testing.T, st *store.Store, prov source.Provider, opts ...Option) (*Poller, *events.Hub) {
	t.Helper()
	reg := source.NewRegistry()
	reg.Register(prov)
	hub := events.NewHub(slog.Default())
	hub.Start()
	t.Cleanup(hub.Stop)
	p := New(st, reg, hub, slog.Default(), opts...)
	return p, hub
}

func TestPollEnqueuesAndDedupes(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	series, _ := makeFeed(t, st, "RELEASING")
	prov := &stubProvider{id: "stub", results: frierenResults()}
	// ani.zip reports eps 27 and 28 as aired; the per-episode search then queries
	// those two and the stub returns their releases.
	res := fakeResolver{eps: []anizip.Episode{
		{Number: 27, AirDate: pastDate()},
		{Number: 28, AirDate: pastDate()},
	}}
	p, _ := newPoller(t, st, prov, WithResolver(res))

	p.PollDue(ctx)

	eps, err := st.Read().ListEpisodesBySeries(ctx, series.ID)
	if err != nil {
		t.Fatalf("ListEpisodesBySeries: %v", err)
	}
	// One best release per distinct episode (27 and 28) => 2 episodes; the
	// Erai-raws duplicate of ep 28 must lose to SubsPlease in autoselect.
	if len(eps) != 2 {
		names := make([]string, len(eps))
		for i, e := range eps {
			if e.Title != nil {
				names[i] = *e.Title
			}
		}
		t.Fatalf("want 2 episodes, got %d: %v", len(eps), names)
	}
	for _, e := range eps {
		if e.Status != "queued" {
			t.Errorf("episode %d status = %q, want queued", e.ID, e.Status)
		}
		if e.ReleaseGroup == nil || *e.ReleaseGroup != "SubsPlease" {
			t.Errorf("episode %d group = %v, want SubsPlease", e.ID, e.ReleaseGroup)
		}
	}

	// Second pass: everything is in seen_cache, so no new episodes.
	p.PollDue(ctx)
	eps2, _ := st.Read().ListEpisodesBySeries(ctx, series.ID)
	if len(eps2) != 2 {
		t.Errorf("after second poll want 2 episodes (deduped), got %d", len(eps2))
	}
}

// TestPollLocksGroupOnFirstEnqueue verifies the first enqueued episode locks the
// series to its release group (SubsPlease wins trusted-rank), so later episodes
// prefer it.
func TestPollLocksGroupOnFirstEnqueue(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	series, _ := makeFeed(t, st, "RELEASING")
	prov := &stubProvider{id: "stub", results: frierenResults()}
	res := fakeResolver{eps: []anizip.Episode{
		{Number: 27, AirDate: pastDate()},
		{Number: 28, AirDate: pastDate()},
	}}
	p, _ := newPoller(t, st, prov, WithResolver(res))

	p.PollDue(ctx)

	got, err := st.Read().GetSeries(ctx, series.ID)
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	if got.LockedReleaseGroup == nil || *got.LockedReleaseGroup != "SubsPlease" {
		t.Fatalf("locked_release_group = %v, want SubsPlease", got.LockedReleaseGroup)
	}
}

// TestPollLockedGroupPickAndNotOverwritten pre-locks the series to Erai-raws, then
// a poll over a candidate set with both groups must pick the Erai-raws release
// (stage-1 locked-group pick beats SubsPlease's trusted-rank) and leave the lock.
func TestPollLockedGroupPickAndNotOverwritten(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	series, _ := makeFeed(t, st, "RELEASING")
	if err := st.Write().UpdateSeriesLockedGroup(ctx, store.UpdateSeriesLockedGroupParams{
		LockedReleaseGroup: strptr("Erai-raws"), ID: series.ID,
	}); err != nil {
		t.Fatalf("set lock: %v", err)
	}

	prov := &stubProvider{id: "stub", results: frierenResults()}
	res := fakeResolver{eps: []anizip.Episode{{Number: 28, AirDate: pastDate()}}}
	p, _ := newPoller(t, st, prov, WithResolver(res))

	p.PollDue(ctx)

	eps, err := st.Read().ListEpisodesBySeries(ctx, series.ID)
	if err != nil {
		t.Fatalf("ListEpisodesBySeries: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("want 1 episode (ep 28), got %d", len(eps))
	}
	if eps[0].ReleaseGroup == nil || *eps[0].ReleaseGroup != "Erai-raws" {
		t.Errorf("enqueued group = %v, want Erai-raws (locked-group stage)", eps[0].ReleaseGroup)
	}
	got, _ := st.Read().GetSeries(ctx, series.ID)
	if got.LockedReleaseGroup == nil || *got.LockedReleaseGroup != "Erai-raws" {
		t.Errorf("lock = %v, want Erai-raws (unchanged)", got.LockedReleaseGroup)
	}
}

// TestPollStage2Fallback covers the air-date+24h gate when the locked group has no
// release for the episode. With lock=Erai-raws and only a SubsPlease release for
// ep 5: a past air date (>24h) lets the SubsPlease fallback enqueue; a today air
// date (<24h) keeps it skipped until the gate opens.
func TestPollStage2Fallback(t *testing.T) {
	cases := []struct {
		name    string
		airDate string
		wantEnq bool
	}{
		{"past air+24h fires fallback", pastDate(), true},
		{"within 24h skips", time.Now().Format("2006-01-02"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openStore(t)
			ctx := context.Background()
			series, _ := makeFeed(t, st, "RELEASING")
			if err := st.Write().UpdateSeriesLockedGroup(ctx, store.UpdateSeriesLockedGroupParams{
				LockedReleaseGroup: strptr("Erai-raws"), ID: series.ID,
			}); err != nil {
				t.Fatalf("set lock: %v", err)
			}
			// Provider returns only SubsPlease for ep 5 — the locked group is absent.
			prov := &stubProvider{id: "stub", results: perEpisodeReleases(5, 5)}
			res := fakeResolver{eps: []anizip.Episode{{Number: 5, AirDate: tc.airDate}}}
			p, _ := newPoller(t, st, prov, WithResolver(res))

			p.PollDue(ctx)

			eps, _ := st.Read().ListEpisodesBySeries(ctx, series.ID)
			if tc.wantEnq {
				if len(eps) != 1 || eps[0].ReleaseGroup == nil || *eps[0].ReleaseGroup != "SubsPlease" {
					t.Fatalf("want SubsPlease ep enqueued, got %d episodes", len(eps))
				}
			} else if len(eps) != 0 {
				t.Fatalf("want nothing enqueued within 24h of air, got %d", len(eps))
			}
		})
	}
}

func TestPollSkipsCompletedSeries(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	series, _ := makeFeed(t, st, "FINISHED")

	// Archive all 28 episodes so the series is derived-status "completed".
	for i := 1; i <= 28; i++ {
		ep, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
			Uuid:     "ep-" + time.Now().Format("150405.000000000") + "-" + itoa(i),
			SeriesID: series.ID, SourceKind: "torrent", Status: "archived",
			EpisodeNo: i64ptr(int64(i)),
		})
		if err != nil {
			t.Fatalf("seed archived episode: %v", err)
		}
		_ = ep
	}

	prov := &stubProvider{id: "stub", results: frierenResults()}
	p, _ := newPoller(t, st, prov)
	p.PollDue(ctx)

	// The completed series must not be fetched, and no new (queued) episode rows
	// may be created.
	if prov.calls != 0 {
		t.Errorf("provider called %d times for a completed series, want 0", prov.calls)
	}
	eps, _ := st.Read().ListEpisodesByStatus(ctx, "queued")
	if len(eps) != 0 {
		t.Errorf("completed series enqueued %d episodes, want 0", len(eps))
	}
}

func TestPollSkipsUnsubscribed(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	series, _ := makeFeed(t, st, "RELEASING")
	if err := st.Write().SetSeriesSubscribed(ctx, store.SetSeriesSubscribedParams{
		Subscribed: 0, ID: series.ID,
	}); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}

	prov := &stubProvider{id: "stub", results: frierenResults()}
	p, _ := newPoller(t, st, prov)
	p.PollDue(ctx)

	if prov.calls != 0 {
		t.Errorf("provider called %d times for an unsubscribed series, want 0", prov.calls)
	}
}

func TestProviderForUnregistered(t *testing.T) {
	p := New(nil, source.NewRegistry(), nil, slog.Default())

	// A site pointing at a provider id that isn't registered.
	if _, err := p.providerFor(store.Feed{Site: strptr("ghost")}); !errorsIs(err, errProviderNotRegistered) {
		t.Errorf("providerFor(ghost) err = %v, want errProviderNotRegistered", err)
	}

	// A feed with no site at all.
	if _, err := p.providerFor(store.Feed{Site: nil}); !errorsIs(err, errProviderNotRegistered) {
		t.Errorf("providerFor(nil site) err = %v, want errProviderNotRegistered", err)
	}
}

func TestPollSkipsUnregisteredProviderWithoutError(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	series, feed := makeFeed(t, st, "RELEASING")

	// An empty registry means the feed's "stub" provider is unregistered, so the
	// feed must be skipped quietly: nothing enqueued, no error stamped.
	hub := events.NewHub(slog.Default())
	hub.Start()
	t.Cleanup(hub.Stop)
	p := New(st, source.NewRegistry(), hub, slog.Default())
	p.PollDue(ctx)

	eps, _ := st.Read().ListEpisodesBySeries(ctx, series.ID)
	if len(eps) != 0 {
		t.Errorf("unregistered-provider feed enqueued %d episodes, want 0", len(eps))
	}
	got, err := st.Read().GetFeed(ctx, feed.ID)
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if got.ErrorMessage != nil {
		t.Errorf("feed error_message = %q, want nil (graceful skip)", *got.ErrorMessage)
	}
}

// TestPollRespectsBackfillWatermark verifies the poller auto-downloads only
// episodes above the backfill floor: with floor=5 and eps 1..7 all aired, only
// 6 and 7 enqueue; nothing at or below 5.
func TestPollRespectsBackfillWatermark(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	series, _ := makeFeed(t, st, "RELEASING")
	if err := st.Write().UpdateSeriesBackfillFrom(ctx, store.UpdateSeriesBackfillFromParams{
		BackfillFromEpisode: i64ptr(5), ID: series.ID,
	}); err != nil {
		t.Fatalf("set backfill floor: %v", err)
	}

	// A per-episode release for whatever number is queried.
	prov := &stubProvider{id: "stub"}
	prov.results = perEpisodeReleases(1, 7)
	eps := make([]anizip.Episode, 0, 7)
	for n := 1; n <= 7; n++ {
		eps = append(eps, anizip.Episode{Number: n, AirDate: pastDate()})
	}
	p, _ := newPoller(t, st, prov, WithResolver(fakeResolver{eps: eps}))

	p.PollDue(ctx)

	got, err := st.Read().ListEpisodesBySeries(ctx, series.ID)
	if err != nil {
		t.Fatalf("ListEpisodesBySeries: %v", err)
	}
	nums := map[int64]bool{}
	for _, e := range got {
		if e.EpisodeNo != nil {
			nums[*e.EpisodeNo] = true
		}
	}
	if len(nums) != 2 || !nums[6] || !nums[7] {
		t.Fatalf("enqueued episodes = %v, want exactly {6,7}", nums)
	}
	for n := int64(1); n <= 5; n++ {
		if nums[n] {
			t.Errorf("episode %d at/below watermark was enqueued", n)
		}
	}
}

// TestPollSkipsFutureAirDates verifies an episode whose air date is in the future
// is never queried/enqueued even when it sits above the watermark.
func TestPollSkipsFutureAirDates(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	series, _ := makeFeed(t, st, "RELEASING")

	prov := &stubProvider{id: "stub", results: perEpisodeReleases(1, 3)}
	res := fakeResolver{eps: []anizip.Episode{
		{Number: 1, AirDate: pastDate()},
		{Number: 2, AirDate: pastDate()},
		{Number: 3, AirDate: futureDate()}, // not yet aired
	}}
	p, _ := newPoller(t, st, prov, WithResolver(res))

	p.PollDue(ctx)

	got, _ := st.Read().ListEpisodesBySeries(ctx, series.ID)
	nums := map[int64]bool{}
	for _, e := range got {
		if e.EpisodeNo != nil {
			nums[*e.EpisodeNo] = true
		}
	}
	if nums[3] {
		t.Errorf("future episode 3 was enqueued, want skipped")
	}
	if !nums[1] || !nums[2] {
		t.Errorf("aired episodes = %v, want 1 and 2 enqueued", nums)
	}
}

// perEpisodeReleases builds one SubsPlease release per episode in [lo,hi], so the
// stub's per-episode SmartSearch filter returns the matching release for each n.
func perEpisodeReleases(lo, hi int) []*source.AnimeTorrent {
	var out []*source.AnimeTorrent
	for n := lo; n <= hi; n++ {
		out = append(out, &source.AnimeTorrent{
			Provider: "stub", Name: "[SubsPlease] Sousou no Frieren - " + itoa(n) + " (1080p)",
			ReleaseGroup: "SubsPlease", Resolution: "1080p", EpisodeNumber: n,
			Seeders: 1000, InfoHash: "hash" + itoa(n), Magnet: "magnet:?xt=urn:btih:hash" + itoa(n),
		})
	}
	return out
}

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
