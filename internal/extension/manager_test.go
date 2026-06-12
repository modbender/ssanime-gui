package extension

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir: dir,
		DBPath:  filepath.Join(dir, "test.db"),
		Port:    config.DefaultPort,
	}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// payloadServer serves the fixture.js JS payload so InstallExtension can fetch it.
func payloadServer(t *testing.T) *httptest.Server {
	t.Helper()
	body := loadFixture(t, "fixture.js")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestManagerInstallEnableDisableUninstall(t *testing.T) {
	st := openStore(t)
	reg := source.NewRegistry()
	srv := payloadServer(t)

	mgr := NewManager(st, reg, srv.Client(), t.TempDir(), testLogger(t))
	ctx := context.Background()

	repo, err := mgr.AddRepo(ctx, "Test Repo", "https://example.test/index.json")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	entry := IndexEntry{
		ID:      "test.extension.fixture",
		Name:    "Fixture",
		Version: "1.0.0",
		Type:    ExtTypeTorrent,
		NSFW:    false,
		Code:    srv.URL + "/fixture.js",
	}

	ext, err := mgr.InstallExtension(ctx, entry, repo.ID)
	if err != nil {
		t.Fatalf("InstallExtension: %v", err)
	}
	if ext.Type != ExtTypeTorrent {
		t.Errorf("installed type = %q, want %q", ext.Type, ExtTypeTorrent)
	}

	// Install registers the provider immediately.
	p, ok := reg.Get(entry.ID)
	if !ok {
		t.Fatal("provider not registered after install")
	}

	// The installed row is enabled and discoverable as a torrent extension.
	enabled, err := st.Read().ListEnabledExtensionsByType(ctx, ExtTypeTorrent)
	if err != nil {
		t.Fatalf("ListEnabledExtensionsByType: %v", err)
	}
	if len(enabled) != 1 || enabled[0].ExtID != entry.ID {
		t.Fatalf("enabled torrent extensions = %+v, want one (%s)", enabled, entry.ID)
	}

	// The registered provider drives a real SmartSearch.
	res, err := p.SmartSearch(ctx, source.SmartSearchOptions{
		Media:         source.Media{RomajiTitle: "Test Anime"},
		EpisodeNumber: 5,
	})
	if err != nil {
		t.Fatalf("SmartSearch: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("SmartSearch returned no results")
	}

	// Disable unregisters live.
	if err := mgr.DisableExtension(ctx, ext.ID); err != nil {
		t.Fatalf("DisableExtension: %v", err)
	}
	if _, ok := reg.Get(entry.ID); ok {
		t.Error("provider still registered after disable")
	}

	// Enable re-registers live.
	if err := mgr.EnableExtension(ctx, ext.ID); err != nil {
		t.Fatalf("EnableExtension: %v", err)
	}
	if _, ok := reg.Get(entry.ID); !ok {
		t.Error("provider not registered after re-enable")
	}

	// Uninstall unregisters and removes the row.
	if err := mgr.UninstallExtension(ctx, ext.ID); err != nil {
		t.Fatalf("UninstallExtension: %v", err)
	}
	if _, ok := reg.Get(entry.ID); ok {
		t.Error("provider still registered after uninstall")
	}
	if _, err := st.Read().GetExtension(ctx, ext.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetExtension after uninstall err = %v, want sql.ErrNoRows", err)
	}
}
