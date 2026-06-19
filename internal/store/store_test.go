package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/defaults"
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
	// download_backend is seeded null ("Auto"): the queue resolves it to the
	// default download client at run time rather than pinning a client id.
	if set.DownloadBackend != nil {
		t.Errorf("settings download_backend = %v, want nil (Auto)", set.DownloadBackend)
	}
	if set.NamingTemplate != defaultNamingTemplate {
		t.Errorf("naming_template = %q, want %q", set.NamingTemplate, defaultNamingTemplate)
	}
}

// TestTrustedReleaseGroupsRoundTrip verifies migration 00013: a fresh install
// seeds the default JSON allowlist, and UpdateSettings persists a replacement —
// including an explicitly-empty array — verbatim.
func TestTrustedReleaseGroupsRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	set, err := s.Read().GetSettings(ctx)
	if err != nil {
		t.Fatalf("settings missing: %v", err)
	}
	if set.TrustedReleaseGroups != defaultTrustedReleaseGroups {
		t.Errorf("seeded trusted_release_groups = %q, want %q", set.TrustedReleaseGroups, defaultTrustedReleaseGroups)
	}

	base := UpdateSettingsParams{
		DownloadRoot:        set.DownloadRoot,
		EncodedRoot:         set.EncodedRoot,
		CleanupPolicy:       set.CleanupPolicy,
		ProcessedDir:        set.ProcessedDir,
		NamingTemplate:      set.NamingTemplate,
		DownloadBackend:     set.DownloadBackend,
		DefaultProfileID:    set.DefaultProfileID,
		ConcurrencyDownload: set.ConcurrencyDownload,
		ConcurrencyEncode:   set.ConcurrencyEncode,
		FfmpegPath:          set.FfmpegPath,
		YtdlpPath:           set.YtdlpPath,
		Port:                set.Port,
		DohEnabled:          set.DohEnabled,
		SetupCompleted:      set.SetupCompleted,
		ShowNsfw:            set.ShowNsfw,
	}

	custom := base
	custom.TrustedReleaseGroups = `["ASW"]`
	if _, err := s.Write().UpdateSettings(ctx, custom); err != nil {
		t.Fatalf("UpdateSettings custom: %v", err)
	}
	got, _ := s.Read().GetSettings(ctx)
	if got.TrustedReleaseGroups != `["ASW"]` {
		t.Errorf("after update trusted_release_groups = %q, want [\"ASW\"]", got.TrustedReleaseGroups)
	}

	empty := base
	empty.TrustedReleaseGroups = `[]`
	if _, err := s.Write().UpdateSettings(ctx, empty); err != nil {
		t.Fatalf("UpdateSettings empty: %v", err)
	}
	got, _ = s.Read().GetSettings(ctx)
	if got.TrustedReleaseGroups != `[]` {
		t.Errorf("after empty update trusted_release_groups = %q, want []", got.TrustedReleaseGroups)
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
	wantProfiles := len(defaults.Values.Profiles)
	if len(profiles) != wantProfiles {
		t.Errorf("profile count = %d, want %d (no duplicates after re-seed)", len(profiles), wantProfiles)
	}
	clients, err := s.Read().ListDownloadClients(ctx)
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	if len(clients) != 1 {
		t.Errorf("download client count = %d, want 1", len(clients))
	}
}

// TestSourceCleanedAtRoundTrip verifies migration 00007: the nullable
// source_cleaned_at column defaults to NULL on a fresh episode and persists a
// stamped unix value through MarkEpisodeSourceCleaned.
func TestSourceCleanedAtRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	series, err := s.Write().CreateSeries(ctx, CreateSeriesParams{
		Uuid: newUUID(), Title: "Cleanup Series", SeasonNumber: 1,
	})
	if err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}
	ep, err := s.Write().CreateEpisode(ctx, CreateEpisodeParams{
		Uuid: newUUID(), SeriesID: series.ID, SourceKind: "torrent", Status: "downloaded",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	if ep.SourceCleanedAt != nil {
		t.Errorf("fresh episode source_cleaned_at = %v, want nil", ep.SourceCleanedAt)
	}

	now := int64(1_700_000_000)
	if err := s.Write().MarkEpisodeSourceCleaned(ctx, MarkEpisodeSourceCleanedParams{
		SourceCleanedAt: &now, ID: ep.ID,
	}); err != nil {
		t.Fatalf("MarkEpisodeSourceCleaned: %v", err)
	}

	reread, err := s.Read().GetEpisode(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEpisode: %v", err)
	}
	if reread.SourceCleanedAt == nil || *reread.SourceCleanedAt != now {
		t.Errorf("source_cleaned_at = %v, want %d", reread.SourceCleanedAt, now)
	}

	// The join query also surfaces the column.
	withSeries, err := s.Read().GetEpisodeWithSeries(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEpisodeWithSeries: %v", err)
	}
	if withSeries.SeriesTitle != "Cleanup Series" {
		t.Errorf("series_title = %q, want %q", withSeries.SeriesTitle, "Cleanup Series")
	}
	if withSeries.Episode.SourceCleanedAt == nil || *withSeries.Episode.SourceCleanedAt != now {
		t.Errorf("joined source_cleaned_at = %v, want %d", withSeries.Episode.SourceCleanedAt, now)
	}
}

func TestNoBuiltinExtensionsSeeded(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	exts, err := s.Read().ListExtensions(ctx)
	if err != nil {
		t.Fatalf("list extensions: %v", err)
	}
	if len(exts) != 0 {
		t.Errorf("extension count = %d, want 0 (sourcing is extensions-only)", len(exts))
	}
}

func TestCreateAndListTorrentExtension(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	row, err := s.Write().CreateExtension(ctx, CreateExtensionParams{
		Uuid:      newUUID(),
		ExtID:     "test.ext.torrent",
		Name:      "Test Torrent",
		Type:      "torrent",
		Lang:      "javascript",
		Payload:   p("export default {};"),
		Enabled:   1,
		IsBuiltin: 0,
		Nsfw:      0,
		Icon:      nil,
	})
	if err != nil {
		t.Fatalf("create extension: %v", err)
	}
	if row.Type != "torrent" {
		t.Errorf("type = %q, want torrent", row.Type)
	}

	enabled, err := s.Read().ListEnabledExtensionsByType(ctx, "torrent")
	if err != nil {
		t.Fatalf("list enabled by type: %v", err)
	}
	if len(enabled) != 1 || enabled[0].ExtID != "test.ext.torrent" {
		t.Fatalf("enabled torrent extensions = %+v, want one (test.ext.torrent)", enabled)
	}
}

func TestSettingsSetupAndNsfwRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	cur, err := s.Read().GetSettings(ctx)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if cur.SetupCompleted != 0 || cur.ShowNsfw != 0 {
		t.Errorf("fresh settings setup_completed=%d show_nsfw=%d, want 0/0", cur.SetupCompleted, cur.ShowNsfw)
	}

	if _, err := s.Write().UpdateSettings(ctx, UpdateSettingsParams{
		DownloadRoot:        cur.DownloadRoot,
		EncodedRoot:         cur.EncodedRoot,
		CleanupPolicy:       cur.CleanupPolicy,
		ProcessedDir:        cur.ProcessedDir,
		NamingTemplate:      cur.NamingTemplate,
		DownloadBackend:     cur.DownloadBackend,
		DefaultProfileID:    cur.DefaultProfileID,
		ConcurrencyDownload: cur.ConcurrencyDownload,
		ConcurrencyEncode:   cur.ConcurrencyEncode,
		FfmpegPath:          cur.FfmpegPath,
		YtdlpPath:           cur.YtdlpPath,
		Port:                cur.Port,
		DohEnabled:          cur.DohEnabled,
		SetupCompleted:      1,
		ShowNsfw:            1,
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	reread, err := s.Read().GetSettings(ctx)
	if err != nil {
		t.Fatalf("re-read settings: %v", err)
	}
	if reread.SetupCompleted != 1 || reread.ShowNsfw != 1 {
		t.Errorf("after update setup_completed=%d show_nsfw=%d, want 1/1", reread.SetupCompleted, reread.ShowNsfw)
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
