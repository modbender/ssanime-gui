package source

import (
	"context"
	"testing"
)

func loadFrierenTorrents(t *testing.T) []*AnimeTorrent {
	t.Helper()
	srv := fixtureServer(t, "nyaa_frieren.xml")
	n := NewNyaa(srv.Client())
	got, err := n.fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	return got
}

func frierenMedia() Media {
	eng := "Frieren: Beyond Journey's End"
	return Media{
		ID:           154587,
		RomajiTitle:  "Sousou no Frieren",
		EnglishTitle: &eng,
		Status:       "FINISHED",
		Format:       "TV",
		EpisodeCount: 28,
	}
}

func TestSelectBestPrefersTrustedGroupAtResolution(t *testing.T) {
	torrents := loadFrierenTorrents(t)
	media := frierenMedia()

	best, err := SelectBest(media, torrents, SelectOptions{
		Resolution: "1080",
		Episode:    28,
	})
	if err != nil {
		t.Fatalf("SelectBest: %v", err)
	}
	// Both SubsPlease and Erai-raws are trusted; SubsPlease ranks first in the
	// trusted list, so it wins despite Erai-raws having more seeders.
	if best.ReleaseGroup != "SubsPlease" {
		t.Errorf("best group = %q, want SubsPlease (trusted-rank wins over seeders)", best.ReleaseGroup)
	}
	if best.Resolution != "1080p" {
		t.Errorf("best resolution = %q, want 1080p", best.Resolution)
	}
	if best.EpisodeNumber != 28 {
		t.Errorf("best episode = %d, want 28", best.EpisodeNumber)
	}
}

func TestSelectBestRejectsUnrelatedHighSeeders(t *testing.T) {
	torrents := loadFrierenTorrents(t)
	media := frierenMedia()

	// The unrelated show has 9000 seeders but a non-matching title and is 1080p;
	// the title filter must keep it out.
	best, err := SelectBest(media, torrents, SelectOptions{Resolution: "1080"})
	if err != nil {
		t.Fatalf("SelectBest: %v", err)
	}
	if !titleMatches(best.Name, media.Titles()) {
		t.Errorf("selected unrelated release: %q", best.Name)
	}
}

func TestSelectBestBatch(t *testing.T) {
	torrents := loadFrierenTorrents(t)
	media := frierenMedia()

	best, err := SelectBest(media, torrents, SelectOptions{
		Resolution:  "1080",
		PreferBatch: true,
	})
	if err != nil {
		t.Fatalf("SelectBest batch: %v", err)
	}
	if !best.IsBatch {
		t.Errorf("expected a batch release, got %q", best.Name)
	}
}

func TestSelectBestNoMatch(t *testing.T) {
	torrents := loadFrierenTorrents(t)
	media := frierenMedia()

	if _, err := SelectBest(media, torrents, SelectOptions{Resolution: "2160"}); err == nil {
		t.Error("expected error when no release matches the requested resolution")
	}
}

func TestSelectBestRequireTrustedGroup(t *testing.T) {
	torrents := []*AnimeTorrent{
		{Name: "[Nobody] Sousou no Frieren - 01 (1080p)", ReleaseGroup: "Nobody",
			Resolution: "1080p", EpisodeNumber: 1, Seeders: 999},
	}
	media := frierenMedia()
	if _, err := SelectBest(media, torrents, SelectOptions{
		Resolution: "1080", Episode: 1, RequireTrustedGroup: true,
	}); err == nil {
		t.Error("expected error when only untrusted groups are present and trusted is required")
	}
}
