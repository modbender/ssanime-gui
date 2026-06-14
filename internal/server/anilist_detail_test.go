package server

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/anilist"
	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

// fakeDetailFetcher returns a canned AniList detail (or an error) and counts
// calls so tests can assert the cache short-circuits upstream.
type fakeDetailFetcher struct {
	detail anilist.MediaDetail
	err    error
	calls  int
}

func (f *fakeDetailFetcher) GetDetail(_ context.Context, id int) (anilist.MediaDetail, error) {
	f.calls++
	if f.err != nil {
		return anilist.MediaDetail{}, f.err
	}
	d := f.detail
	d.ID = id
	return d, nil
}

// fakeAnizipFetcher returns canned episodes or an error.
type fakeAnizipFetcher struct {
	eps   []anizip.Episode
	err   error
	calls int
}

func (f *fakeAnizipFetcher) GetEpisodes(_ context.Context, _ int) ([]anizip.Episode, error) {
	f.calls++
	return f.eps, f.err
}

func (f *fakeAnizipFetcher) GetIDs(_ context.Context, anilistID int) (anizip.IDs, error) {
	return anizip.IDs{AnilistID: anilistID}, f.err
}

// newDetailServer builds a server wired with the given fetchers and returns it
// alongside the store so tests can inspect/seed the cache.
func newDetailServer(t *testing.T, al AnilistDetailFetcher, az AnizipFetcher) (http.Handler, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: filepath.Join(dir, "detail.db"), Port: config.DefaultPort}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)
	return New(st, hub, nil, Config{AnilistDetail: al, Anizip: az}), st
}

func sampleDetail() anilist.MediaDetail {
	return anilist.MediaDetail{
		Description:  "A synopsis.",
		Genres:       []string{"Action"},
		AverageScore: 80,
		Studio:       "Studio X",
		Source:       "MANGA",
		Season:       "FALL",
		SeasonYear:   2023,
		Duration:     24,
		EpisodeCount: 12,
		NextAiring:   &anilist.AiringEpisode{Episode: 13, AiringAt: 1700000000},
		Trailer:      &anilist.Trailer{Site: "youtube", VideoID: "vid", Thumbnail: "https://i.ytimg.com/vi/vid/hq.jpg"},
		StreamingEpisodes: []anilist.StreamingEpisode{
			{Title: "Stream 1", Thumbnail: "https://img1.ak.crunchyroll.com/1.jpg"},
		},
		Relations:       []anilist.RelatedMedia{{AnilistID: 2, RelationType: "PREQUEL", EnglishTitle: "Rel", CoverImage: "https://s4.anilist.co/r.jpg"}},
		Recommendations: []anilist.RelatedMedia{{AnilistID: 3, EnglishTitle: "Rec", CoverImage: "https://s4.anilist.co/x.jpg"}},
	}
}

func sampleAnizip() []anizip.Episode {
	return []anizip.Episode{
		{Number: 1, Title: "Ep One", Thumbnail: "https://artworks.thetvdb.com/1.jpg", AirDate: "2023-10-01", Overview: "ov1", RuntimeMin: 24},
		{Number: 2, Title: "Ep Two", Thumbnail: "https://artworks.thetvdb.com/2.jpg", AirDate: "2023-10-08", Overview: "ov2", RuntimeMin: 24},
	}
}

// TestDetailFetchAndCacheContract verifies a cold fetch merges both sources into
// the frozen payload, upserts the cache, and a second call serves the cache
// without re-hitting upstream.
func TestDetailFetchAndCacheContract(t *testing.T) {
	al := &fakeDetailFetcher{detail: sampleDetail()}
	az := &fakeAnizipFetcher{eps: sampleAnizip()}
	srv, st := newDetailServer(t, al, az)

	rec := getJSON(t, srv, "/api/anilist/21/detail")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[AnilistDetail](t, rec)
	if resp.Data == nil {
		t.Fatalf("no data: %s", rec.Body.String())
	}
	d := resp.Data
	if d.AnilistID != 21 {
		t.Errorf("anilist_id = %d", d.AnilistID)
	}
	if d.Description != "A synopsis." || d.Studio != "Studio X" || d.SourceMaterial != "MANGA" {
		t.Errorf("scalar fields wrong: %+v", d)
	}
	if d.DurationMin != 24 || d.EpisodeCount != 12 || d.AverageScore != 80 {
		t.Errorf("numeric fields wrong: %+v", d)
	}
	if d.NextAiring == nil || d.NextAiring.Episode != 13 {
		t.Errorf("next_airing = %+v", d.NextAiring)
	}
	if d.Trailer == nil || d.Trailer.VideoID != "vid" {
		t.Errorf("trailer = %+v", d.Trailer)
	}
	if len(d.Episodes) != 2 || d.Episodes[0].Number != 1 || d.Episodes[1].Number != 2 {
		t.Fatalf("episodes = %+v", d.Episodes)
	}
	if d.Episodes[0].Title != "Ep One" || d.Episodes[0].Thumbnail == "" {
		t.Errorf("episode 1 merge wrong: %+v", d.Episodes[0])
	}
	if len(d.Relations) != 1 || d.Relations[0].RelationType != "PREQUEL" {
		t.Errorf("relations = %+v", d.Relations)
	}
	if len(d.Recommendations) != 1 || d.Recommendations[0].AnilistID != 3 {
		t.Errorf("recommendations = %+v", d.Recommendations)
	}

	// Cache row written.
	if _, err := st.Read().GetAnilistDetailCache(context.Background(), 21); err != nil {
		t.Fatalf("cache row not written: %v", err)
	}

	// Second call serves the cache: no extra upstream calls.
	rec2 := getJSON(t, srv, "/api/anilist/21/detail")
	if rec2.Code != http.StatusOK {
		t.Fatalf("second detail: %d", rec2.Code)
	}
	if al.calls != 1 {
		t.Errorf("AniList called %d times, want 1 (cache should short-circuit)", al.calls)
	}
	if az.calls != 1 {
		t.Errorf("ani.zip called %d times, want 1", az.calls)
	}
}

// TestDetailStaleCacheRefetched verifies a row older than the TTL is refetched.
func TestDetailStaleCacheRefetched(t *testing.T) {
	al := &fakeDetailFetcher{detail: sampleDetail()}
	az := &fakeAnizipFetcher{eps: sampleAnizip()}
	srv, st := newDetailServer(t, al, az)

	// Seed a stale cache row (fetched 25h ago).
	stale := time.Now().Add(-25 * time.Hour).Unix()
	if err := st.Write().UpsertAnilistDetailCache(context.Background(), store.UpsertAnilistDetailCacheParams{
		AnilistID: 21, Payload: `{"anilist_id":21,"description":"OLD"}`, FetchedAt: stale,
	}); err != nil {
		t.Fatalf("seed stale: %v", err)
	}

	rec := getJSON(t, srv, "/api/anilist/21/detail")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[AnilistDetail](t, rec)
	if resp.Data == nil || resp.Data.Description != "A synopsis." {
		t.Fatalf("stale row should have been refetched, got %+v", resp.Data)
	}
	if al.calls != 1 {
		t.Errorf("AniList calls = %d, want 1 (stale triggers refetch)", al.calls)
	}
}

// TestDetailServesStaleOnAnilistFailure verifies a stale row is served when the
// AniList fetch fails (rate-limit posture).
func TestDetailServesStaleOnAnilistFailure(t *testing.T) {
	al := &fakeDetailFetcher{err: errors.New("rate limited (429)")}
	az := &fakeAnizipFetcher{eps: sampleAnizip()}
	srv, st := newDetailServer(t, al, az)

	stale := time.Now().Add(-48 * time.Hour).Unix()
	if err := st.Write().UpsertAnilistDetailCache(context.Background(), store.UpsertAnilistDetailCacheParams{
		AnilistID: 21, Payload: `{"anilist_id":21,"description":"STALE BUT SERVED"}`, FetchedAt: stale,
	}); err != nil {
		t.Fatalf("seed stale: %v", err)
	}

	rec := getJSON(t, srv, "/api/anilist/21/detail")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 serving stale, got %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[AnilistDetail](t, rec)
	if resp.Data == nil || resp.Data.Description != "STALE BUT SERVED" {
		t.Fatalf("expected stale payload served, got %+v", resp.Data)
	}
}

// TestDetailErrorsWhenNoCacheAndAnilistFails verifies a hard error when there is
// neither a cache row nor a successful AniList fetch.
func TestDetailErrorsWhenNoCacheAndAnilistFails(t *testing.T) {
	al := &fakeDetailFetcher{err: errors.New("rate limited (429)")}
	az := &fakeAnizipFetcher{eps: sampleAnizip()}
	srv, _ := newDetailServer(t, al, az)

	rec := getJSON(t, srv, "/api/anilist/21/detail")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[AnilistDetail](t, rec)
	if resp.Error == "" {
		t.Error("expected an error message in the envelope")
	}
}

// TestDetailDegradesWhenAnizipAbsent verifies that ani.zip failing alone yields
// an AniList-only payload: episodes fall back to streamingEpisodes (by position),
// the page still works, no error.
func TestDetailDegradesWhenAnizipAbsent(t *testing.T) {
	al := &fakeDetailFetcher{detail: sampleDetail()}
	az := &fakeAnizipFetcher{err: errors.New("ani.zip down")}
	srv, _ := newDetailServer(t, al, az)

	rec := getJSON(t, srv, "/api/anilist/21/detail")
	if rec.Code != http.StatusOK {
		t.Fatalf("ani.zip failure should still 200, got %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[AnilistDetail](t, rec)
	if resp.Data == nil {
		t.Fatalf("no data: %s", rec.Body.String())
	}
	// One streamingEpisode supplies episode 1 by position; no ani.zip overview.
	if len(resp.Data.Episodes) != 1 || resp.Data.Episodes[0].Number != 1 {
		t.Fatalf("expected one streamingEpisode-derived episode, got %+v", resp.Data.Episodes)
	}
	if resp.Data.Episodes[0].Title != "Stream 1" {
		t.Errorf("episode title = %q, want streamingEpisodes fallback", resp.Data.Episodes[0].Title)
	}
	if resp.Data.Episodes[0].Overview != "" {
		t.Errorf("overview should be empty without ani.zip, got %q", resp.Data.Episodes[0].Overview)
	}
}

// TestDetailNilSlicesSerializeAsArrays verifies genres/episodes/relations are
// always [] not null in the JSON.
func TestDetailNilSlicesSerializeAsArrays(t *testing.T) {
	al := &fakeDetailFetcher{detail: anilist.MediaDetail{Description: "bare"}}
	az := &fakeAnizipFetcher{}
	srv, _ := newDetailServer(t, al, az)

	rec := getJSON(t, srv, "/api/anilist/21/detail")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, frag := range []string{`"genres":[]`, `"episodes":[]`, `"relations":[]`, `"recommendations":[]`, `"next_airing":null`, `"trailer":null`} {
		if !contains(body, frag) {
			t.Errorf("payload missing %q: %s", frag, body)
		}
	}
}

// TestRefreshBustsDetailCache verifies POST /series/{id}/refresh deletes the
// detail cache row for that series' anilist_id.
func TestRefreshBustsDetailCache(t *testing.T) {
	al := &fakeDetailFetcher{detail: sampleDetail()}
	az := &fakeAnizipFetcher{eps: sampleAnizip()}

	// Build a server that also has a metadata refresher (so /refresh succeeds).
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: filepath.Join(dir, "refresh.db"), Port: config.DefaultPort}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)

	ctx := context.Background()
	anilistID := int64(21)
	s, err := st.Write().CreateSeries(ctx, store.CreateSeriesParams{
		Uuid: mustUUID(), Title: "Cached Show", AnilistID: &anilistID,
	})
	if err != nil {
		t.Fatalf("create series: %v", err)
	}

	// Seed a cache row for this anilist id.
	if err := st.Write().UpsertAnilistDetailCache(ctx, store.UpsertAnilistDetailCacheParams{
		AnilistID: anilistID, Payload: `{"anilist_id":21}`, FetchedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	refresher := &fakeRefresher{series: s}
	srv := New(st, hub, nil, Config{Refresher: refresher, AnilistDetail: al, Anizip: az})

	rec := postJSON(t, srv, "/api/series/"+itoa(int(s.ID))+"/refresh", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body.String())
	}

	// The cache row must be gone.
	if _, err := st.Read().GetAnilistDetailCache(ctx, anilistID); err == nil {
		t.Fatal("expected detail cache row to be busted by refresh")
	}
}

// fakeRefresher satisfies MetadataRefresher, returning a fixed series.
type fakeRefresher struct{ series store.Series }

func (f *fakeRefresher) RefreshSeries(_ context.Context, _ int64) (store.Series, error) {
	return f.series, nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
