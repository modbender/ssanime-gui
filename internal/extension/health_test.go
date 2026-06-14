package extension

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// testExtPayload builds a JS extension whose test() fetches upstreamURL and
// throws when the response is not ok — the Hayase liveness convention.
func testExtPayload(upstreamURL string) string {
	return fmt.Sprintf(`export default new class {
		async test() {
			const res = await fetch(%q)
			if (!res.ok) throw new Error('upstream down: ' + res.status)
			return true
		}
		async single({ titles }) {
			return [{ title: 'x', link: 'magnet:?xt=urn:btih:deadbeef' }]
		}
	}()`, upstreamURL)
}

// probeExtPayload builds an extension with NO test() method whose single()
// throws or succeeds based on the upstream — exercises the probe fallback.
func probeExtPayload(upstreamURL string) string {
	return fmt.Sprintf(`export default new class {
		async single({ titles }) {
			const res = await fetch(%q)
			if (!res.ok) throw new Error('upstream down: ' + res.status)
			return [{ title: 'x', link: 'magnet:?xt=urn:btih:deadbeef' }]
		}
	}()`, upstreamURL)
}

func TestProviderTest(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/up":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(up.Close)
	client := up.Client()
	logger := testLogger(t)
	ctx := context.Background()

	t.Run("test() healthy", func(t *testing.T) {
		p, err := NewJSProvider("t.up", "Up", testExtPayload(up.URL+"/up"), client, logger)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Test(ctx); err != nil {
			t.Fatalf("Test() = %v, want nil (healthy)", err)
		}
	})

	t.Run("test() dead", func(t *testing.T) {
		p, err := NewJSProvider("t.down", "Down", testExtPayload(up.URL+"/down"), client, logger)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Test(ctx); err == nil {
			t.Fatal("Test() = nil, want error (dead upstream)")
		}
	})

	t.Run("probe fallback when test() absent", func(t *testing.T) {
		// Healthy probe.
		ph, err := NewJSProvider("p.up", "ProbeUp", probeExtPayload(up.URL+"/up"), client, logger)
		if err != nil {
			t.Fatal(err)
		}
		if err := ph.Test(ctx); err != nil {
			t.Fatalf("probe Test() = %v, want nil", err)
		}
		// Dead probe.
		pd, err := NewJSProvider("p.down", "ProbeDown", probeExtPayload(up.URL+"/down"), client, logger)
		if err != nil {
			t.Fatal(err)
		}
		if err := pd.Test(ctx); err == nil {
			t.Fatal("probe Test() = nil, want error (dead upstream)")
		}
	})
}

// TestHealthRecorder verifies the recorder hook fires on every run path and that
// the manager's wired recorder persists one health record per ext_id.
func TestHealthRecorder(t *testing.T) {
	logger := testLogger(t)
	ctx := context.Background()

	t.Run("hook fires on search outcome", func(t *testing.T) {
		p, err := NewJSProvider("r.ext", "Rec", testExtPayload("http://127.0.0.1:0/never"), http.DefaultClient, logger)
		if err != nil {
			t.Fatal(err)
		}
		var gotID, gotErr string
		var gotHealthy bool
		var calls int
		p.SetHealthRecorder(func(id string, healthy bool, errMsg string) {
			calls++
			gotID, gotHealthy, gotErr = id, healthy, errMsg
		})
		// single() returns a result without touching the network → healthy.
		if _, err := p.SmartSearch(ctx, source.SmartSearchOptions{Media: source.Media{RomajiTitle: "x"}}); err != nil {
			t.Fatalf("SmartSearch: %v", err)
		}
		if calls != 1 || gotID != "r.ext" || !gotHealthy || gotErr != "" {
			t.Fatalf("recorder: calls=%d id=%q healthy=%v err=%q, want 1/r.ext/true/empty", calls, gotID, gotHealthy, gotErr)
		}
	})

	t.Run("manager recorder persists health", func(t *testing.T) {
		st := openStore(t)
		mgr := NewManager(st, source.NewRegistry(), http.DefaultClient, t.TempDir(), logger)

		payload := `export default new class { async single(){ return [{title:'x',link:'magnet:?xt=urn:btih:deadbeef'}] } }()`
		ext, err := st.Write().UpsertExtensionByExtID(ctx, store.UpsertExtensionByExtIDParams{
			Uuid: uuid.NewString(), ExtID: "persist.ext", Name: "Persist",
			Type: ExtTypeTorrent, Lang: "javascript", Enabled: 1, Payload: &payload,
		})
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}

		p, err := mgr.buildInstalledProvider(ext)
		if err != nil {
			t.Fatalf("buildInstalledProvider: %v", err)
		}
		if _, err := p.SmartSearch(ctx, source.SmartSearchOptions{Media: source.Media{RomajiTitle: "x"}}); err != nil {
			t.Fatalf("SmartSearch: %v", err)
		}

		row, err := st.Read().GetExtension(ctx, ext.ID)
		if err != nil {
			t.Fatalf("GetExtension: %v", err)
		}
		if row.Healthy == nil || *row.Healthy != 1 {
			t.Fatalf("Healthy = %v, want 1", row.Healthy)
		}
		if row.HealthCheckedAt == nil || *row.HealthCheckedAt == 0 {
			t.Fatalf("HealthCheckedAt = %v, want a timestamp", row.HealthCheckedAt)
		}
		if row.HealthError != nil {
			t.Fatalf("HealthError = %v, want nil on healthy", *row.HealthError)
		}
	})
}

// TestPreviewRepo covers a mixed usable/dead index plus the invalid-index 4xx path.
func TestPreviewRepo(t *testing.T) {
	logger := testLogger(t)
	ctx := context.Background()

	// Upstream the extensions' test() probes.
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/up") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(api.Close)

	// Repo server: index.json + per-extension code payloads.
	mux := http.NewServeMux()
	mux.HandleFunc("/alive.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(testExtPayload(api.URL + "/up")))
	})
	mux.HandleFunc("/dead.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(testExtPayload(api.URL + "/down")))
	})
	var repo *httptest.Server
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[
			{"id":"alive","name":"Alive","version":"1.0.0","type":"torrent","code":%q},
			{"id":"dead","name":"Dead","version":"1.0.0","type":"torrent","code":%q}
		]`, repo.URL+"/alive.js", repo.URL+"/dead.js")
	})
	mux.HandleFunc("/badindex.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json at all`))
	})
	repo = httptest.NewServer(mux)
	t.Cleanup(repo.Close)

	mgr := NewManager(openStore(t), source.NewRegistry(), repo.Client(), t.TempDir(), logger)

	t.Run("mixed usable and dead", func(t *testing.T) {
		entries, err := mgr.PreviewRepo(ctx, repo.URL+"/index.json")
		if err != nil {
			t.Fatalf("PreviewRepo: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("entries = %d, want 2", len(entries))
		}
		byID := map[string]PreviewEntry{}
		for _, e := range entries {
			byID[e.ExtID] = e
		}
		if a := byID["alive"]; !a.Usable || a.Error != "" {
			t.Errorf("alive = %+v, want usable", a)
		}
		if d := byID["dead"]; d.Usable || d.Error == "" {
			t.Errorf("dead = %+v, want unusable with error", d)
		}
	})

	t.Run("invalid index is an error", func(t *testing.T) {
		if _, err := mgr.PreviewRepo(ctx, repo.URL+"/badindex.json"); err == nil {
			t.Fatal("PreviewRepo on invalid index = nil error, want error")
		} else if !strings.Contains(err.Error(), "Repository unreachable or invalid") {
			t.Fatalf("err = %v, want 'Repository unreachable or invalid' prefix", err)
		}
	})
}
