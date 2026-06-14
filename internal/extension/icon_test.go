package extension

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// installWithIcon upserts a minimal extension row carrying the given icon URL
// (empty string means no icon) and returns its DB id.
func installWithIcon(t *testing.T, st *store.Store, icon string) int64 {
	t.Helper()
	var iconPtr *string
	if icon != "" {
		iconPtr = &icon
	}
	ext, err := st.Write().UpsertExtensionByExtID(context.Background(), store.UpsertExtensionByExtIDParams{
		Uuid:    uuid.NewString(),
		ExtID:   "test.icon." + uuid.NewString(),
		Name:    "Icon Fixture",
		Type:    ExtTypeTorrent,
		Lang:    "javascript",
		Enabled: 1,
		Icon:    iconPtr,
	})
	if err != nil {
		t.Fatalf("UpsertExtensionByExtID: %v", err)
	}
	return ext.ID
}

func TestFetchIcon(t *testing.T) {
	st := openStore(t)
	mgr := NewManager(st, source.NewRegistry(), http.DefaultClient, t.TempDir(), testLogger(t))
	ctx := context.Background()

	imgBytes := []byte{0x89, 'P', 'N', 'G'}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/icon.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imgBytes)
		case "/not-image":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	mgr.httpClient = srv.Client()

	t.Run("image", func(t *testing.T) {
		id := installWithIcon(t, st, srv.URL+"/icon.png")
		ct, body, err := mgr.FetchIcon(ctx, id)
		if err != nil {
			t.Fatalf("FetchIcon: %v", err)
		}
		if ct != "image/png" {
			t.Errorf("content-type = %q, want image/png", ct)
		}
		if string(body) != string(imgBytes) {
			t.Errorf("body = %x, want %x", body, imgBytes)
		}
	})

	t.Run("non-image upstream", func(t *testing.T) {
		id := installWithIcon(t, st, srv.URL+"/not-image")
		if _, _, err := mgr.FetchIcon(ctx, id); !errors.Is(err, ErrNoIcon) {
			t.Errorf("err = %v, want ErrNoIcon", err)
		}
	})

	t.Run("upstream 404", func(t *testing.T) {
		id := installWithIcon(t, st, srv.URL+"/missing")
		if _, _, err := mgr.FetchIcon(ctx, id); !errors.Is(err, ErrNoIcon) {
			t.Errorf("err = %v, want ErrNoIcon", err)
		}
	})

	t.Run("no icon", func(t *testing.T) {
		id := installWithIcon(t, st, "")
		if _, _, err := mgr.FetchIcon(ctx, id); !errors.Is(err, ErrNoIcon) {
			t.Errorf("err = %v, want ErrNoIcon", err)
		}
	})

	t.Run("missing extension", func(t *testing.T) {
		if _, _, err := mgr.FetchIcon(ctx, 999999); err == nil {
			t.Error("expected error for missing extension")
		}
	})
}
