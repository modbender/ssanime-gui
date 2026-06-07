package encode

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/store"
)

// fakeEncoder writes placeholder files instead of shelling out to ffmpeg, so the
// fan-out / archive / cleanup state machine runs without ffmpeg. failRes != 0
// makes that one resolution fail (to exercise the keep-original path).
type fakeEncoder struct {
	calls   int32
	failRes int
}

func (f *fakeEncoder) Encode(_ context.Context, req EncodeRequest, onProgress ProgressFunc) (EncodeResult, error) {
	atomic.AddInt32(&f.calls, 1)
	if onProgress != nil {
		onProgress(50, "2.0x")
		onProgress(100, "2.0x")
	}
	if f.failRes != 0 && req.Resolution == f.failRes {
		return EncodeResult{}, context.DeadlineExceeded // any non-cancel error
	}
	if err := os.MkdirAll(filepath.Dir(req.Output), 0o755); err != nil {
		return EncodeResult{}, err
	}
	// Write a "smaller than source" placeholder.
	if err := os.WriteFile(req.Output, []byte("encoded-"+req.Output), 0o644); err != nil {
		return EncodeResult{}, err
	}
	return EncodeResult{Snapshot: `{"fake":true}`, Size: 10}, nil
}

func (f *fakeEncoder) Thumbnails(_ context.Context, _, destDir string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for i := 0; i < 3; i++ {
		p := filepath.Join(destDir, "0"+string(rune('0'+i))+".jpg")
		if err := os.WriteFile(p, []byte("img"), 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

func openTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: filepath.Join(dir, "test.db"), Port: config.DefaultPort}
	s, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}

// seedDownloadedEpisode creates a series + a downloaded episode with a real
// source file on disk, returning the episode id and source dir.
func seedDownloadedEpisode(t *testing.T, st *store.Store, dataDir string, episodeNo *int64) (int64, string) {
	t.Helper()
	ctx := context.Background()

	series, err := st.Write().CreateSeries(ctx, store.CreateSeriesParams{
		Uuid: "s1", Title: "Test Series", SeasonNumber: 1,
	})
	if err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}

	srcDir := filepath.Join(dataDir, "downloads", "ep1")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	srcFile := filepath.Join(srcDir, "source.mkv")
	if err := os.WriteFile(srcFile, []byte("the-original-large-source-file"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	ep, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: "e1", SeriesID: series.ID, SourceKind: "torrent",
		Status: "queued", EpisodeNo: episodeNo,
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	if err := st.Write().MarkEpisodeDownloaded(ctx, store.MarkEpisodeDownloadedParams{
		SourcePath: &srcFile, SourceSize: ptr[int64](30), ID: ep.ID,
	}); err != nil {
		t.Fatalf("MarkEpisodeDownloaded: %v", err)
	}
	return ep.ID, srcDir
}

func waitEpisodeStatus(t *testing.T, st *store.Store, id int64, want string, timeout time.Duration) store.Episode {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ep, err := st.Read().GetEpisode(context.Background(), id)
		if err != nil {
			t.Fatalf("GetEpisode: %v", err)
		}
		if ep.Status == want {
			return ep
		}
		time.Sleep(5 * time.Millisecond)
	}
	ep, _ := st.Read().GetEpisode(context.Background(), id)
	t.Fatalf("episode %d status=%q, want %q", id, ep.Status, want)
	return store.Episode{}
}

func TestQueueFansOutAndArchivesAllResolutions(t *testing.T) {
	st, dataDir := openTestStore(t)
	id, srcDir := seedDownloadedEpisode(t, st, dataDir, ptr[int64](5))

	// Encoded root on the same drive so move works in CI.
	encRoot := filepath.Join(dataDir, "library")
	updateEncodedRoot(t, st, encRoot, "delete")

	fe := &fakeEncoder{}
	q := New(st, fe, nil, Options{Workers: 1, DataDir: dataDir})
	q.Start()
	defer q.Stop()

	ep := waitEpisodeStatus(t, st, id, "archived", 5*time.Second)
	_ = ep

	// One encoded_outputs row per default resolution, all archived.
	outs, err := st.Read().ListEncodedOutputsByEpisode(context.Background(), id)
	if err != nil {
		t.Fatalf("ListEncodedOutputsByEpisode: %v", err)
	}
	if len(outs) != 3 {
		t.Fatalf("got %d outputs, want 3 (1080/720/480)", len(outs))
	}
	for _, o := range outs {
		if o.Status != "archived" {
			t.Errorf("output %dp status=%q, want archived", o.Resolution, o.Status)
		}
		if o.EncodedPath == nil || !fileExists(*o.EncodedPath) {
			t.Errorf("output %dp encoded file missing: %v", o.Resolution, o.EncodedPath)
		}
		if o.EncodedParamsSnapshot == nil {
			t.Errorf("output %dp missing params snapshot", o.Resolution)
		}
	}

	// Each output lives at its Jellyfin path under the right res subfolder.
	want720 := filepath.Join(encRoot, "Test Series", "Season 01", "720p", "Test Series - S01E05.mkv")
	if !fileExists(want720) {
		t.Errorf("expected 720p output at %s", want720)
	}

	// Screenshots created from the highest-res output.
	shots, err := st.Read().ListScreenshotsByEpisode(context.Background(), id)
	if err != nil {
		t.Fatalf("ListScreenshotsByEpisode: %v", err)
	}
	if len(shots) == 0 {
		t.Error("expected screenshots from the thumbnail pass")
	}

	// Cleanup policy = delete: the original source dir is gone, source_path NULL.
	if fileExists(filepath.Join(srcDir, "source.mkv")) {
		t.Error("original source should be deleted after full archive")
	}
	if ep2, _ := st.Read().GetEpisode(context.Background(), id); ep2.SourcePath != nil {
		t.Errorf("source_path should be NULL after delete, got %v", ep2.SourcePath)
	}
}

func TestQueueKeepsOriginalOnOutputError(t *testing.T) {
	st, dataDir := openTestStore(t)
	id, srcDir := seedDownloadedEpisode(t, st, dataDir, ptr[int64](1))
	updateEncodedRoot(t, st, filepath.Join(dataDir, "library"), "delete")

	// Fail the 480p output; 1080/720 succeed.
	fe := &fakeEncoder{failRes: 480}
	q := New(st, fe, nil, Options{Workers: 1, DataDir: dataDir})
	q.Start()
	defer q.Stop()

	ep := waitEpisodeStatus(t, st, id, "error", 5*time.Second)
	if ep.ErrorMessage == nil {
		t.Error("expected an error_message when an output fails")
	}
	// Original must be kept for retry.
	if !fileExists(filepath.Join(srcDir, "source.mkv")) {
		t.Error("original must be kept when any output errors")
	}

	outs, _ := st.Read().ListEncodedOutputsByEpisode(context.Background(), id)
	var archived, errored int
	for _, o := range outs {
		switch o.Status {
		case "archived":
			archived++
		case "error":
			errored++
		}
	}
	if archived != 2 || errored != 1 {
		t.Errorf("got archived=%d errored=%d, want 2/1", archived, errored)
	}
}

func TestQueueMovePolicy(t *testing.T) {
	st, dataDir := openTestStore(t)
	id, srcDir := seedDownloadedEpisode(t, st, dataDir, ptr[int64](2))
	processed := filepath.Join(dataDir, "processed")
	updateEncodedRoot(t, st, filepath.Join(dataDir, "library"), "move")
	setProcessedDir(t, st, processed)

	fe := &fakeEncoder{}
	q := New(st, fe, nil, Options{Workers: 1, DataDir: dataDir})
	q.Start()
	defer q.Stop()

	waitEpisodeStatus(t, st, id, "archived", 5*time.Second)

	if fileExists(filepath.Join(srcDir, "source.mkv")) {
		t.Error("original should be moved out of the download dir")
	}
	moved := filepath.Join(processed, filepath.Base(srcDir), "source.mkv")
	if !fileExists(moved) {
		t.Errorf("expected moved original at %s", moved)
	}
	ep, _ := st.Read().GetEpisode(context.Background(), id)
	if ep.SourcePath == nil || *ep.SourcePath != moved {
		t.Errorf("source_path = %v, want %s", ep.SourcePath, moved)
	}
}

// --- store helpers ---

func updateEncodedRoot(t *testing.T, st *store.Store, encRoot, policy string) {
	t.Helper()
	ctx := context.Background()
	set, err := st.Read().GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if _, err := st.Write().UpdateSettings(ctx, store.UpdateSettingsParams{
		DownloadRoot:        set.DownloadRoot,
		EncodedRoot:         encRoot,
		CleanupPolicy:       policy,
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
	}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
}

func setProcessedDir(t *testing.T, st *store.Store, dir string) {
	t.Helper()
	ctx := context.Background()
	set, _ := st.Read().GetSettings(ctx)
	if _, err := st.Write().UpdateSettings(ctx, store.UpdateSettingsParams{
		DownloadRoot:        set.DownloadRoot,
		EncodedRoot:         set.EncodedRoot,
		CleanupPolicy:       set.CleanupPolicy,
		ProcessedDir:        &dir,
		NamingTemplate:      set.NamingTemplate,
		DownloadBackend:     set.DownloadBackend,
		DefaultProfileID:    set.DefaultProfileID,
		ConcurrencyDownload: set.ConcurrencyDownload,
		ConcurrencyEncode:   set.ConcurrencyEncode,
		FfmpegPath:          set.FfmpegPath,
		YtdlpPath:           set.YtdlpPath,
		Port:                set.Port,
		DohEnabled:          set.DohEnabled,
	}); err != nil {
		t.Fatalf("UpdateSettings(processed): %v", err)
	}
}
