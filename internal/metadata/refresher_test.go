package metadata

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/anilist"
	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

// fakeAniList returns canned media (or a fixed error) and records the ids it was
// asked for, so a test can assert which series were fetched.
type fakeAniList struct {
	media    map[int]anilist.Media
	err      error
	batchIDs [][]int
	getCalls int
}

func (f *fakeAniList) GetMediaBatch(_ context.Context, ids []int) (map[int]anilist.Media, error) {
	f.batchIDs = append(f.batchIDs, append([]int(nil), ids...))
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[int]anilist.Media)
	for _, id := range ids {
		if m, ok := f.media[id]; ok {
			out[id] = m
		}
	}
	return out, nil
}

func (f *fakeAniList) GetMedia(_ context.Context, id int) (anilist.Media, error) {
	f.getCalls++
	if f.err != nil {
		return anilist.Media{}, f.err
	}
	m, ok := f.media[id]
	if !ok {
		return anilist.Media{}, errors.New("not found")
	}
	return m, nil
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

func newHub(t *testing.T) *events.Hub {
	t.Helper()
	hub := events.NewHub(slog.Default())
	hub.Start()
	t.Cleanup(hub.Stop)
	return hub
}

func strptr(s string) *string { return &s }
func i64ptr(i int64) *int64   { return &i }

// addSeries inserts a subscribed series with an anilist id and an explicit
// metadata_refreshed_at (nil = never refreshed).
func addSeries(t *testing.T, st *store.Store, title string, anilistID int64, airing string, subscribed int64, refreshedAt *int64) store.Series {
	t.Helper()
	s, err := st.Write().CreateSeries(context.Background(), store.CreateSeriesParams{
		Uuid:                title + "-uuid",
		Title:               title,
		SeasonNumber:        1,
		PosterPortrait:      1,
		Subscribed:          subscribed,
		AnilistID:           &anilistID,
		AiringStatus:        strptr(airing),
		Status:              strptr(airing),
		EpisodeCount:        i64ptr(12),
		MetadataRefreshedAt: refreshedAt,
	})
	if err != nil {
		t.Fatalf("CreateSeries(%s): %v", title, err)
	}
	return s
}

// fixedClock returns a clock pinned to a fixed instant.
func fixedClock(at time.Time) func() time.Time { return func() time.Time { return at } }

func TestRefreshDueUpdatesStaleReleasingSeries(t *testing.T) {
	st := openStore(t)
	hub := newHub(t)
	ctx := context.Background()

	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-48 * time.Hour).Unix() // older than the 24h staleness cutoff
	s := addSeries(t, st, "Frieren", 154587, "RELEASING", 1, &stale)

	fake := &fakeAniList{media: map[int]anilist.Media{
		154587: {ID: 154587, RomajiTitle: "Sousou no Frieren", Status: "FINISHED", EpisodeCount: 28},
	}}
	r := New(st, fake, hub, slog.Default(), WithClock(fixedClock(now)))

	r.RefreshDue(ctx)

	got, err := st.Read().GetSeries(ctx, s.ID)
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	if got.AiringStatus == nil || *got.AiringStatus != "FINISHED" {
		t.Errorf("airing_status = %v, want FINISHED", got.AiringStatus)
	}
	if got.EpisodeCount == nil || *got.EpisodeCount != 28 {
		t.Errorf("episode_count = %v, want 28", got.EpisodeCount)
	}
	if got.MetadataRefreshedAt == nil || *got.MetadataRefreshedAt != now.Unix() {
		t.Errorf("metadata_refreshed_at = %v, want %d", got.MetadataRefreshedAt, now.Unix())
	}
	// Title is a display/unique key and must never be overwritten by a refresh.
	if got.Title != "Frieren" {
		t.Errorf("title = %q, want unchanged 'Frieren'", got.Title)
	}
}

func TestRefreshDueSkipsFinishedAndUnsubscribed(t *testing.T) {
	st := openStore(t)
	hub := newHub(t)
	ctx := context.Background()

	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	// Both are stale, but neither is eligible: one FINISHED, one unsubscribed.
	addSeries(t, st, "Done", 1, "FINISHED", 1, nil)
	addSeries(t, st, "Muted", 2, "RELEASING", 0, nil)

	fake := &fakeAniList{media: map[int]anilist.Media{
		1: {ID: 1, Status: "RELEASING"},
		2: {ID: 2, Status: "RELEASING"},
	}}
	r := New(st, fake, hub, slog.Default(), WithClock(fixedClock(now)))

	r.RefreshDue(ctx)

	// No batch request should have been issued for any id.
	for _, ids := range fake.batchIDs {
		if len(ids) > 0 {
			t.Errorf("unexpected batch fetch for ids %v (none should be eligible)", ids)
		}
	}
}

func TestRefreshDueRateLimitedLeavesRowsUntouched(t *testing.T) {
	st := openStore(t)
	hub := newHub(t)
	ctx := context.Background()

	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-48 * time.Hour).Unix()
	s := addSeries(t, st, "Frieren", 154587, "RELEASING", 1, &stale)

	fake := &fakeAniList{err: errors.New("anilist: rate limited (429)")}
	r := New(st, fake, hub, slog.Default(), WithClock(fixedClock(now)))

	// Must not panic and must not error the loop.
	r.RefreshDue(ctx)

	got, err := st.Read().GetSeries(ctx, s.ID)
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	if got.AiringStatus == nil || *got.AiringStatus != "RELEASING" {
		t.Errorf("airing_status = %v, want unchanged RELEASING", got.AiringStatus)
	}
	// metadata_refreshed_at must NOT be stamped, so the series is retried next tick.
	if got.MetadataRefreshedAt == nil || *got.MetadataRefreshedAt != stale {
		t.Errorf("metadata_refreshed_at = %v, want unchanged %d", got.MetadataRefreshedAt, stale)
	}
}

func TestRefreshSeriesNoAnilistID(t *testing.T) {
	st := openStore(t)
	hub := newHub(t)
	ctx := context.Background()

	s, err := st.Write().CreateSeries(ctx, store.CreateSeriesParams{
		Uuid: "no-al-uuid", Title: "Manual", SeasonNumber: 1, PosterPortrait: 1, Subscribed: 1,
	})
	if err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}
	r := New(st, &fakeAniList{}, hub, slog.Default())

	if _, err := r.RefreshSeries(ctx, s.ID); !errors.Is(err, ErrNoAnilistID) {
		t.Errorf("err = %v, want ErrNoAnilistID", err)
	}
}

func TestRefreshSeriesSuccess(t *testing.T) {
	st := openStore(t)
	hub := newHub(t)
	ctx := context.Background()

	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s := addSeries(t, st, "Frieren", 154587, "RELEASING", 1, nil)
	fake := &fakeAniList{media: map[int]anilist.Media{
		154587: {ID: 154587, RomajiTitle: "Sousou no Frieren", Status: "FINISHED", EpisodeCount: 28},
	}}
	r := New(st, fake, hub, slog.Default(), WithClock(fixedClock(now)))

	updated, err := r.RefreshSeries(ctx, s.ID)
	if err != nil {
		t.Fatalf("RefreshSeries: %v", err)
	}
	if updated.AiringStatus == nil || *updated.AiringStatus != "FINISHED" {
		t.Errorf("airing_status = %v, want FINISHED", updated.AiringStatus)
	}
	if updated.MetadataRefreshedAt == nil || *updated.MetadataRefreshedAt != now.Unix() {
		t.Errorf("metadata_refreshed_at = %v, want %d", updated.MetadataRefreshedAt, now.Unix())
	}
	if fake.getCalls != 1 {
		t.Errorf("GetMedia called %d times, want 1", fake.getCalls)
	}
}
