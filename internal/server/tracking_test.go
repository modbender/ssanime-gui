package server

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

// newTrackingServer returns a server plus the underlying store so tests can
// assert on persisted state directly.
func newTrackingServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: filepath.Join(dir, "track.db"), Port: config.DefaultPort}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)
	return New(st, hub, nil, Config{}), st
}

// addTrackedSeries inserts a subscribed series with an anilist id and returns it.
func addTrackedSeries(t *testing.T, st *store.Store, title string, anilistID int64) store.Series {
	t.Helper()
	id := anilistID
	s, err := st.Write().CreateSeries(context.Background(), store.CreateSeriesParams{
		Uuid:       mustUUID(),
		Title:      title,
		AnilistID:  &id,
		Subscribed: 1,
	})
	if err != nil {
		t.Fatalf("create series: %v", err)
	}
	return s
}

// TestTrackCreatesSeriesAndFeed verifies POST /api/track creates a subscribed
// series + an auto-feed and returns 201.
func TestTrackCreatesSeriesAndFeed(t *testing.T) {
	srv, st := newTrackingServer(t)

	// AniList is nil (Config{} has no client) so the series is created with a
	// placeholder title — the unreachable-tolerant path.
	rec := postJSON(t, srv, "/api/track", TrackRequest{AnilistID: 12345})
	if rec.Code != http.StatusCreated {
		t.Fatalf("track: status=%d body=%s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[TrackResponse](t, rec)
	if resp.Data == nil {
		t.Fatalf("no data: %s", rec.Body.String())
	}
	if resp.Data.SeriesID == 0 || resp.Data.FeedID == 0 {
		t.Fatalf("track did not return series_id/feed_id: %+v", *resp.Data)
	}

	// The series must be subscribed and have exactly one enabled feed.
	s, err := st.Read().GetSeries(context.Background(), resp.Data.SeriesID)
	if err != nil {
		t.Fatalf("get series: %v", err)
	}
	if s.Subscribed != 1 {
		t.Errorf("subscribed = %d, want 1", s.Subscribed)
	}
	feeds, _ := st.Read().ListFeedsBySeries(context.Background(), resp.Data.SeriesID)
	if len(feeds) != 1 || feeds[0].Enabled != 1 {
		t.Fatalf("expected one enabled feed, got %+v", feeds)
	}
}

// TestTrackIdempotent verifies re-tracking an existing series returns 200, keeps
// one feed, and clears any manual override (re-engages to Active).
func TestTrackIdempotent(t *testing.T) {
	srv, st := newTrackingServer(t)
	s := addTrackedSeries(t, st, "Existing", 999)

	// Pause it first, then re-track and confirm user_status is cleared.
	if err := st.Write().SetSeriesUserStatus(context.Background(), store.SetSeriesUserStatusParams{ID: s.ID, UserStatus: strPtr(userStatusPaused)}); err != nil {
		t.Fatalf("pause: %v", err)
	}

	rec := postJSON(t, srv, "/api/track", TrackRequest{AnilistID: 999})
	if rec.Code != http.StatusOK {
		t.Fatalf("re-track: want 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	updated, _ := st.Read().GetSeries(context.Background(), s.ID)
	if updated.UserStatus != nil {
		t.Errorf("user_status = %v, want nil after re-track", *updated.UserStatus)
	}
	feeds, _ := st.Read().ListFeedsBySeries(context.Background(), s.ID)
	if len(feeds) != 1 {
		t.Errorf("expected exactly one feed after re-track, got %d", len(feeds))
	}
}

// TestPauseDropResumeStatus verifies the manual override endpoints write the
// expected user_status values.
func TestPauseDropResumeStatus(t *testing.T) {
	srv, st := newTrackingServer(t)
	s := addTrackedSeries(t, st, "Override Me", 555)
	id := itoa(int(s.ID))

	cases := []struct {
		path string
		want *string
	}{
		{"/api/series/" + id + "/pause", strPtr(userStatusPaused)},
		{"/api/series/" + id + "/drop", strPtr(userStatusDropped)},
		{"/api/series/" + id + "/resume", nil},
	}
	for _, c := range cases {
		rec := postJSON(t, srv, c.path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", c.path, rec.Code, rec.Body.String())
		}
		got, _ := st.Read().GetSeries(context.Background(), s.ID)
		if c.want == nil {
			if got.UserStatus != nil {
				t.Errorf("%s: user_status = %v, want nil", c.path, *got.UserStatus)
			}
		} else if got.UserStatus == nil || *got.UserStatus != *c.want {
			t.Errorf("%s: user_status = %v, want %q", c.path, got.UserStatus, *c.want)
		}
	}
}

// TestPausedSeriesSkippedByPoller verifies a paused series' feed is excluded from
// ListFeedsDueForPoll (the automation gate), while a NULL-status series is due.
func TestPausedSeriesSkippedByPoller(t *testing.T) {
	srv, st := newTrackingServer(t)
	ctx := context.Background()

	// Two tracked series, each auto-feeded via /track.
	for _, id := range []int64{111, 222} {
		rec := postJSON(t, srv, "/api/track", TrackRequest{AnilistID: id})
		if rec.Code != http.StatusCreated {
			t.Fatalf("track %d: %d %s", id, rec.Code, rec.Body.String())
		}
	}
	s111, _ := st.Read().GetSeriesByAnilistID(ctx, i64ptrLocal(111))

	// Pause the first; it must drop out of the due-for-poll set.
	if err := st.Write().SetSeriesUserStatus(ctx, store.SetSeriesUserStatusParams{ID: s111.ID, UserStatus: strPtr(userStatusPaused)}); err != nil {
		t.Fatalf("pause: %v", err)
	}

	now := int64(1 << 40) // far future so feeds are due
	due, err := st.Read().ListFeedsDueForPoll(ctx, &now)
	if err != nil {
		t.Fatalf("due feeds: %v", err)
	}
	for _, f := range due {
		if f.SeriesID == s111.ID {
			t.Fatalf("paused series %d should not be due for poll", s111.ID)
		}
	}
	if len(due) != 1 {
		t.Fatalf("expected exactly the one active feed due, got %d", len(due))
	}
}

// TestTrackedBuckets verifies /api/tracked groups by status, honoring user_status
// for paused/dropped and derivedStatus for the rest.
func TestTrackedBuckets(t *testing.T) {
	srv, st := newTrackingServer(t)
	ctx := context.Background()

	active := addTrackedSeries(t, st, "Active One", 1)
	paused := addTrackedSeries(t, st, "Paused One", 2)
	dropped := addTrackedSeries(t, st, "Dropped One", 3)
	_ = active

	if err := st.Write().SetSeriesUserStatus(ctx, store.SetSeriesUserStatusParams{ID: paused.ID, UserStatus: strPtr(userStatusPaused)}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if err := st.Write().SetSeriesUserStatus(ctx, store.SetSeriesUserStatusParams{ID: dropped.ID, UserStatus: strPtr(userStatusDropped)}); err != nil {
		t.Fatalf("drop: %v", err)
	}

	rec := getJSON(t, srv, "/api/tracked")
	if rec.Code != http.StatusOK {
		t.Fatalf("tracked: %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[TrackedResponse](t, rec)
	if resp.Data == nil {
		t.Fatalf("no data: %s", rec.Body.String())
	}
	if len(resp.Data.Paused) != 1 || resp.Data.Paused[0].ID != paused.ID {
		t.Errorf("paused bucket = %+v, want [%d]", resp.Data.Paused, paused.ID)
	}
	if len(resp.Data.Dropped) != 1 || resp.Data.Dropped[0].ID != dropped.ID {
		t.Errorf("dropped bucket = %+v, want [%d]", resp.Data.Dropped, dropped.ID)
	}
	// Active One has no episodes and no airing_status → derivedStatus "airing" → in_progress.
	if len(resp.Data.InProgress) != 1 || resp.Data.InProgress[0].ID != active.ID {
		t.Errorf("in_progress bucket = %+v, want [%d]", resp.Data.InProgress, active.ID)
	}
}

// TestManualEnqueueReengages verifies that manually enqueuing an episode clears
// a paused series' user_status (→ Active).
func TestManualEnqueueReengages(t *testing.T) {
	srv, st := newTrackingServer(t)
	ctx := context.Background()
	s := addTrackedSeries(t, st, "Reengage Me", 42)

	if err := st.Write().SetSeriesUserStatus(ctx, store.SetSeriesUserStatusParams{ID: s.ID, UserStatus: strPtr(userStatusPaused)}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	magnet := "magnet:?xt=urn:btih:deadbeef"
	ep, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: mustUUID(), SeriesID: s.ID, SourceKind: "torrent", Magnet: &magnet, Status: "downloaded",
	})
	if err != nil {
		t.Fatalf("create episode: %v", err)
	}

	rec := postJSON(t, srv, "/api/episodes/"+itoa(int(ep.ID))+"/encode", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("enqueue: %d %s", rec.Code, rec.Body.String())
	}

	updated, _ := st.Read().GetSeries(ctx, s.ID)
	if updated.UserStatus != nil {
		t.Errorf("user_status = %v, want nil after manual enqueue", *updated.UserStatus)
	}
}

// TestDiscoveryColdCacheReturnsEmptyRows verifies /api/discovery returns 200 with
// rows present but empty when no discovery provider is wired.
func TestDiscoveryColdCacheReturnsEmptyRows(t *testing.T) {
	srv, _ := newTrackingServer(t)
	rec := getJSON(t, srv, "/api/discovery")
	if rec.Code != http.StatusOK {
		t.Fatalf("discovery: %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[DiscoveryResponse](t, rec)
	if resp.Error != "" {
		t.Fatalf("discovery error: %s", resp.Error)
	}
	if resp.Data == nil || len(resp.Data.Rows) == 0 {
		t.Fatalf("expected non-empty rows list even when cold; got %+v", resp.Data)
	}
	for _, row := range resp.Data.Rows {
		if row.Items == nil {
			t.Errorf("row %q items should be [] not null", row.Key)
		}
	}
}

// TestDownloadAvailableCreatesAndReengages verifies POST
// /api/series/{id}/available/download on a paused series: it creates the queued
// episode for the requested number, clears user_status (→ Active), and is
// idempotent (a second identical call reuses the row, not a duplicate).
func TestDownloadAvailableCreatesAndReengages(t *testing.T) {
	srv, st := newTrackingServer(t)
	ctx := context.Background()
	s := addTrackedSeries(t, st, "Available DL", 7777)

	if err := st.Write().SetSeriesUserStatus(ctx, store.SetSeriesUserStatusParams{ID: s.ID, UserStatus: strPtr(userStatusPaused)}); err != nil {
		t.Fatalf("pause: %v", err)
	}

	body := DownloadAvailableRequest{
		SourceURL:  "magnet:?xt=urn:btih:availbeef",
		Number:     3,
		Resolution: "1080p",
	}
	rec := postJSON(t, srv, "/api/series/"+itoa(int(s.ID))+"/available/download", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("download available: want 201, got %d; body=%s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[EpisodeDetail](t, rec)
	if resp.Data == nil {
		t.Fatalf("no data: %s", rec.Body.String())
	}
	if resp.Data.Status != "queued" {
		t.Errorf("status = %q, want queued", resp.Data.Status)
	}
	if resp.Data.EpisodeNo == nil || *resp.Data.EpisodeNo != 3 {
		t.Errorf("episode_no = %v, want 3", resp.Data.EpisodeNo)
	}
	if resp.Data.Resolution == nil || *resp.Data.Resolution != 1080 {
		t.Errorf("resolution = %v, want 1080", resp.Data.Resolution)
	}

	// The magnet must be stored in magnet (not source_url) per the poller mapping.
	created, err := st.Read().GetEpisode(ctx, resp.Data.ID)
	if err != nil {
		t.Fatalf("get created episode: %v", err)
	}
	if created.Magnet == nil || *created.Magnet != body.SourceURL {
		t.Errorf("magnet = %v, want %q", created.Magnet, body.SourceURL)
	}

	// The paused override must be cleared (series re-engaged to Active).
	updated, _ := st.Read().GetSeries(ctx, s.ID)
	if updated.UserStatus != nil {
		t.Errorf("user_status = %v, want nil after download", *updated.UserStatus)
	}

	// Idempotent: a second identical call reuses the row (200) and does not add one.
	rec2 := postJSON(t, srv, "/api/series/"+itoa(int(s.ID))+"/available/download", body)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second download: want 200 (existing), got %d; body=%s", rec2.Code, rec2.Body.String())
	}
	resp2 := decodeBody[EpisodeDetail](t, rec2)
	if resp2.Data == nil || resp2.Data.ID != resp.Data.ID {
		t.Fatalf("idempotent call returned a different episode: %+v vs id %d", resp2.Data, resp.Data.ID)
	}
	eps, _ := st.Read().ListEpisodesBySeries(ctx, s.ID)
	if len(eps) != 1 {
		t.Fatalf("expected exactly one episode after duplicate download, got %d", len(eps))
	}
}

func i64ptrLocal(i int64) *int64 { return &i }
