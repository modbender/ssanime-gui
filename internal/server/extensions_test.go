package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/extension"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

func openServerStore(t *testing.T) *store.Store {
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

// TestHandleTestExtensionPersists drives POST /api/extensions/{id}/test against a
// real store + Manager and asserts the response reports healthy and the row's
// centralized health record was written.
func TestHandleTestExtensionPersists(t *testing.T) {
	st := openServerStore(t)
	reg := source.NewRegistry()
	mgr := extension.NewManager(st, reg, http.DefaultClient, t.TempDir(), nil)

	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)
	srv := New(st, hub, nil, Config{Registry: reg, ExtMgr: mgr})

	// An extension with a test() that resolves without network → healthy.
	payload := `export default new class { async test(){ return true } }()`
	ext, err := st.Write().UpsertExtensionByExtID(context.Background(), store.UpsertExtensionByExtIDParams{
		Uuid: uuid.NewString(), ExtID: "srv.test.ext", Name: "SrvTest",
		Type: "torrent", Lang: "javascript", Enabled: 1, Payload: &payload,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/extensions/"+strconv.FormatInt(ext.ID, 10)+"/test", nil)
	req.Host = loopbackHost
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var resp Response[ExtensionTestResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data == nil || !resp.Data.Healthy || resp.Data.Error != "" {
		t.Fatalf("response = %+v, want healthy", resp.Data)
	}
	if resp.Data.CheckedAt == 0 {
		t.Fatalf("checked_at = 0, want a timestamp")
	}

	row, err := st.Read().GetExtension(context.Background(), ext.ID)
	if err != nil {
		t.Fatalf("GetExtension: %v", err)
	}
	if row.Healthy == nil || *row.Healthy != 1 {
		t.Fatalf("persisted Healthy = %v, want 1", row.Healthy)
	}
	if row.HealthCheckedAt == nil {
		t.Fatalf("persisted HealthCheckedAt = nil, want timestamp")
	}
}
