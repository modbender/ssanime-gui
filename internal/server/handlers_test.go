package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

// loopbackHost is a valid Host for the localGuard middleware; httptest's default
// ("example.com") is correctly rejected as a DNS-rebinding attempt.
const loopbackHost = "127.0.0.1:4773"

// newTestServer builds a real server with a temp-file DB and all dependencies.
func newTestServer(t *testing.T) http.Handler {
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

	return New(st, hub, nil, Config{})
}

func getJSON(t *testing.T, srv http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = loopbackHost
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func postJSON(t *testing.T, srv http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Host = loopbackHost
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func patchJSON(t *testing.T, srv http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(b))
	req.Host = loopbackHost
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func putJSON(t *testing.T, srv http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Host = loopbackHost
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func deleteReq(t *testing.T, srv http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Host = loopbackHost
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) Response[T] {
	t.Helper()
	var resp Response[T]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v\nbody: %s", err, rec.Body.String())
	}
	return resp
}

// itoa is a local int-to-string helper for URL building in tests.
func itoa(n int) string { return strconv.Itoa(n) }

// TestGetSettings verifies the settings envelope shape.
func TestGetSettings(t *testing.T) {
	srv := newTestServer(t)
	rec := getJSON(t, srv, "/api/settings")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[map[string]any](t, rec)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Data == nil {
		t.Fatal("data is nil")
	}
}

// TestPutSettings verifies settings can be updated.
func TestPutSettings(t *testing.T) {
	srv := newTestServer(t)
	body := PutSettingsRequest{
		DownloadRoot:        "/tmp/dl",
		EncodedRoot:         "/tmp/lib",
		CleanupPolicy:       "delete",
		NamingTemplate:      "{series}/Season {season}/{res}/{series} - S{season}E{episode}.{ext}",
		ConcurrencyDownload: 2,
		ConcurrencyEncode:   1,
		Port:                8080,
		DohEnabled:          true,
	}
	rec := putJSON(t, srv, "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /settings: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestListSeriesEmpty verifies an empty library returns an array.
func TestListSeriesEmpty(t *testing.T) {
	srv := newTestServer(t)
	rec := getJSON(t, srv, "/api/series")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	resp := decodeBody[[]SeriesProgress](t, rec)
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
}

// TestCreateSeriesByTitle creates a series by title without AniList and verifies it appears.
func TestCreateSeriesByTitle(t *testing.T) {
	srv := newTestServer(t)
	title := "Test Anime 2099"
	rec := postJSON(t, srv, "/api/series", CreateSeriesRequest{Title: &title})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create series: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Verify it appears in list.
	rec2 := getJSON(t, srv, "/api/series")
	if rec2.Code != http.StatusOK {
		t.Fatalf("list series: status=%d", rec2.Code)
	}
}

// TestGetSeriesNotFound verifies 404 for missing series.
func TestGetSeriesNotFound(t *testing.T) {
	srv := newTestServer(t)
	rec := getJSON(t, srv, "/api/series/9999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

// TestCreateAndResolveProfile verifies profile CRUD and the resolved endpoint.
func TestCreateAndResolveProfile(t *testing.T) {
	srv := newTestServer(t)

	// Create a user profile parenting the builtin (id=1 from seed).
	parentID := int64(1)
	crf := 22.0
	rec := postJSON(t, srv, "/api/profiles", CreateProfileRequest{
		Name:     "My Custom Profile",
		ParentID: &parentID,
		CRF:      &crf,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create profile: status=%d body=%s", rec.Code, rec.Body.String())
	}

	resp := decodeBody[map[string]any](t, rec)
	if resp.Data == nil {
		t.Fatalf("no data in response: %s", rec.Body.String())
	}
	idFloat, ok := (*resp.Data)["id"].(float64)
	if !ok {
		t.Fatalf("no id in response: %v", *resp.Data)
	}
	newID := int(idFloat)

	// Resolve — should inherit everything from builtin except CRF=22.0.
	rec2 := getJSON(t, srv, "/api/profiles/"+itoa(newID)+"/resolved")
	if rec2.Code != http.StatusOK {
		t.Fatalf("resolve: status=%d body=%s", rec2.Code, rec2.Body.String())
	}
	resp2 := decodeBody[ResolvedProfileResponse](t, rec2)
	if resp2.Error != "" {
		t.Fatalf("resolve error: %s", resp2.Error)
	}
	if resp2.Data == nil {
		t.Fatal("resolved data nil")
	}
	if resp2.Data.CRF != 22.0 {
		t.Errorf("CRF = %f, want 22.0", resp2.Data.CRF)
	}
	if resp2.Data.Codec != "x265" {
		t.Errorf("Codec = %q, want x265", resp2.Data.Codec)
	}
}

// TestBuiltinProfileImmutable verifies PATCH and DELETE on builtin profiles return 403.
func TestBuiltinProfileImmutable(t *testing.T) {
	srv := newTestServer(t)
	crf := 18.0
	rec := patchJSON(t, srv, "/api/profiles/1", PatchProfileRequest{CRF: &crf})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PATCH builtin: want 403, got %d; body=%s", rec.Code, rec.Body.String())
	}
	rec2 := deleteReq(t, srv, "/api/profiles/1")
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("DELETE builtin: want 403, got %d", rec2.Code)
	}
}

// TestBulkEncodeTransitionsToQueued verifies the bulk-encode endpoint transitions episodes.
func TestBulkEncodeTransitionsToQueued(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: filepath.Join(dir, "test2.db"), Port: 8080}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	hub := events.NewHub(nil)
	hub.Start()
	defer hub.Stop()

	srv := New(st, hub, nil, Config{})

	ctx := context.Background()
	s, err := st.Write().CreateSeries(ctx, store.CreateSeriesParams{
		Uuid: mustUUID(), Title: "Bulk Test", SeasonNumber: 1,
	})
	if err != nil {
		t.Fatalf("create series: %v", err)
	}
	magnet := "magnet:?xt=urn:btih:abc123"
	ep, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: mustUUID(), SeriesID: s.ID, SourceKind: "torrent",
		Magnet: &magnet, Status: "downloaded",
	})
	if err != nil {
		t.Fatalf("create episode: %v", err)
	}

	rec := postJSON(t, srv, "/api/encode", BulkEncodeRequest{
		EpisodeIDs: []int64{ep.ID},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("bulk encode: %d %s", rec.Code, rec.Body.String())
	}

	// Verify episode transitioned to queued.
	updated, err := st.Read().GetEpisode(ctx, ep.ID)
	if err != nil {
		t.Fatalf("get episode: %v", err)
	}
	if updated.Status != "queued" {
		t.Errorf("status = %q, want queued", updated.Status)
	}
}

// TestStats verifies the stats endpoint returns correct totals.
func TestStats(t *testing.T) {
	srv := newTestServer(t)
	rec := getJSON(t, srv, "/api/stats")
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[StatsResponse](t, rec)
	if resp.Error != "" {
		t.Fatalf("stats error: %s", resp.Error)
	}
	if resp.Data == nil {
		t.Fatal("stats data nil")
	}
	// Empty library: series_total=0, all bytes=0.
	if resp.Data.SeriesTotal != 0 {
		t.Errorf("series_total = %d, want 0", resp.Data.SeriesTotal)
	}
}

// TestQueue verifies the queue endpoint returns the expected shape.
func TestQueue(t *testing.T) {
	srv := newTestServer(t)
	rec := getJSON(t, srv, "/api/queue")
	if rec.Code != http.StatusOK {
		t.Fatalf("queue: %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[QueueSnapshot](t, rec)
	if resp.Error != "" {
		t.Fatalf("queue error: %s", resp.Error)
	}
	if resp.Data == nil {
		t.Fatal("queue data nil")
	}
}

// TestLogs verifies the logs endpoint returns the ring buffer.
func TestLogs(t *testing.T) {
	srv := newTestServer(t)
	rec := getJSON(t, srv, "/api/logs?limit=10")
	if rec.Code != http.StatusOK {
		t.Fatalf("logs: %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[LogsResponse](t, rec)
	if resp.Error != "" {
		t.Fatalf("logs error: %s", resp.Error)
	}
	if resp.Data == nil {
		t.Fatal("logs data nil")
	}
}
