package extension

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/modbender/ssanime-gui/internal/source"
)

// loadFixture reads testdata/<name>.
func loadFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// TestStripExportDefault checks that the pre-processor converts the ES module
// export-default pattern into something goja can run.
func TestStripExportDefault(t *testing.T) {
	cases := []struct {
		in   string
		want string // substring expected in output
	}{
		{
			in:   `export default new class Foo { search() {} }`,
			want: `var __ssExt = (new class Foo { search() {} })`,
		},
		{
			// No export default — passthrough.
			in:   `var x = 1;`,
			want: `var x = 1;`,
		},
	}
	for _, c := range cases {
		got := stripExportDefault(c.in)
		if !strings.Contains(got, c.want) {
			t.Errorf("stripExportDefault(%q) = %q; want it to contain %q", c.in, got, c.want)
		}
	}
}

// TestFixtureExtensionLoads verifies:
// 1. The goja VM compiles and executes the fixture extension.
// 2. The JSProvider adapter implements source.Provider.
// 3. Search returns []*AnimeTorrent correctly marshalled.
func TestFixtureExtensionLoads(t *testing.T) {
	payload := loadFixture(t, "fixture.js")

	// Use a no-op HTTP client; the fixture extension doesn't call fetch.
	p, err := NewJSProvider("fixture-ext", "Fixture", payload, http.DefaultClient, testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProvider: %v", err)
	}

	// Verify the interface is satisfied (compile-time guarantee + runtime check).
	var _ source.Provider = p

	ctx := context.Background()
	opts := source.SearchOptions{
		Media: source.Media{RomajiTitle: "Test Anime", EpisodeCount: 12},
		Query: "Test Anime",
	}

	torrents, err := p.Search(ctx, opts)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(torrents) == 0 {
		t.Fatal("Search returned no results")
	}

	got := torrents[0]
	if got.Provider != "fixture-ext" {
		t.Errorf("Provider = %q; want %q", got.Provider, "fixture-ext")
	}
	if !strings.HasPrefix(got.Link, "magnet:") && !strings.HasPrefix(got.Magnet, "magnet:") {
		t.Errorf("expected a magnet URI, got Link=%q Magnet=%q", got.Link, got.Magnet)
	}
	if got.Seeders != 42 {
		t.Errorf("Seeders = %d; want 42", got.Seeders)
	}
	t.Logf("fixture result: name=%q magnet=%q seeders=%d", got.Name, got.Magnet, got.Seeders)
}

// TestFixtureSmartSearch verifies SmartSearch wires the episode number.
func TestFixtureSmartSearch(t *testing.T) {
	payload := loadFixture(t, "fixture.js")
	p, err := NewJSProvider("fixture-ext", "Fixture", payload, http.DefaultClient, testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProvider: %v", err)
	}

	ctx := context.Background()
	opts := source.SmartSearchOptions{
		Media:         source.Media{RomajiTitle: "Test Anime"},
		EpisodeNumber: 5,
	}
	torrents, err := p.SmartSearch(ctx, opts)
	if err != nil {
		t.Fatalf("SmartSearch: %v", err)
	}
	if len(torrents) == 0 {
		t.Fatal("SmartSearch returned no results")
	}
	// The fixture encodes the episode number in the name.
	if !strings.Contains(torrents[0].Name, "E5") {
		t.Errorf("expected E5 in name, got %q", torrents[0].Name)
	}
}

// TestFetchShim verifies the fetch() shim delivers HTTP responses to JS.
func TestFetchShim(t *testing.T) {
	// Set up a local test server that returns a JSON array.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"title":    "Shim Test Anime S01E01",
				"link":     "magnet:?xt=urn:btih:AABBCCDDAABBCCDDAABBCCDDAABBCCDD11223344",
				"hash":     "AABBCCDDAABBCCDDAABBCCDDAABBCCDD11223344",
				"seeders":  10,
				"leechers": 2,
			},
		})
	}))
	defer ts.Close()

	// A tiny inline extension that calls fetch against our test server.
	payload := `
export default new class FetchTest {
  async single({ titles }) {
    const res = await fetch("` + ts.URL + `/api");
    const data = await res.json();
    return data;
  }
  search = this.single;
}
`
	p, err := NewJSProvider("fetch-test", "FetchTest", payload, ts.Client(), testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProvider: %v", err)
	}

	ctx := context.Background()
	results, err := p.Search(ctx, source.SearchOptions{
		Media: source.Media{RomajiTitle: "Shim Test"},
	})
	if err != nil {
		t.Fatalf("Search (fetch shim): %v", err)
	}
	if len(results) == 0 {
		t.Fatal("fetch shim returned no results")
	}
	if results[0].Seeders != 10 {
		t.Errorf("Seeders = %d; want 10", results[0].Seeders)
	}
	t.Logf("fetch shim result: %+v", results[0])
}

// TestRepoIndexParse verifies the Hayase index.json fixture parses correctly.
func TestRepoIndexParse(t *testing.T) {
	data := loadFixture(t, "hayase_index.json")
	var entries []IndexEntry
	if err := json.Unmarshal([]byte(data), &entries); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	nyaa := entries[0]
	if nyaa.ID != "hayase.extension.nyaa" {
		t.Errorf("ID = %q; want hayase.extension.nyaa", nyaa.ID)
	}
	if nyaa.Type != "torrent" {
		t.Errorf("Type = %q; want torrent", nyaa.Type)
	}
	if nyaa.Code == "" {
		t.Error("Code URL must not be empty")
	}
	t.Logf("index parsed: %d entries, first=%s v%s", len(entries), nyaa.ID, nyaa.Version)
}

// TestGetSettings verifies GetSettings returns a valid Settings struct.
func TestGetSettings(t *testing.T) {
	payload := loadFixture(t, "fixture.js")
	p, err := NewJSProvider("fixture-ext", "Fixture", payload, http.DefaultClient, testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProvider: %v", err)
	}
	s := p.GetSettings()
	if s.Type != source.ProviderTypeMain {
		t.Errorf("Type = %v; want main", s.Type)
	}
}

// TestMarshalTorrentsHibikeShape checks that hibike-shaped results (name,
// magnetLink, infoHash) are also handled correctly.
func TestMarshalTorrentsHibikeShape(t *testing.T) {
	payload := `
export default new class HibikeProvider {
  async single({ titles }) {
    return [{
      name: "hibike shaped result",
      magnetLink: "magnet:?xt=urn:btih:CAFEBABECAFEBABECAFEBABECAFEBABE00001234",
      infoHash: "CAFEBABECAFEBABECAFEBABECAFEBABE00001234",
      seeders: 99,
      leechers: 1,
      resolution: "1080p",
      releaseGroup: "TestGroup",
      episodeNumber: 7,
      isBatch: false,
      isBestRelease: true,
    }];
  }
  search = this.single;
}
`
	p, err := NewJSProvider("hibike-test", "HibikeTest", payload, http.DefaultClient, testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProvider: %v", err)
	}
	results, err := p.Search(context.Background(), source.SearchOptions{
		Media: source.Media{RomajiTitle: "Test"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}
	r := results[0]
	if r.Name != "hibike shaped result" {
		t.Errorf("Name = %q", r.Name)
	}
	if r.Magnet == "" {
		t.Error("Magnet must not be empty for hibike shape")
	}
	if r.Seeders != 99 {
		t.Errorf("Seeders = %d; want 99", r.Seeders)
	}
}
