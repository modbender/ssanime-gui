package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

// newTestServerWithStore is like newTestServer but also returns the underlying
// store so reveal tests can seed episodes/outputs and configure the roots.
func newTestServerWithStore(t *testing.T) (http.Handler, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir: dir,
		DBPath:  filepath.Join(dir, "test.db"),
		Port:    config.DefaultPort,
	}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)

	return New(st, hub, nil, Config{}), st, dir
}

// setRoots points download_root/encoded_root at the given dirs for reveal guards.
func setRoots(t *testing.T, st *store.Store, downloadRoot, encodedRoot string) {
	t.Helper()
	ctx := context.Background()
	set, err := st.Read().GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if _, err := st.Write().UpdateSettings(ctx, store.UpdateSettingsParams{
		DownloadRoot:        downloadRoot,
		EncodedRoot:         encodedRoot,
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
	}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
}

// seedEpisodeWithSource creates a series + an episode whose source_path is the
// given path (may be ""/nonexistent to drive the 404/409 cases).
func seedEpisodeWithSource(t *testing.T, st *store.Store, sourcePath string) int64 {
	t.Helper()
	ctx := context.Background()
	series, err := st.Write().CreateSeries(ctx, store.CreateSeriesParams{
		Uuid: "rev-s", Title: "Reveal Series", SeasonNumber: 1,
	})
	if err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}
	ep, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: "rev-e", SeriesID: series.ID, SourceKind: "torrent", Status: "downloaded",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	if sourcePath != "" {
		if err := st.Write().SetEpisodeSourcePath(ctx, store.SetEpisodeSourcePathParams{
			SourcePath: &sourcePath, ID: ep.ID,
		}); err != nil {
			t.Fatalf("SetEpisodeSourcePath: %v", err)
		}
	}
	return ep.ID
}

func postReveal(t *testing.T, srv http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Host = loopbackHost
	req.Header.Set("Origin", "http://"+loopbackHost)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// --- revealArgv (pure, per-GOOS) ---

func TestRevealArgv(t *testing.T) {
	const p = "/library/show/ep.mkv"
	cases := []struct {
		goos string
		want []string
	}{
		{"windows", []string{"explorer", "/select," + p}},
		{"darwin", []string{"open", "-R", p}},
		{"linux", []string{"xdg-open", filepath.Dir(p)}},
	}
	for _, c := range cases {
		t.Run(c.goos, func(t *testing.T) {
			got, err := revealArgv(c.goos, p)
			if err != nil {
				t.Fatalf("revealArgv(%s): %v", c.goos, err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("argv = %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("argv = %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestRevealArgvUnsupportedGOOS(t *testing.T) {
	if _, err := revealArgv("plan9", "/x"); err == nil {
		t.Fatal("expected error for unsupported GOOS")
	}
}

// --- pathUnderRoot guard ---

func TestPathUnderRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"data", "downloads")
	cases := []struct {
		name string
		abs  string
		want bool
	}{
		{"inside", filepath.Join(root, "show", "ep.mkv"), true},
		{"root itself", root, true},
		{"sibling prefix", root + "X", false},
		{"outside", filepath.Join(string(filepath.Separator)+"etc", "passwd"), false},
		{"traversal", filepath.Join(root, "..", "..", "etc", "passwd"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			abs, _ := filepath.Abs(filepath.Clean(c.abs))
			if got := pathUnderRoot(abs, root); got != c.want {
				t.Errorf("pathUnderRoot(%q, %q) = %v, want %v", abs, root, got, c.want)
			}
		})
	}
	if pathUnderRoot("/anything", "") {
		t.Error("empty root must deny everything")
	}
}

// --- handler: episode source reveal ---

func TestRevealEpisodeMissingRow(t *testing.T) {
	srv, st, dir := newTestServerWithStore(t)
	setRoots(t, st, filepath.Join(dir, "dl"), filepath.Join(dir, "lib"))
	rec := postReveal(t, srv, "/api/episodes/9999/reveal")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRevealEpisodeUnsetPath(t *testing.T) {
	srv, st, dir := newTestServerWithStore(t)
	dl := filepath.Join(dir, "dl")
	setRoots(t, st, dl, filepath.Join(dir, "lib"))
	id := seedEpisodeWithSource(t, st, "") // no source_path
	rec := postReveal(t, srv, "/api/episodes/"+itoa(int(id))+"/reveal")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRevealEpisodeOutsideRoot(t *testing.T) {
	srv, st, dir := newTestServerWithStore(t)
	dl := filepath.Join(dir, "dl")
	setRoots(t, st, dl, filepath.Join(dir, "lib"))
	// A real file, but outside the configured download_root.
	outside := filepath.Join(dir, "elsewhere", "leak.mkv")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := seedEpisodeWithSource(t, st, outside)
	rec := postReveal(t, srv, "/api/episodes/"+itoa(int(id))+"/reveal")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRevealEpisodeTraversalRejected(t *testing.T) {
	srv, st, dir := newTestServerWithStore(t)
	dl := filepath.Join(dir, "dl")
	if err := os.MkdirAll(dl, 0o755); err != nil {
		t.Fatal(err)
	}
	setRoots(t, st, dl, filepath.Join(dir, "lib"))
	// Stored path uses traversal to escape the root.
	traversal := filepath.Join(dl, "..", "elsewhere", "leak.mkv")
	if err := os.MkdirAll(filepath.Dir(filepath.Clean(traversal)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Clean(traversal), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := seedEpisodeWithSource(t, st, traversal)
	rec := postReveal(t, srv, "/api/episodes/"+itoa(int(id))+"/reveal")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRevealEpisodeFileGone(t *testing.T) {
	srv, st, dir := newTestServerWithStore(t)
	dl := filepath.Join(dir, "dl")
	if err := os.MkdirAll(dl, 0o755); err != nil {
		t.Fatal(err)
	}
	setRoots(t, st, dl, filepath.Join(dir, "lib"))
	// In-root path but the file does not exist (e.g. cleaned-up source).
	gone := filepath.Join(dl, "show", "ep.mkv")
	id := seedEpisodeWithSource(t, st, gone)
	rec := postReveal(t, srv, "/api/episodes/"+itoa(int(id))+"/reveal")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
}

// --- handler: output reveal ---

func TestRevealOutputMissingRow(t *testing.T) {
	srv, st, dir := newTestServerWithStore(t)
	setRoots(t, st, filepath.Join(dir, "dl"), filepath.Join(dir, "lib"))
	rec := postReveal(t, srv, "/api/outputs/9999/reveal")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRevealOutputOutsideRoot(t *testing.T) {
	srv, st, dir := newTestServerWithStore(t)
	lib := filepath.Join(dir, "lib")
	setRoots(t, st, filepath.Join(dir, "dl"), lib)
	ctx := context.Background()
	series, _ := st.Write().CreateSeries(ctx, store.CreateSeriesParams{Uuid: "o-s", Title: "S", SeasonNumber: 1})
	ep, _ := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: "o-e", SeriesID: series.ID, SourceKind: "torrent", Status: "encoded",
	})
	out, err := st.Write().CreateEncodedOutput(ctx, store.CreateEncodedOutputParams{
		Uuid: "o-o", EpisodeID: ep.ID, Resolution: 720, Status: "encoded",
	})
	if err != nil {
		t.Fatalf("CreateEncodedOutput: %v", err)
	}
	outside := filepath.Join(dir, "elsewhere", "enc.mkv")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.Write().MarkEncodedOutputEncoded(ctx, store.MarkEncodedOutputEncodedParams{
		EncodedPath: &outside, EncodedSize: nil, ID: out.ID,
	}); err != nil {
		t.Fatalf("MarkEncodedOutputEncoded: %v", err)
	}
	rec := postReveal(t, srv, "/api/outputs/"+itoa(int(out.ID))+"/reveal")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

// --- GET /api/episodes/{id} ---

func TestGetEpisodeEndpoint(t *testing.T) {
	srv, st, _ := newTestServerWithStore(t)
	ctx := context.Background()
	series, _ := st.Write().CreateSeries(ctx, store.CreateSeriesParams{
		Uuid: "ge-s", Title: "Detail Series", SeasonNumber: 1,
	})
	ep, _ := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: "ge-e", SeriesID: series.ID, SourceKind: "torrent", Status: "downloaded",
	})

	rec := getJSON(t, srv, "/api/episodes/"+itoa(int(ep.ID)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[EpisodeDetail](t, rec)
	if resp.Error != "" || resp.Data == nil {
		t.Fatalf("unexpected envelope: err=%q data=%v", resp.Error, resp.Data)
	}
	if resp.Data.ID != ep.ID {
		t.Errorf("id = %d, want %d", resp.Data.ID, ep.ID)
	}
	if resp.Data.SeriesTitle != "Detail Series" {
		t.Errorf("series_title = %q, want %q", resp.Data.SeriesTitle, "Detail Series")
	}
}

func TestGetEpisodeNotFound(t *testing.T) {
	srv, _, _ := newTestServerWithStore(t)
	rec := getJSON(t, srv, "/api/episodes/9999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}
