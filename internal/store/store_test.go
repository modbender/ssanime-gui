package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/modbender/ssanime-gui/internal/config"
)

// openTestStore opens a fresh Store backed by a temp-file DB (WAL + the dual
// pool need a real file, not :memory:) with migrations, recovery, and seeds run.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir: dir,
		DBPath:  filepath.Join(dir, "test.db"),
		Port:    config.DefaultPort,
	}
	s, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSeedsPresent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	prof, err := s.Read().GetEncodeProfileByName(ctx, builtinProfileName)
	if err != nil {
		t.Fatalf("builtin profile missing: %v", err)
	}
	if prof.Builtin != 1 {
		t.Errorf("builtin flag = %d, want 1", prof.Builtin)
	}
	if prof.Crf == nil || *prof.Crf != 24.2 {
		t.Errorf("crf = %v, want 24.2", prof.Crf)
	}
	if prof.OutputResolutions == nil || *prof.OutputResolutions != "[1080,720,480]" {
		t.Errorf("output_resolutions = %v, want [1080,720,480]", prof.OutputResolutions)
	}

	client, err := s.Read().GetDefaultDownloadClient(ctx)
	if err != nil {
		t.Fatalf("default download client missing: %v", err)
	}
	if client.Kind != "embedded" {
		t.Errorf("default client kind = %q, want embedded", client.Kind)
	}

	set, err := s.Read().GetSettings(ctx)
	if err != nil {
		t.Fatalf("settings missing: %v", err)
	}
	if set.ID != 1 {
		t.Errorf("settings id = %d, want 1", set.ID)
	}
	if set.DefaultProfileID == nil || *set.DefaultProfileID != prof.ID {
		t.Errorf("settings default_profile_id = %v, want %d", set.DefaultProfileID, prof.ID)
	}
	if set.DownloadBackend == nil || *set.DownloadBackend != client.ID {
		t.Errorf("settings download_backend = %v, want %d", set.DownloadBackend, client.ID)
	}
	if set.NamingTemplate != defaultNamingTemplate {
		t.Errorf("naming_template = %q, want %q", set.NamingTemplate, defaultNamingTemplate)
	}
}

func TestSeedIdempotent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Re-running seed must not duplicate rows or error.
	if err := s.seed(ctx, &config.Config{DataDir: t.TempDir()}); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	profiles, err := s.Read().ListEncodeProfiles(ctx)
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Errorf("profile count = %d, want 1", len(profiles))
	}
	clients, err := s.Read().ListDownloadClients(ctx)
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	if len(clients) != 1 {
		t.Errorf("download client count = %d, want 1", len(clients))
	}
}

func TestEpisodeStatusTransition(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	series, err := s.Write().CreateSeries(ctx, CreateSeriesParams{
		Uuid:           newUUID(),
		Title:          "Frieren",
		SeasonNumber:   1,
		PosterPortrait: 1,
	})
	if err != nil {
		t.Fatalf("create series: %v", err)
	}

	ep, err := s.Write().CreateEpisode(ctx, CreateEpisodeParams{
		Uuid:       newUUID(),
		SeriesID:   series.ID,
		EpisodeNo:  p[int64](1),
		SourceKind: "torrent",
		Status:     "queued",
	})
	if err != nil {
		t.Fatalf("create episode: %v", err)
	}
	if ep.Status != "queued" {
		t.Fatalf("initial status = %q, want queued", ep.Status)
	}

	for _, want := range []string{"downloading", "downloaded", "encoding", "encoded", "archived"} {
		if err := s.Write().SetEpisodeStatus(ctx, SetEpisodeStatusParams{Status: want, ID: ep.ID}); err != nil {
			t.Fatalf("set status %q: %v", want, err)
		}
		got, err := s.Read().GetEpisode(ctx, ep.ID)
		if err != nil {
			t.Fatalf("get episode: %v", err)
		}
		if got.Status != want {
			t.Fatalf("status = %q, want %q", got.Status, want)
		}
	}

	// CHECK constraint must reject an invalid status.
	if err := s.Write().SetEpisodeStatus(ctx, SetEpisodeStatusParams{Status: "bogus", ID: ep.ID}); err == nil {
		t.Error("expected CHECK violation for invalid status, got nil")
	}
}

func TestCrashRecoveryResetsOrphanedStatuses(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	series, err := s.Write().CreateSeries(ctx, CreateSeriesParams{
		Uuid: newUUID(), Title: "Recover", SeasonNumber: 1, PosterPortrait: 1,
	})
	if err != nil {
		t.Fatalf("create series: %v", err)
	}

	mkEpisode := func(status string) Episode {
		ep, err := s.Write().CreateEpisode(ctx, CreateEpisodeParams{
			Uuid: newUUID(), SeriesID: series.ID, SourceKind: "torrent", Status: "queued",
		})
		if err != nil {
			t.Fatalf("create episode: %v", err)
		}
		if err := s.Write().SetEpisodeStatus(ctx, SetEpisodeStatusParams{Status: status, ID: ep.ID}); err != nil {
			t.Fatalf("set status %q: %v", status, err)
		}
		return ep
	}

	downloading := mkEpisode("downloading")
	encoding := mkEpisode("encoding")

	out, err := s.Write().CreateEncodedOutput(ctx, CreateEncodedOutputParams{
		Uuid: newUUID(), EpisodeID: encoding.ID, Resolution: 1080, Status: "queued",
	})
	if err != nil {
		t.Fatalf("create encoded output: %v", err)
	}
	if err := s.Write().SetEncodedOutputStatus(ctx, SetEncodedOutputStatusParams{Status: "thumbnailing", ID: out.ID}); err != nil {
		t.Fatalf("set output status: %v", err)
	}

	if err := s.recoverOrphaned(ctx); err != nil {
		t.Fatalf("recoverOrphaned: %v", err)
	}

	gotDl, _ := s.Read().GetEpisode(ctx, downloading.ID)
	if gotDl.Status != "queued" {
		t.Errorf("downloading episode reset to %q, want queued", gotDl.Status)
	}
	gotEnc, _ := s.Read().GetEpisode(ctx, encoding.ID)
	if gotEnc.Status != "downloaded" {
		t.Errorf("encoding episode reset to %q, want downloaded", gotEnc.Status)
	}
	gotOut, _ := s.Read().GetEncodedOutput(ctx, out.ID)
	if gotOut.Status != "queued" {
		t.Errorf("thumbnailing output reset to %q, want queued", gotOut.Status)
	}
}

func TestSettingsSingletonGetUpdate(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	cur, err := s.Read().GetSettings(ctx)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}

	params := UpdateSettingsParams{
		DownloadRoot:        "/tmp/dl",
		EncodedRoot:         "/tmp/lib",
		CleanupPolicy:       "move",
		ProcessedDir:        p("/tmp/processed"),
		NamingTemplate:      cur.NamingTemplate,
		DownloadBackend:     cur.DownloadBackend,
		DefaultProfileID:    cur.DefaultProfileID,
		ConcurrencyDownload: 5,
		ConcurrencyEncode:   2,
		Port:                9000,
		DohEnabled:          0,
	}
	updated, err := s.Write().UpdateSettings(ctx, params)
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if updated.ID != 1 {
		t.Errorf("updated id = %d, want 1 (singleton)", updated.ID)
	}
	if updated.CleanupPolicy != "move" || updated.ConcurrencyDownload != 5 || updated.Port != 9000 || updated.DohEnabled != 0 {
		t.Errorf("update not applied: %+v", updated)
	}

	reread, err := s.Read().GetSettings(ctx)
	if err != nil {
		t.Fatalf("re-read settings: %v", err)
	}
	if reread.CleanupPolicy != "move" || reread.ConcurrencyDownload != 5 {
		t.Errorf("re-read mismatch: %+v", reread)
	}
}
