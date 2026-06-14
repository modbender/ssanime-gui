package extension

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/source"
)

// TestEndToEndFakeExtension wires the whole adapter path: an extension that uses
// atob + navigator.onLine at load, reads its settings, fetches a canned torrent
// list, and tags one result with a settings value — proving options, settings,
// the runtime shims, and torrent marshalling all work together.
func TestEndToEndFakeExtension(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"title":    "[SubsPlease] One Piece - 1071 (1080p) [ABCD1234].mkv",
				"link":     magnet40,
				"hash":     "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
				"seeders":  321,
				"leechers": 4,
			},
		})
	}))
	defer ts.Close()

	// The base64 of the test server URL, decoded via atob at load like real
	// extensions do.
	js := `
const API = atob(btoa("` + ts.URL + `"));
export default new class {
  async single(o, settings) {
    if (!navigator.onLine) throw new Error("offline");
    const r = await fetch(API + "/search?ep=" + o.episode + "&res=" + o.resolution);
    const list = await r.json();
    // Tag the first result's name with a settings value to prove settings flow.
    if (list.length && settings && settings.tag) {
      list[0].title = "[" + settings.tag + "] " + list[0].title;
    }
    return list;
  }
}`

	resolver := &stubResolver{ids: anizip.IDs{
		AnilistID: 21, AnidbID: 69, TvdbID: 81797,
		Episodes: map[int]anizip.EpisodeIDs{1071: {AnidbEid: 9999}},
	}}
	settings := map[string]interface{}{"tag": "TAGGED"}

	p, err := NewJSProviderWithDeps("e2e", "E2E", js, ts.Client(), resolver, settings, testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProviderWithDeps: %v", err)
	}

	res, err := p.SmartSearch(context.Background(), source.SmartSearchOptions{
		Media:         source.Media{ID: 21, RomajiTitle: "One Piece", EpisodeCount: 1100, Format: "TV"},
		EpisodeNumber: 1071,
		Resolution:    "1080",
	})
	if err != nil {
		t.Fatalf("SmartSearch: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("no results")
	}
	got := res[0]
	if got.Seeders != 321 {
		t.Errorf("Seeders = %d, want 321", got.Seeders)
	}
	if got.Magnet == "" {
		t.Error("expected a magnet")
	}
	// Settings flowed into JS and tagged the name.
	if got.Name == "" || got.Name[:8] != "[TAGGED]" {
		t.Errorf("Name = %q, want it to start with [TAGGED] (settings reached JS)", got.Name)
	}
	// habari enrichment from the name.
	if got.Resolution != "1080p" {
		t.Errorf("Resolution = %q, want 1080p (parsed from name)", got.Resolution)
	}
}
