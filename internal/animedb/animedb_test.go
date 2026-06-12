package animedb

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// quietLogger discards animedb's INFO load chatter so test output stays focused.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeDataset is a minimal manami-shaped payload exercising the load/index path
// without any network or zstd. It includes: a normal AniList-sourced TV entry
// with synonyms, an ONGOING entry, a MAL-only-picture entry, and one entry with
// NO AniList source (must be skipped). The wrapper carries unrelated leading
// keys ($schema, license, lastUpdate) so seekToDataArray's skip logic is tested.
const fakeDataset = `{
  "$schema": "https://example/schema.json",
  "license": {"name": "ODbL", "url": "https://example/license"},
  "lastUpdate": "2026-04-02",
  "scoreRange": {"minInclusive": 1.0, "maxInclusive": 10.0},
  "data": [
    {
      "sources": ["https://myanimelist.net/anime/1535", "https://anilist.co/anime/1535"],
      "title": "Death Note",
      "type": "TV",
      "episodes": 37,
      "status": "FINISHED",
      "animeSeason": {"season": "FALL", "year": 2006},
      "picture": "https://cdn.myanimelist.net/images/anime/1079/138100.jpg",
      "synonyms": ["Notatnik smierci", "デスノート"]
    },
    {
      "sources": ["https://anilist.co/anime/154587"],
      "title": "Frieren: Beyond Journey's End",
      "type": "TV",
      "episodes": 28,
      "status": "ONGOING",
      "animeSeason": {"season": "FALL", "year": 2023},
      "picture": "https://s4.anilist.co/file/anilistcdn/media/anime/cover/154587.jpg",
      "synonyms": ["Sousou no Frieren", "葬送のフリーレン"]
    },
    {
      "sources": ["https://anilist.co/anime/999", "https://kitsu.app/anime/42"],
      "title": "Mal Picture Movie",
      "type": "MOVIE",
      "episodes": 1,
      "status": "UPCOMING",
      "picture": "https://cdn.myanimelist.net/images/anime/9/99999.jpg",
      "synonyms": []
    },
    {
      "sources": ["https://myanimelist.net/anime/777", "https://anidb.net/anime/777"],
      "title": "No AniList Here",
      "type": "OVA",
      "episodes": 2,
      "status": "FINISHED",
      "synonyms": ["Orphan Show"]
    },
    {
      "sources": ["https://anilist.co/anime/22"],
      "title": "Unknown Bits",
      "type": "UNKNOWN",
      "episodes": 0,
      "status": "UNKNOWN",
      "animeSeason": {"season": "UNDEFINED"},
      "synonyms": []
    }
  ]
}`

func loadFake(t *testing.T) *DB {
	t.Helper()
	d := New(t.TempDir(), WithLogger(quietLogger()))
	if err := d.loadFromReader(context.Background(), strings.NewReader(fakeDataset)); err != nil {
		t.Fatalf("loadFromReader: %v", err)
	}
	if !d.Ready() {
		t.Fatal("Ready() false after load")
	}
	return d
}

func TestLoadSkipsEntriesWithoutAniListSource(t *testing.T) {
	d := loadFake(t)
	// 5 entries in, 1 has no AniList source → 4 indexed.
	if got := len(d.index); got != 4 {
		t.Fatalf("indexed %d entries, want 4", got)
	}
	if r := d.Search("No AniList Here", 10); len(r) != 0 {
		t.Fatalf("non-AniList entry should be unsearchable, got %+v", r)
	}
}

func TestSearchByTitle(t *testing.T) {
	d := loadFake(t)
	res := d.Search("death note", 25)
	if len(res) == 0 {
		t.Fatal("expected a hit for 'death note'")
	}
	if res[0].AniListID != 1535 {
		t.Fatalf("top hit AniListID = %d, want 1535", res[0].AniListID)
	}
	if res[0].Title != "Death Note" {
		t.Fatalf("title = %q", res[0].Title)
	}
}

func TestSearchBySynonym(t *testing.T) {
	d := loadFake(t)
	res := d.Search("Sousou no Frieren", 25)
	if len(res) == 0 {
		t.Fatal("expected a synonym hit for Frieren")
	}
	if res[0].AniListID != 154587 {
		t.Fatalf("synonym top hit = %d, want 154587", res[0].AniListID)
	}
}

func TestSearchExactTitleOutranksSubstring(t *testing.T) {
	d := loadFake(t)
	// "Death Note" is an exact title; nothing else should outrank it.
	res := d.Search("Death Note", 25)
	if res[0].AniListID != 1535 {
		t.Fatalf("exact title not ranked first: %+v", res[0])
	}
}

func TestStatusTypeSeasonMapping(t *testing.T) {
	d := loadFake(t)

	byID := func(id int) Result {
		for _, r := range d.index {
			if r.AniListID == id {
				return r.Result
			}
		}
		t.Fatalf("id %d not indexed", id)
		return Result{}
	}

	death := byID(1535)
	if death.Status != "FINISHED" || death.Type != "TV" || death.Season != "FALL" || death.Year != 2006 {
		t.Fatalf("death note mapping wrong: %+v", death)
	}

	frieren := byID(154587)
	if frieren.Status != "RELEASING" { // ONGOING → RELEASING
		t.Fatalf("ongoing should map to RELEASING, got %q", frieren.Status)
	}

	movie := byID(999)
	if movie.Status != "NOT_YET_RELEASED" || movie.Type != "MOVIE" { // UPCOMING → NOT_YET_RELEASED
		t.Fatalf("upcoming/movie mapping wrong: %+v", movie)
	}
	// MAL picture is passed through verbatim (the server, not animedb, applies
	// the CSP host filter).
	if !strings.Contains(movie.Picture, "cdn.myanimelist.net") {
		t.Fatalf("MAL picture not preserved: %q", movie.Picture)
	}

	unk := byID(22)
	if unk.Status != "" || unk.Type != "" || unk.Season != "" {
		t.Fatalf("UNKNOWN/UNDEFINED should map to empty, got %+v", unk)
	}
}

func TestSearchLimitAndEmptyQuery(t *testing.T) {
	d := loadFake(t)
	if r := d.Search("", 25); r != nil {
		t.Fatalf("empty query should return nil, got %+v", r)
	}
	if r := d.Search("a", 0); r != nil {
		t.Fatalf("zero limit should return nil, got %+v", r)
	}
	// A broad substring ("e" matches many) capped to 2.
	if r := d.Search("note", 1); len(r) > 1 {
		t.Fatalf("limit not enforced: got %d", len(r))
	}
}

// TestConcurrentReloadIsSafe runs Search concurrently with repeated index swaps
// to surface data races under -race; the swap happens under the write lock.
func TestConcurrentReloadIsSafe(t *testing.T) {
	d := New(t.TempDir(), WithLogger(quietLogger()))
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if err := d.loadFromReader(context.Background(), strings.NewReader(fakeDataset)); err != nil {
				t.Errorf("reload: %v", err)
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = d.Search("frieren", 25)
			_ = d.Ready()
		}
	}()

	wg.Wait()
}

// TestSeekToDataArrayRejectsNonObject guards the streaming entry point against a
// malformed top-level value.
func TestSeekToDataArrayRejectsNonObject(t *testing.T) {
	d := New(t.TempDir(), WithLogger(quietLogger()))
	err := d.loadFromReader(context.Background(), strings.NewReader(`[1,2,3]`))
	if err == nil {
		t.Fatal("expected error on non-object top level")
	}
}
