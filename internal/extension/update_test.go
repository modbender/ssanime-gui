package extension

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// updateRepoServer serves a repo index.json at /index.json and the JS payload at
// /fixture.js. The index version is read live from *version so a test can flip it
// between an install and an auto-update pass. Options carries one boolean schema
// key ("useTorrent") so settings-merge behaviour is observable.
func updateRepoServer(t *testing.T, version *string) *httptest.Server {
	t.Helper()
	body := loadFixture(t, "fixture.js")
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/fixture.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		entries := []IndexEntry{{
			ID:      "test.extension.fixture",
			Name:    "Fixture",
			Version: *version,
			Type:    ExtTypeTorrent,
			Code:    srv.URL + "/fixture.js",
			Options: json.RawMessage(`{"useTorrent":{"type":"boolean","value":true},"newOpt":{"type":"boolean","value":false}}`),
		}}
		_ = json.NewEncoder(w).Encode(entries)
	})
	return srv
}

func installFixture(t *testing.T, mgr *Manager, srv *httptest.Server, version string) (store.ExtensionRepo, store.Extension) {
	t.Helper()
	ctx := context.Background()
	repo, err := mgr.AddRepo(ctx, "Test Repo", srv.URL+"/index.json")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	entry := IndexEntry{
		ID:      "test.extension.fixture",
		Name:    "Fixture",
		Version: version,
		Type:    ExtTypeTorrent,
		Code:    srv.URL + "/fixture.js",
		Options: json.RawMessage(`{"useTorrent":{"type":"boolean","value":true}}`),
	}
	ext, err := mgr.InstallExtension(ctx, entry, repo.ID)
	if err != nil {
		t.Fatalf("InstallExtension: %v", err)
	}
	return repo, ext
}

func TestAutoUpdateAppliesNewerVersion(t *testing.T) {
	st := openStore(t)
	reg := source.NewRegistry()
	version := "1.0.0"
	srv := updateRepoServer(t, &version)
	mgr := NewManager(st, reg, srv.Client(), t.TempDir(), testLogger(t))
	ctx := context.Background()

	_, ext := installFixture(t, mgr, srv, "1.0.0")
	origUUID := ext.Uuid
	origID := ext.ID

	// A user override on an existing setting must survive the update.
	override := `{"useTorrent":false}`
	if err := st.Write().UpdateExtensionSettings(ctx, store.UpdateExtensionSettingsParams{
		Settings: &override, ID: ext.ID,
	}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	version = "1.1.0"
	mgr.AutoUpdateAll(ctx)

	row, err := st.Read().GetExtension(ctx, ext.ID)
	if err != nil {
		t.Fatalf("GetExtension: %v", err)
	}
	if row.Version == nil || *row.Version != "1.1.0" {
		t.Errorf("version = %v, want 1.1.0", row.Version)
	}
	// Identity preserved.
	if row.ID != origID || row.Uuid != origUUID {
		t.Errorf("identity changed: id %d->%d uuid %q->%q", origID, row.ID, origUUID, row.Uuid)
	}
	// User override kept, new default merged under it.
	settings := resolveSettings(nil, row.Settings)
	if settings["useTorrent"] != false {
		t.Errorf("user override lost: useTorrent = %v, want false", settings["useTorrent"])
	}
	if settings["newOpt"] != false {
		t.Errorf("new default not merged: newOpt = %v, want false", settings["newOpt"])
	}
	// Enabled extension's provider is registered after update.
	if _, ok := reg.Get("test.extension.fixture"); !ok {
		t.Error("enabled provider not registered after update")
	}
}

func TestAutoUpdateEqualVersionIsNoOp(t *testing.T) {
	st := openStore(t)
	reg := source.NewRegistry()
	version := "1.0.0"
	srv := updateRepoServer(t, &version)

	// Count payload fetches so a no-op pass is observable (no re-download).
	var fetches int64
	base := srv.Client()
	mgr := NewManager(st, reg, base, t.TempDir(), testLogger(t))
	ctx := context.Background()

	_, ext := installFixture(t, mgr, srv, "1.0.0")
	before, _ := st.Read().GetExtension(ctx, ext.ID)

	// Wrap the client to count payload requests during the pass only.
	mgr.httpClient = &http.Client{Transport: countingTransport{base.Transport, &fetches}}
	version = "1.0.0" // unchanged
	mgr.AutoUpdateAll(ctx)

	if n := atomic.LoadInt64(&fetches); n != 0 {
		t.Errorf("payload fetched %d times on equal-version pass, want 0", n)
	}
	after, _ := st.Read().GetExtension(ctx, ext.ID)
	if after.ModifiedAt != before.ModifiedAt {
		t.Errorf("row modified on no-op pass: %d -> %d", before.ModifiedAt, after.ModifiedAt)
	}
}

func TestAutoUpdateDisabledStaysDisabledAndUnregistered(t *testing.T) {
	st := openStore(t)
	reg := source.NewRegistry()
	version := "1.0.0"
	srv := updateRepoServer(t, &version)
	mgr := NewManager(st, reg, srv.Client(), t.TempDir(), testLogger(t))
	ctx := context.Background()

	_, ext := installFixture(t, mgr, srv, "1.0.0")
	if err := mgr.DisableExtension(ctx, ext.ID); err != nil {
		t.Fatalf("DisableExtension: %v", err)
	}
	if _, ok := reg.Get("test.extension.fixture"); ok {
		t.Fatal("provider registered while disabled (precondition)")
	}

	version = "1.1.0"
	mgr.AutoUpdateAll(ctx)

	row, err := st.Read().GetExtension(ctx, ext.ID)
	if err != nil {
		t.Fatalf("GetExtension: %v", err)
	}
	if row.Enabled != 0 {
		t.Errorf("enabled flag flipped on update: %d, want 0", row.Enabled)
	}
	if row.Version == nil || *row.Version != "1.1.0" {
		t.Errorf("disabled extension not updated: version = %v, want 1.1.0", row.Version)
	}
	if _, ok := reg.Get("test.extension.fixture"); ok {
		t.Error("disabled provider was registered by update")
	}
}

func TestAutoUpdateBadRepoIndexSkipsAndContinues(t *testing.T) {
	st := openStore(t)
	reg := source.NewRegistry()
	mgr := NewManager(st, reg, http.DefaultClient, t.TempDir(), testLogger(t))
	ctx := context.Background()

	// A repo whose index URL is unreachable must not panic or abort the pass.
	if _, err := mgr.AddRepo(ctx, "Dead", "http://127.0.0.1:1/index.json"); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	mgr.AutoUpdateAll(ctx) // must return without error/panic
}

type countingTransport struct {
	base http.RoundTripper
	n    *int64
}

func (c countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := c.base
	if rt == nil {
		rt = http.DefaultTransport
	}
	if req.URL.Path == "/fixture.js" {
		atomic.AddInt64(c.n, 1)
	}
	return rt.RoundTrip(req)
}
