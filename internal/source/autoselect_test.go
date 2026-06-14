package source

import (
	"testing"
)

// loadFrierenTorrents builds the release set the autoselect tests rank: two
// trusted-group 1080p copies of ep 28 (SubsPlease lower seeders, Erai-raws
// higher), a 720p copy, a 1080p batch, and an unrelated high-seeder 1080p show
// that the title filter must reject.
func loadFrierenTorrents(t *testing.T) []*AnimeTorrent {
	t.Helper()
	return []*AnimeTorrent{
		{Name: "[SubsPlease] Sousou no Frieren - 28 (1080p)", ReleaseGroup: "SubsPlease",
			Resolution: "1080p", EpisodeNumber: 28, IsBatch: false, Seeders: 1542},
		{Name: "[Erai-raws] Sousou no Frieren - 28 (1080p)", ReleaseGroup: "Erai-raws",
			Resolution: "1080p", EpisodeNumber: 28, IsBatch: false, Seeders: 2100},
		{Name: "[SubsPlease] Sousou no Frieren - 28 (720p)", ReleaseGroup: "SubsPlease",
			Resolution: "720p", EpisodeNumber: 28, IsBatch: false, Seeders: 480},
		{Name: "[SubsPlease] Sousou no Frieren (01-28) (1080p) [Batch]", ReleaseGroup: "SubsPlease",
			Resolution: "1080p", EpisodeNumber: -1, IsBatch: true, Seeders: 900},
		{Name: "[SubsPlease] Some Other Show - 05 (1080p)", ReleaseGroup: "SubsPlease",
			Resolution: "1080p", EpisodeNumber: 5, IsBatch: false, Seeders: 9000},
	}
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

// TestSelectBestDropsNowUntrustedGroups confirms groups trimmed from the trusted
// list (ASW, Judas) are dropped under RequireTrustedGroup, even with high seeders.
func TestSelectBestDropsNowUntrustedGroups(t *testing.T) {
	torrents := []*AnimeTorrent{
		{Name: "[ASW] Sousou no Frieren - 01 (1080p)", ReleaseGroup: "ASW",
			Resolution: "1080p", EpisodeNumber: 1, Seeders: 5000},
		{Name: "[Judas] Sousou no Frieren - 01 (1080p)", ReleaseGroup: "Judas",
			Resolution: "1080p", EpisodeNumber: 1, Seeders: 8000},
	}
	media := frierenMedia()
	if _, err := SelectBest(media, torrents, SelectOptions{
		Resolution: "1080", Episode: 1, RequireTrustedGroup: true,
	}); err == nil {
		t.Error("expected error: ASW/Judas are no longer trusted")
	}
}

// TestSelectBestGroupFilter checks the Group hard filter overrides trusted-rank:
// it returns the named group even when a higher-ranked trusted group is present,
// and errors when the named group has no release.
func TestSelectBestGroupFilter(t *testing.T) {
	torrents := loadFrierenTorrents(t)
	media := frierenMedia()

	best, err := SelectBest(media, torrents, SelectOptions{
		Resolution: "1080", Episode: 28, Group: "Erai-raws",
	})
	if err != nil {
		t.Fatalf("SelectBest Group=Erai-raws: %v", err)
	}
	if best.ReleaseGroup != "Erai-raws" {
		t.Errorf("best group = %q, want Erai-raws (Group filter overrides trusted-rank)", best.ReleaseGroup)
	}

	if _, err := SelectBest(media, torrents, SelectOptions{
		Resolution: "1080", Episode: 28, Group: "Nobody",
	}); err == nil {
		t.Error("expected error when the locked group has no release")
	}
}

// TestSelectBestCustomTrustedGroups confirms opts.TrustedGroups overrides the
// package default: a group the default never trusted (ASW) becomes trusted and is
// selectable, while a default-trusted group (SubsPlease) excluded from the custom
// list is dropped under RequireTrustedGroup.
func TestSelectBestCustomTrustedGroups(t *testing.T) {
	media := frierenMedia()
	torrents := []*AnimeTorrent{
		{Name: "[ASW] Sousou no Frieren - 01 (1080p)", ReleaseGroup: "ASW",
			Resolution: "1080p", EpisodeNumber: 1, Seeders: 5000},
		{Name: "[SubsPlease] Sousou no Frieren - 01 (1080p)", ReleaseGroup: "SubsPlease",
			Resolution: "1080p", EpisodeNumber: 1, Seeders: 100},
	}

	// Custom list trusts ASW only. ASW is now selectable...
	best, err := SelectBest(media, torrents, SelectOptions{
		Resolution: "1080", Episode: 1, RequireTrustedGroup: true,
		TrustedGroups: []string{"ASW"},
	})
	if err != nil {
		t.Fatalf("SelectBest custom trusted: %v", err)
	}
	if best.ReleaseGroup != "ASW" {
		t.Errorf("best group = %q, want ASW (custom trusted list)", best.ReleaseGroup)
	}

	// ...and a default-trusted group not in the custom list is rejected when it's
	// the only candidate.
	only := []*AnimeTorrent{torrents[1]}
	if _, err := SelectBest(media, only, SelectOptions{
		Resolution: "1080", Episode: 1, RequireTrustedGroup: true,
		TrustedGroups: []string{"ASW"},
	}); err == nil {
		t.Error("expected error: SubsPlease is not in the custom trusted list")
	}
}

// TestSelectBestEmptyTrustedGroupsNoFilter confirms an explicitly-empty (non-nil)
// TrustedGroups disables the trusted-only hard filter: SelectBest falls back to
// best-available and returns a release instead of erroring.
func TestSelectBestEmptyTrustedGroupsNoFilter(t *testing.T) {
	media := frierenMedia()
	torrents := []*AnimeTorrent{
		{Name: "[Nobody] Sousou no Frieren - 01 (1080p)", ReleaseGroup: "Nobody",
			Resolution: "1080p", EpisodeNumber: 1, Seeders: 999},
	}

	best, err := SelectBest(media, torrents, SelectOptions{
		Resolution: "1080", Episode: 1, RequireTrustedGroup: true,
		TrustedGroups: []string{}, // explicit empty = no trust filter
	})
	if err != nil {
		t.Fatalf("SelectBest empty trusted: %v", err)
	}
	if best.ReleaseGroup != "Nobody" {
		t.Errorf("best group = %q, want Nobody (no trust filter, best-available)", best.ReleaseGroup)
	}
}

// TestSelectBestNilTrustedGroupsUsesDefault confirms a nil TrustedGroups still
// uses the package default (existing callers/tests behaviour is unchanged): an
// untrusted-by-default group is dropped under RequireTrustedGroup.
func TestSelectBestNilTrustedGroupsUsesDefault(t *testing.T) {
	media := frierenMedia()
	torrents := []*AnimeTorrent{
		{Name: "[Nobody] Sousou no Frieren - 01 (1080p)", ReleaseGroup: "Nobody",
			Resolution: "1080p", EpisodeNumber: 1, Seeders: 999},
	}
	if _, err := SelectBest(media, torrents, SelectOptions{
		Resolution: "1080", Episode: 1, RequireTrustedGroup: true,
		TrustedGroups: nil, // nil → package default applies
	}); err == nil {
		t.Error("expected error: nil TrustedGroups falls back to the default allowlist")
	}
}
