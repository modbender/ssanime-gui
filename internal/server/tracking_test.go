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

	// Put it on hold first, then re-track and confirm it returns to watching.
	if err := st.Write().SetSeriesWatchStatus(context.Background(), store.SetSeriesWatchStatusParams{ID: s.ID, WatchStatus: watchStatusOnHold}); err != nil {
		t.Fatalf("on_hold: %v", err)
	}

	rec := postJSON(t, srv, "/api/track", TrackRequest{AnilistID: 999})
	if rec.Code != http.StatusOK {
		t.Fatalf("re-track: want 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	updated, _ := st.Read().GetSeries(context.Background(), s.ID)
	if updated.WatchStatus != watchStatusWatching {
		t.Errorf("watch_status = %q, want watching after re-track", updated.WatchStatus)
	}
	feeds, _ := st.Read().ListFeedsBySeries(context.Background(), s.ID)
	if len(feeds) != 1 {
		t.Errorf("expected exactly one feed after re-track, got %d", len(feeds))
	}
}

// TestPauseDropResumeStatus verifies the legacy pause/drop/resume aliases write
// the expected watch_status values (pause->on_hold, drop->dropped, resume->watching).
func TestPauseDropResumeStatus(t *testing.T) {
	srv, st := newTrackingServer(t)
	s := addTrackedSeries(t, st, "Override Me", 555)
	id := itoa(int(s.ID))

	cases := []struct {
		path string
		want string
	}{
		{"/api/series/" + id + "/pause", watchStatusOnHold},
		{"/api/series/" + id + "/drop", watchStatusDropped},
		{"/api/series/" + id + "/resume", watchStatusWatching},
	}
	for _, c := range cases {
		rec := postJSON(t, srv, c.path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", c.path, rec.Code, rec.Body.String())
		}
		got, _ := st.Read().GetSeries(context.Background(), s.ID)
		if got.WatchStatus != c.want {
			t.Errorf("%s: watch_status = %q, want %q", c.path, got.WatchStatus, c.want)
		}
	}
}

// TestSetSeriesStatus verifies POST /api/series/{id}/status writes the watch
// status, surfaces it in the SeriesProgress "status" field, and rejects invalid
// values with 400.
func TestSetSeriesStatus(t *testing.T) {
	srv, st := newTrackingServer(t)
	s := addTrackedSeries(t, st, "Status Me", 556)
	id := itoa(int(s.ID))

	for _, status := range []string{watchStatusOnHold, watchStatusDropped, watchStatusWatching} {
		rec := postJSON(t, srv, "/api/series/"+id+"/status", SetStatusRequest{Status: status})
		if rec.Code != http.StatusOK {
			t.Fatalf("set %q: status=%d body=%s", status, rec.Code, rec.Body.String())
		}
		resp := decodeBody[SeriesStatusResponse](t, rec)
		if resp.Data == nil || resp.Data.Series.Status != status {
			t.Fatalf("set %q: response status = %v, want %q", status, resp.Data, status)
		}
		got, _ := st.Read().GetSeries(context.Background(), s.ID)
		if got.WatchStatus != status {
			t.Errorf("set %q: persisted watch_status = %q", status, got.WatchStatus)
		}
	}

	// 'completed' and garbage are rejected (completed is derived, not settable).
	for _, bad := range []string{"completed", "", "paused", "nonsense"} {
		rec := postJSON(t, srv, "/api/series/"+id+"/status", SetStatusRequest{Status: bad})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("set %q: status=%d, want 400", bad, rec.Code)
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

	// Put the first on hold; it must drop out of the due-for-poll set (the gate now
	// polls watch_status = 'watching' only).
	if err := st.Write().SetSeriesWatchStatus(ctx, store.SetSeriesWatchStatusParams{ID: s111.ID, WatchStatus: watchStatusOnHold}); err != nil {
		t.Fatalf("on_hold: %v", err)
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

	if err := st.Write().SetSeriesWatchStatus(ctx, store.SetSeriesWatchStatusParams{ID: paused.ID, WatchStatus: watchStatusOnHold}); err != nil {
		t.Fatalf("on_hold: %v", err)
	}
	if err := st.Write().SetSeriesWatchStatus(ctx, store.SetSeriesWatchStatusParams{ID: dropped.ID, WatchStatus: watchStatusDropped}); err != nil {
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

	if err := st.Write().SetSeriesWatchStatus(ctx, store.SetSeriesWatchStatusParams{ID: s.ID, WatchStatus: watchStatusOnHold}); err != nil {
		t.Fatalf("on_hold: %v", err)
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
	if updated.WatchStatus != watchStatusWatching {
		t.Errorf("watch_status = %q, want watching after manual enqueue", updated.WatchStatus)
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

	if err := st.Write().SetSeriesWatchStatus(ctx, store.SetSeriesWatchStatusParams{ID: s.ID, WatchStatus: watchStatusOnHold}); err != nil {
		t.Fatalf("on_hold: %v", err)
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

	// The on-hold series must be re-engaged to watching.
	updated, _ := st.Read().GetSeries(ctx, s.ID)
	if updated.WatchStatus != watchStatusWatching {
		t.Errorf("watch_status = %q, want watching after download", updated.WatchStatus)
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

// TestUnsubscribeCascade verifies POST /api/series/{id}/unsubscribe deletes the
// series and all child rows (episodes, feeds, encoded_outputs, screenshots) via FK
// cascade in one write, leaves a sibling series untouched, and returns 204.
func TestUnsubscribeCascade(t *testing.T) {
	srv, st := newTrackingServer(t)
	ctx := context.Background()

	// Target series with a feed, an episode, an encoded output, and a screenshot.
	target := addTrackedSeries(t, st, "Cascade Target", 8001)
	if _, err := st.Write().CreateFeed(ctx, store.CreateFeedParams{
		Uuid: mustUUID(), SeriesID: target.ID, Type: "scrape",
		Url: "ssanime://test/cascade", IntervalSeconds: 3600, Enabled: 1,
	}); err != nil {
		t.Fatalf("create feed: %v", err)
	}
	ep, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: mustUUID(), SeriesID: target.ID, SourceKind: "torrent", Status: "archived",
		EpisodeNo: i64ptrLocal(1),
	})
	if err != nil {
		t.Fatalf("create episode: %v", err)
	}
	if _, err := st.Write().CreateEncodedOutput(ctx, store.CreateEncodedOutputParams{
		Uuid: mustUUID(), EpisodeID: ep.ID, Resolution: 1080, Status: "archived",
	}); err != nil {
		t.Fatalf("create encoded output: %v", err)
	}
	if _, err := st.Write().CreateScreenshot(ctx, store.CreateScreenshotParams{
		Uuid: mustUUID(), EpisodeID: ep.ID, SeriesID: target.ID, Path: "/tmp/shot.png",
	}); err != nil {
		t.Fatalf("create screenshot: %v", err)
	}

	// A sibling series that must survive the cascade.
	sibling := addTrackedSeries(t, st, "Cascade Sibling", 8002)
	sibEp, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: mustUUID(), SeriesID: sibling.ID, SourceKind: "torrent", Status: "queued",
		EpisodeNo: i64ptrLocal(1),
	})
	if err != nil {
		t.Fatalf("create sibling episode: %v", err)
	}

	rec := postJSON(t, srv, "/api/series/"+itoa(int(target.ID))+"/unsubscribe", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unsubscribe: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// The series row is gone.
	if _, err := st.Read().GetSeries(ctx, target.ID); err == nil {
		t.Errorf("series %d still present after unsubscribe", target.ID)
	}
	// All children are gone (FK cascade).
	if feeds, _ := st.Read().ListFeedsBySeries(ctx, target.ID); len(feeds) != 0 {
		t.Errorf("feeds not cascaded: %d remain", len(feeds))
	}
	if eps, _ := st.Read().ListEpisodesBySeries(ctx, target.ID); len(eps) != 0 {
		t.Errorf("episodes not cascaded: %d remain", len(eps))
	}
	if outs, _ := st.Read().ListEncodedOutputsByEpisode(ctx, ep.ID); len(outs) != 0 {
		t.Errorf("encoded_outputs not cascaded: %d remain", len(outs))
	}

	// The sibling and its episode are untouched.
	if _, err := st.Read().GetSeries(ctx, sibling.ID); err != nil {
		t.Errorf("sibling series deleted: %v", err)
	}
	if _, err := st.Read().GetEpisode(ctx, sibEp.ID); err != nil {
		t.Errorf("sibling episode deleted: %v", err)
	}

	// Unsubscribing a missing series is a 404.
	rec404 := postJSON(t, srv, "/api/series/"+itoa(int(target.ID))+"/unsubscribe", nil)
	if rec404.Code != http.StatusNotFound {
		t.Errorf("re-unsubscribe: status=%d, want 404", rec404.Code)
	}
}

// TestRetryErroredEpisode verifies POST /api/episodes/{id}/retry on an errored
// episode clears the error, requeues it, and returns the EpisodeDetail; a
// non-errored episode is a 409.
func TestRetryErroredEpisode(t *testing.T) {
	srv, st := newTrackingServer(t)
	ctx := context.Background()
	s := addTrackedSeries(t, st, "Retry Me", 9001)

	ep, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: mustUUID(), SeriesID: s.ID, SourceKind: "torrent", Status: "queued",
		EpisodeNo: i64ptrLocal(1),
	})
	if err != nil {
		t.Fatalf("create episode: %v", err)
	}
	msg := "download failed: no seeders"
	if err := st.Write().SetEpisodeError(ctx, store.SetEpisodeErrorParams{ID: ep.ID, ErrorMessage: &msg}); err != nil {
		t.Fatalf("set error: %v", err)
	}

	rec := postJSON(t, srv, "/api/episodes/"+itoa(int(ep.ID))+"/retry", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry: status=%d body=%s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[EpisodeRetryResponse](t, rec)
	if resp.Data == nil {
		t.Fatalf("no data: %s", rec.Body.String())
	}
	if resp.Data.Episode.Status != "queued" {
		t.Errorf("episode status = %q, want queued", resp.Data.Episode.Status)
	}
	if resp.Data.Episode.ErrorMessage != nil {
		t.Errorf("error_message = %v, want nil after retry", *resp.Data.Episode.ErrorMessage)
	}

	fresh, _ := st.Read().GetEpisode(ctx, ep.ID)
	if fresh.Status != "queued" || fresh.ErrorMessage != nil {
		t.Errorf("persisted = (%q, %v), want (queued, nil)", fresh.Status, fresh.ErrorMessage)
	}
	if fresh.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", fresh.RetryCount)
	}

	// Retrying a non-error (now queued) episode is a 409.
	rec409 := postJSON(t, srv, "/api/episodes/"+itoa(int(ep.ID))+"/retry", nil)
	if rec409.Code != http.StatusConflict {
		t.Errorf("retry non-error: status=%d, want 409", rec409.Code)
	}
}

// TestActivityOrdering verifies GET /api/activity lists all subscribed series with
// their episodes, floats a series with active pipeline work to the top, and orders
// episodes newest-first within a series.
func TestActivityOrdering(t *testing.T) {
	srv, st := newTrackingServer(t)
	ctx := context.Background()

	// Idle series: only an archived episode (no active pipeline work).
	idle := addTrackedSeries(t, st, "Idle Series", 10001)
	if _, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: mustUUID(), SeriesID: idle.ID, SourceKind: "torrent", Status: "archived",
		EpisodeNo: i64ptrLocal(1),
	}); err != nil {
		t.Fatalf("idle episode: %v", err)
	}

	// Active series: a downloading episode -> must float to the top.
	active := addTrackedSeries(t, st, "Active Series", 10002)
	ep1, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: mustUUID(), SeriesID: active.ID, SourceKind: "torrent", Status: "archived",
		EpisodeNo: i64ptrLocal(1),
	})
	if err != nil {
		t.Fatalf("active ep1: %v", err)
	}
	ep2, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: mustUUID(), SeriesID: active.ID, SourceKind: "torrent", Status: "downloading",
		EpisodeNo: i64ptrLocal(2),
	})
	if err != nil {
		t.Fatalf("active ep2: %v", err)
	}

	rec := getJSON(t, srv, "/api/activity")
	if rec.Code != http.StatusOK {
		t.Fatalf("activity: status=%d body=%s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[ActivityResponse](t, rec)
	if resp.Data == nil {
		t.Fatalf("no data: %s", rec.Body.String())
	}
	if len(resp.Data.Series) != 2 {
		t.Fatalf("series count = %d, want 2", len(resp.Data.Series))
	}
	// Active series first.
	if resp.Data.Series[0].ID != active.ID {
		t.Errorf("first series = %d, want active series %d", resp.Data.Series[0].ID, active.ID)
	}
	// Every series carries the frozen status field.
	if resp.Data.Series[0].Status != watchStatusWatching {
		t.Errorf("status = %q, want watching", resp.Data.Series[0].Status)
	}
	// Episodes newest-first within the active series: ep2 (modified later) before ep1.
	eps := resp.Data.Series[0].Episodes
	if len(eps) != 2 || eps[0].ID != ep2.ID || eps[1].ID != ep1.ID {
		t.Errorf("episode order = %v, want [%d, %d]", eps, ep2.ID, ep1.ID)
	}
}

func i64ptrLocal(i int64) *int64 { return &i }
