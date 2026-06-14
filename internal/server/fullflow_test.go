package server

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/poller"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// seriesTitle is the romaji title the metadata refresher lands on the series after
// subscribe; release names embed it so the poller's title matcher accepts them.
const seriesTitle = "Sousou no Frieren"

// fullFlowResolver is a poller.EpisodeResolver returning a fixed aired-episode
// set. It is a separate seam from the server's AnizipFetcher: the server uses its
// fetcher at subscribe time to freeze the backfill floor, while the poller uses
// this resolver at poll time to decide which episodes have aired. The test drives
// them apart on purpose (floor=K vs poll sees K+1,K+2) so episodes actually flow.
type fullFlowResolver struct {
	eps []anizip.Episode
}

func (r fullFlowResolver) GetEpisodes(context.Context, int) ([]anizip.Episode, error) {
	return r.eps, nil
}

// fullFlowProvider is a minimal source.Provider mirroring poller_test's
// stubProvider: SmartSearch filters its fixed result set by the requested episode
// via source.Filter, so per-episode search matches exactly one release.
type fullFlowProvider struct {
	id      string
	results []*source.AnimeTorrent
}

func (s *fullFlowProvider) ID() string { return s.id }
func (s *fullFlowProvider) Search(context.Context, source.SearchOptions) ([]*source.AnimeTorrent, error) {
	return s.results, nil
}
func (s *fullFlowProvider) SmartSearch(_ context.Context, opts source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
	return source.Filter(s.results, opts), nil
}
func (s *fullFlowProvider) GetLatest(context.Context) ([]*source.AnimeTorrent, error) {
	return s.results, nil
}
func (s *fullFlowProvider) GetTorrentMagnetLink(_ context.Context, t *source.AnimeTorrent) (string, error) {
	return t.Magnet, nil
}
func (s *fullFlowProvider) GetTorrentInfoHash(_ context.Context, t *source.AnimeTorrent) (string, error) {
	return t.InfoHash, nil
}
func (s *fullFlowProvider) GetSettings() source.Settings {
	return source.Settings{CanSmartSearch: true, Type: source.ProviderTypeMain}
}

// trustedRelease builds one SubsPlease (trusted-group) release for episode n. The
// name embeds seriesTitle so the poller's title matcher keeps it, and EpisodeNumber
// drives the per-episode SmartSearch filter.
func trustedRelease(n int) *source.AnimeTorrent {
	num := itoa(n)
	return &source.AnimeTorrent{
		Provider: "stub", Name: "[SubsPlease] " + seriesTitle + " - " + num + " (1080p)",
		ReleaseGroup: "SubsPlease", Resolution: "1080p", EpisodeNumber: n,
		Seeders: 1000, InfoHash: "fullflow" + num, Magnet: "magnet:?xt=urn:btih:fullflow" + num,
	}
}

// TestSubscribeThenPollSurfacesEpisodesInActivity is the cross-layer guard for the
// regression "I subscribed to a series and its Activity showed no episodes." (Its
// root cause was an EpisodeNumber=0 / anidbEid=0 bug that silently dropped every
// discovered episode.) The per-layer tests cover the poller chain and the track
// handler in isolation; this ties them together through the HTTP Activity surface:
// subscribe via the Handler, poll on the SAME store, then assert the subscribed
// series shows up in /api/activity WITH the freshly-enqueued episodes.
//
// The floor-vs-resolver interplay is the heart of the scenario. Subscribe freezes
// backfill_from_episode at the highest AIRED episode (K=5 here, via the server's
// AnizipFetcher). The poller only chases episodes ABOVE that floor, so for any
// episode to flow the poll-time resolver must report a higher aired episode than
// the floor — here it reports 6 and 7 as aired (past). Episodes <=5 must stay out.
func TestSubscribeThenPollSurfacesEpisodesInActivity(t *testing.T) {
	const anilistID = 70001
	const floorK = 5 // highest aired episode at subscribe time

	ctx := context.Background()

	// One shared registry: the server's auto-feed stamps Site = registry.List()[0]
	// ("stub"), and the poller resolves that same site to the stub provider — the
	// realistic single-registry wiring.
	reg := source.NewRegistry()
	reg.Register(&fullFlowProvider{
		id:      "stub",
		results: []*source.AnimeTorrent{trustedRelease(6), trustedRelease(7)},
	})

	// Server-side anizip fetcher: eps 1..5 aired (past), so the subscribe-time floor
	// freezes at K=5. Reuses package server's fakeAnizipFetcher.
	srvEps := make([]anizip.Episode, 0, floorK)
	for n := 1; n <= floorK; n++ {
		srvEps = append(srvEps, anizip.Episode{Number: n, AirDate: pastDateFF()})
	}

	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: dir + "/fullflow.db", Port: config.DefaultPort}
	st, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)

	srv := New(st, hub, nil, Config{
		Registry: reg,
		Anizip:   &fakeAnizipFetcher{eps: srvEps},
	})

	// 1. Subscribe through the Handler. New series -> 201, one enabled feed.
	rec := postJSON(t, srv, "/api/track", TrackRequest{AnilistID: anilistID})
	if rec.Code != 201 {
		t.Fatalf("track: status=%d body=%s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[TrackResponse](t, rec)
	if resp.Data == nil || resp.Data.SeriesID == 0 {
		t.Fatalf("track returned no series: %s", rec.Body.String())
	}
	seriesID := resp.Data.SeriesID
	feeds, _ := st.Read().ListFeedsBySeries(ctx, seriesID)
	if len(feeds) != 1 || feeds[0].Enabled != 1 {
		t.Fatalf("want one enabled feed, got %+v", feeds)
	}

	// 2. The backfill floor must be frozen at K — this is what scopes the poll to
	// genuinely-new episodes. If it were wrong, the poll below would enqueue the
	// whole backlog (or nothing) and the scenario would not exercise the bug.
	s, err := st.Read().GetSeries(ctx, seriesID)
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	if s.BackfillFromEpisode == nil || *s.BackfillFromEpisode != floorK {
		t.Fatalf("backfill_from_episode = %v, want %d", s.BackfillFromEpisode, floorK)
	}

	// AniList is nil in this harness so the row carries a placeholder title. Simulate
	// the metadata refresher landing the real romaji title, so the poller's title
	// matcher recognizes the releases (it matches release name against series titles).
	now := time.Now().Unix()
	romaji := seriesTitle
	if err := st.Write().UpdateSeriesMetadata(ctx, store.UpdateSeriesMetadataParams{
		RomajiTitle: romaji, Now: &now, ID: seriesID,
	}); err != nil {
		t.Fatalf("UpdateSeriesMetadata: %v", err)
	}

	// 3. Build the poller on the SAME store + the SAME registry, with a resolver that
	// advances past the floor: eps 1..7 aired, of which only 6 and 7 are above K=5 and
	// not yet in the library. Then run one poll pass.
	pollEps := make([]anizip.Episode, 0, 7)
	for n := 1; n <= 7; n++ {
		pollEps = append(pollEps, anizip.Episode{Number: n, AirDate: pastDateFF()})
	}
	p := poller.New(st, reg, hub, slog.Default(), poller.WithResolver(fullFlowResolver{eps: pollEps}))
	p.PollDue(ctx)

	// 4. The store must now hold exactly the two new episodes (6,7), queued, with the
	// trusted release group.
	eps, err := st.Read().ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatalf("ListEpisodesBySeries: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("want 2 enqueued episodes, got %d", len(eps))
	}
	for _, e := range eps {
		if e.Status != "queued" {
			t.Errorf("episode %v status = %q, want queued", e.EpisodeNo, e.Status)
		}
		if e.ReleaseGroup == nil || *e.ReleaseGroup != "SubsPlease" {
			t.Errorf("episode %v group = %v, want SubsPlease", e.EpisodeNo, e.ReleaseGroup)
		}
	}

	// 5. THE cross-layer guard: the subscribed series must appear in /api/activity
	// WITH a non-empty episodes list. This is the exact surface the user saw empty.
	recAct := getJSON(t, srv, "/api/activity")
	if recAct.Code != 200 {
		t.Fatalf("activity: status=%d body=%s", recAct.Code, recAct.Body.String())
	}
	act := decodeBody[ActivityResponse](t, recAct)
	if act.Data == nil {
		t.Fatalf("activity: no data: %s", recAct.Body.String())
	}
	var found *ActivitySeries
	for i := range act.Data.Series {
		if act.Data.Series[i].ID == seriesID {
			found = &act.Data.Series[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("subscribed series %d absent from Activity (the regression)", seriesID)
	}
	if len(found.Episodes) == 0 {
		t.Fatalf("series %d shows NO episodes in Activity (the regression: subscribed but empty)", seriesID)
	}

	// The Activity episodes must be exactly the enqueued numbers {6,7}, all queued.
	activityNums := map[int64]string{}
	for _, e := range found.Episodes {
		if e.EpisodeNo != nil {
			activityNums[*e.EpisodeNo] = e.Status
		}
	}
	if len(activityNums) != 2 || activityNums[6] != "queued" || activityNums[7] != "queued" {
		t.Fatalf("Activity episodes = %v, want {6:queued, 7:queued}", activityNums)
	}

	// 6a. Negative slice: nothing at or below the floor leaked into Activity. This
	// proves the backfill floor is respected all the way through the HTTP surface.
	for n := int64(1); n <= floorK; n++ {
		if _, ok := activityNums[n]; ok {
			t.Errorf("episode %d at/below floor %d leaked into Activity", n, floorK)
		}
	}

	// 6b. A second poll is idempotent: the seen_cache + have-set keep it from
	// re-enqueuing, so Activity still shows exactly the two episodes (no dupes).
	p.PollDue(ctx)
	recAct2 := getJSON(t, srv, "/api/activity")
	act2 := decodeBody[ActivityResponse](t, recAct2)
	for _, ser := range act2.Data.Series {
		if ser.ID == seriesID && len(ser.Episodes) != 2 {
			t.Errorf("after second poll Activity shows %d episodes, want 2 (idempotent)", len(ser.Episodes))
		}
	}
}

// pastDateFF formats an air date a week in the past (aired) for the air-date gate.
func pastDateFF() string { return time.Now().AddDate(0, 0, -7).Format("2006-01-02") }
