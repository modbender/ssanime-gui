package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// fixtureServer serves the recorded nyaa RSS fixture so the parse path is
// exercised end-to-end (gofeed + the nyaa namespace extension + habari enrich)
// without touching the live network.
func fixtureServer(t *testing.T, file string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("read fixture %s: %v", file, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestNyaaFetchParsesFixture(t *testing.T) {
	srv := fixtureServer(t, "nyaa_frieren.xml")
	n := NewNyaa(srv.Client())

	got, err := n.fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("want 5 items, got %d", len(got))
	}

	// First item: SubsPlease 1080p ep 28.
	first := got[0]
	if first.ReleaseGroup != "SubsPlease" {
		t.Errorf("release group = %q, want SubsPlease", first.ReleaseGroup)
	}
	if first.Resolution != "1080p" {
		t.Errorf("resolution = %q, want 1080p", first.Resolution)
	}
	if first.EpisodeNumber != 28 {
		t.Errorf("episode = %d, want 28", first.EpisodeNumber)
	}
	if first.Seeders != 1542 {
		t.Errorf("seeders = %d, want 1542", first.Seeders)
	}
	if first.InfoHash == "" {
		t.Error("info hash should be parsed from the nyaa extension")
	}
	gib := float64(int64(1) << 30)
	wantSize := int64(1.4 * gib)
	if first.Size != wantSize {
		t.Errorf("size = %d, want %d", first.Size, wantSize)
	}
	if first.Magnet == "" {
		t.Error("magnet should be built from the info hash")
	}

	// The batch item must be flagged as a batch with no single episode.
	var batch *AnimeTorrent
	for _, item := range got {
		if item.IsBatch {
			batch = item
			break
		}
	}
	if batch == nil {
		t.Fatal("expected a batch release in the fixture")
	}
	if batch.EpisodeNumber != -1 {
		t.Errorf("batch episode = %d, want -1", batch.EpisodeNumber)
	}
}

func TestNyaaSmartSearchFiltersByMedia(t *testing.T) {
	srv := fixtureServer(t, "nyaa_frieren.xml")
	n := NewNyaa(srv.Client())
	// Override the search URL to point at the fixture by fetching directly: the
	// SmartSearch filter logic is what we test, fed the parsed fixture.
	results, err := n.fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	eng := "Frieren: Beyond Journey's End"
	media := Media{RomajiTitle: "Sousou no Frieren", EnglishTitle: &eng}
	filtered := filterSmart(results, SmartSearchOptions{
		Media:      media,
		Resolution: "1080",
	})
	// SubsPlease 1080, Erai-raws 1080, and the 1080p batch match; 720p and the
	// unrelated show are dropped.
	if len(filtered) != 3 {
		names := make([]string, len(filtered))
		for i, f := range filtered {
			names[i] = f.Name
		}
		t.Fatalf("want 3 filtered, got %d: %v", len(filtered), names)
	}
	for _, f := range filtered {
		if f.Resolution != "1080p" {
			t.Errorf("filtered item not 1080p: %q", f.Name)
		}
		if !titleMatches(f.Name, media.Titles()) {
			t.Errorf("filtered item does not match media: %q", f.Name)
		}
	}
}
