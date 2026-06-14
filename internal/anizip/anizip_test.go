package anizip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fixtureBody mirrors the real ani.zip /mappings shape: a top-level "episodes"
// object keyed by episode-number string, each entry carrying a language-map
// title, both airDate/airdate and runtime/length variants, overview+summary, and
// a TVDB artwork image.
const fixtureBody = `{
  "titles": {"en": "One Piece", "x-jat": "OP TV"},
  "episodes": {
    "1": {
      "episodeNumber": 1,
      "episode": "1",
      "title": {"en": "Episode One EN", "x-jat": "Episode One Romaji", "ja": "JP"},
      "airDate": "1999-10-20",
      "airdate": "1999-10-20",
      "runtime": 25,
      "length": 24,
      "overview": "The overview.",
      "summary": "The longer summary.",
      "image": "https://artworks.thetvdb.com/banners/v4/episode/361887/screencap/x.jpg"
    },
    "2": {
      "episodeNumber": 2,
      "title": {"x-jat": "Only Romaji"},
      "airdate": "1999-11-17",
      "length": 24,
      "summary": "Only summary.",
      "image": "https://evil.example.com/x.jpg"
    },
    "S1": {
      "title": {"en": "A special with no number"}
    }
  }
}`

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(WithHTTPClient(srv.Client()))
	c.endpoint = srv.URL
	return c
}

func TestGetEpisodesParsesAndSorts(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixtureBody))
	})

	eps, err := c.GetEpisodes(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetEpisodes: %v", err)
	}
	// The "S1" special has no usable integer number and is dropped.
	if len(eps) != 2 {
		t.Fatalf("got %d episodes, want 2: %+v", len(eps), eps)
	}
	if eps[0].Number != 1 || eps[1].Number != 2 {
		t.Fatalf("episodes not sorted by number: %+v", eps)
	}

	e1 := eps[0]
	if e1.Title != "Episode One EN" {
		t.Errorf("title = %q, want English preferred", e1.Title)
	}
	if e1.AirDate != "1999-10-20" {
		t.Errorf("air date = %q", e1.AirDate)
	}
	if e1.RuntimeMin != 25 {
		t.Errorf("runtime = %d, want 25 (runtime preferred over length)", e1.RuntimeMin)
	}
	if e1.Overview != "The overview." {
		t.Errorf("overview = %q, want overview preferred over summary", e1.Overview)
	}
	if e1.Thumbnail == "" {
		t.Error("expected allowlisted TVDB thumbnail to survive")
	}

	e2 := eps[1]
	if e2.Title != "Only Romaji" {
		t.Errorf("title = %q, want x-jat fallback", e2.Title)
	}
	if e2.AirDate != "1999-11-17" {
		t.Errorf("air date = %q, want lowercase airdate fallback", e2.AirDate)
	}
	if e2.RuntimeMin != 24 {
		t.Errorf("runtime = %d, want 24 (length fallback)", e2.RuntimeMin)
	}
	if e2.Overview != "Only summary." {
		t.Errorf("overview = %q, want summary fallback", e2.Overview)
	}
	if e2.Thumbnail != "" {
		t.Errorf("thumbnail = %q, want non-allowlisted host dropped", e2.Thumbnail)
	}
}

// heroBody mirrors the real ani.zip /mappings "images" array: a flat list of
// artwork entries discriminated by coverType. It includes Banner entries (which
// must be excluded), a duplicate Fanart, and a non-allowlisted Fanart so a test
// can assert the Banner exclusion, dedupe, and host filter.
const heroBody = `{
  "images": [
    {"coverType": "Banner", "url": "https://artworks.thetvdb.com/banners/v4/series/81797/banners/b1.jpg"},
    {"coverType": "Fanart", "url": "https://artworks.thetvdb.com/banners/v4/series/81797/backgrounds/f1.jpg"},
    {"coverType": "Clearlogo", "url": "https://artworks.thetvdb.com/banners/v4/series/81797/clearlogo/abc.png"},
    {"coverType": "Poster", "url": "https://artworks.thetvdb.com/banners/v4/series/81797/posters/y.jpg"},
    {"coverType": "Fanart", "url": "https://evil.example.com/f-bad.jpg"},
    {"coverType": "Fanart", "url": "https://artworks.thetvdb.com/banners/v4/series/81797/backgrounds/f2.jpg"},
    {"coverType": "Fanart", "url": "https://artworks.thetvdb.com/banners/v4/series/81797/backgrounds/f1.jpg"}
  ],
  "episodes": {}
}`

func TestGetHeroArtExtractsLogoAndWide(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(heroBody))
	})
	logo, wide, err := c.GetHeroArt(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetHeroArt: %v", err)
	}
	wantLogo := "https://artworks.thetvdb.com/banners/v4/series/81797/clearlogo/abc.png"
	if logo != wantLogo {
		t.Errorf("logo = %q, want %q", logo, wantLogo)
	}
	// Only Fanart, in source order, non-allowlisted host dropped. Banner (the
	// low-res graphical strip), Poster, and Clearlogo are excluded.
	want := []string{
		"https://artworks.thetvdb.com/banners/v4/series/81797/backgrounds/f1.jpg",
		"https://artworks.thetvdb.com/banners/v4/series/81797/backgrounds/f2.jpg",
	}
	if len(wide) != len(want) {
		t.Fatalf("wide = %v, want %v", wide, want)
	}
	for i := range want {
		if wide[i] != want[i] {
			t.Errorf("wide[%d] = %q, want %q (full=%v)", i, wide[i], want[i], wide)
		}
	}
}

func TestGetHeroArtNoLogoIsEmpty(t *testing.T) {
	// Payload with only wide art (no Clearlogo): logo "" with no error, wide kept.
	body := `{"images":[{"coverType":"Fanart","url":"https://artworks.thetvdb.com/banners/v4/series/1/backgrounds/x.jpg"}],"episodes":{}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	logo, wide, err := c.GetHeroArt(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetHeroArt: %v", err)
	}
	if logo != "" {
		t.Errorf("logo = %q, want empty when no Clearlogo present", logo)
	}
	if len(wide) != 1 {
		t.Errorf("wide = %v, want the single allowlisted Fanart", wide)
	}
}

func TestGetHeroArtNonAllowlistedHostDropped(t *testing.T) {
	// A Clearlogo served from a non-allowlisted host is dropped by safeImageURL.
	body := `{"images":[{"coverType":"Clearlogo","url":"https://evil.example.com/logo.png"}],"episodes":{}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	logo, wide, err := c.GetHeroArt(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetHeroArt: %v", err)
	}
	if logo != "" {
		t.Errorf("logo = %q, want empty for non-allowlisted host", logo)
	}
	if len(wide) != 0 {
		t.Errorf("wide = %v, want empty", wide)
	}
}

func TestGetHeroArtNotFoundIsEmpty(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	logo, wide, err := c.GetHeroArt(context.Background(), 999999)
	if err != nil {
		t.Fatalf("404 should not be an error: %v", err)
	}
	if logo != "" || wide != nil {
		t.Errorf("got logo=%q wide=%v, want empty for unmapped id", logo, wide)
	}
}

func TestGetEpisodesNotFoundIsEmpty(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	eps, err := c.GetEpisodes(context.Background(), 999999)
	if err != nil {
		t.Fatalf("404 should not be an error: %v", err)
	}
	if eps != nil {
		t.Errorf("expected nil episodes for an unmapped id, got %+v", eps)
	}
}

func TestGetEpisodesServerErrorPropagates(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := c.GetEpisodes(context.Background(), 21); err == nil {
		t.Fatal("expected an error on HTTP 500")
	}
}

// idsFixture is a full mappings payload: every anime id present, themoviedb_id
// as a STRING (the common ani.zip quirk), and two episodes with anidbEid/tvdbId.
const idsFixture = `{
  "mappings": {
    "anidb_id": 69,
    "mal_id": 21,
    "thetvdb_id": 81797,
    "themoviedb_id": "37854",
    "anilist_id": 21,
    "kitsu_id": 12,
    "anisearch_id": 2734
  },
  "episodes": {
    "1": {"episodeNumber": 1, "anidbEid": 440, "tvdbId": 5505123},
    "2": {"episodeNumber": 2, "anidbEid": 441, "tvdbId": 5505124}
  }
}`

func TestGetIDsFullMappings(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(idsFixture))
	})
	ids, err := c.GetIDs(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetIDs: %v", err)
	}
	if ids.AnilistID != 21 || ids.AnidbID != 69 || ids.MalID != 21 ||
		ids.TvdbID != 81797 || ids.TmdbID != 37854 || ids.KitsuID != 12 || ids.AnisearchID != 2734 {
		t.Fatalf("anime ids wrong: %+v", ids)
	}
	if len(ids.Episodes) != 2 {
		t.Fatalf("episodes = %d, want 2: %+v", len(ids.Episodes), ids.Episodes)
	}
	if ep := ids.Episodes[1]; ep.AnidbEid != 440 || ep.TvdbEid != 5505123 {
		t.Errorf("episode 1 ids = %+v, want {440, 5505123}", ep)
	}
	if ep := ids.Episodes[2]; ep.AnidbEid != 441 || ep.TvdbEid != 5505124 {
		t.Errorf("episode 2 ids = %+v, want {441, 5505124}", ep)
	}
}

func TestGetIDsTmdbAsNumber(t *testing.T) {
	body := `{"mappings":{"anilist_id":21,"themoviedb_id":37854},"episodes":{}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	ids, err := c.GetIDs(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetIDs: %v", err)
	}
	if ids.TmdbID != 37854 {
		t.Errorf("TmdbID (numeric) = %d, want 37854", ids.TmdbID)
	}
}

func TestGetIDsMissingAnimeIDsAreZero(t *testing.T) {
	// Only anilist_id present; the rest must default to 0 without erroring.
	body := `{"mappings":{"anilist_id":21},"episodes":{}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	ids, err := c.GetIDs(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetIDs: %v", err)
	}
	if ids.AnilistID != 21 {
		t.Errorf("AnilistID = %d, want 21", ids.AnilistID)
	}
	if ids.AnidbID != 0 || ids.TvdbID != 0 || ids.TmdbID != 0 || ids.MalID != 0 {
		t.Errorf("missing ids should be 0: %+v", ids)
	}
}

func TestGetIDsNoMappingsObject(t *testing.T) {
	// Mappings object entirely absent: anime ids 0, AnilistID backfilled from arg.
	body := `{"episodes":{"1":{"episodeNumber":1,"anidbEid":440}}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	ids, err := c.GetIDs(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetIDs: %v", err)
	}
	if ids.AnilistID != 21 {
		t.Errorf("AnilistID backfill = %d, want 21", ids.AnilistID)
	}
	if ids.AnidbID != 0 {
		t.Errorf("AnidbID = %d, want 0 when mappings absent", ids.AnidbID)
	}
	if ep := ids.Episodes[1]; ep.AnidbEid != 440 {
		t.Errorf("episode 1 anidbEid = %d, want 440", ep.AnidbEid)
	}
}

func TestGetIDsNoEpisodeMap(t *testing.T) {
	body := `{"mappings":{"anilist_id":21,"anidb_id":69}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	ids, err := c.GetIDs(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetIDs: %v", err)
	}
	if len(ids.Episodes) != 0 {
		t.Errorf("episodes = %d, want 0", len(ids.Episodes))
	}
	if ids.AnidbID != 69 {
		t.Errorf("AnidbID = %d, want 69", ids.AnidbID)
	}
}

func TestGetIDsEpisodeMissingFields(t *testing.T) {
	// Episode 1 has no tvdbId; episode 2 has no anidbEid — each missing field is 0.
	body := `{"mappings":{"anilist_id":21},"episodes":{
	  "1":{"episodeNumber":1,"anidbEid":440},
	  "2":{"episodeNumber":2,"tvdbId":999}
	}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	ids, err := c.GetIDs(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetIDs: %v", err)
	}
	if ep := ids.Episodes[1]; ep.AnidbEid != 440 || ep.TvdbEid != 0 {
		t.Errorf("ep1 = %+v, want {440, 0}", ep)
	}
	if ep := ids.Episodes[2]; ep.AnidbEid != 0 || ep.TvdbEid != 999 {
		t.Errorf("ep2 = %+v, want {0, 999}", ep)
	}
}

func TestGetIDsEpisodeKeyFallback(t *testing.T) {
	// Episode entry without episodeNumber: number parsed from the map key.
	body := `{"mappings":{"anilist_id":21},"episodes":{"3":{"anidbEid":442,"tvdbId":7}}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	ids, err := c.GetIDs(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetIDs: %v", err)
	}
	if ep, ok := ids.Episodes[3]; !ok || ep.AnidbEid != 442 {
		t.Errorf("episode keyed by string '3' should map to 3: %+v ok=%v", ep, ok)
	}
}

func TestGetIDsNotFoundIsZero(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ids, err := c.GetIDs(context.Background(), 999999)
	if err != nil {
		t.Fatalf("404 should not error: %v", err)
	}
	if ids.AnilistID != 0 || ids.AnidbID != 0 || len(ids.Episodes) != 0 {
		t.Errorf("404 should yield zero IDs, got %+v", ids)
	}
}

func TestGetIDsServerErrorPropagates(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := c.GetIDs(context.Background(), 21); err == nil {
		t.Fatal("expected an error on HTTP 500")
	}
}

func TestGetIDsMalformedJSONPropagates(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	})
	if _, err := c.GetIDs(context.Background(), 21); err == nil {
		t.Fatal("expected a decode error on malformed JSON")
	}
}

// A garbage themoviedb_id string must not fail the whole decode — it defaults to 0.
func TestGetIDsTmdbGarbageStringDefaultsZero(t *testing.T) {
	body := `{"mappings":{"anilist_id":21,"themoviedb_id":"not-a-number"},"episodes":{}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	ids, err := c.GetIDs(context.Background(), 21)
	if err != nil {
		t.Fatalf("garbage tmdb id should not fail decode: %v", err)
	}
	if ids.TmdbID != 0 {
		t.Errorf("TmdbID = %d, want 0 for non-numeric string", ids.TmdbID)
	}
	if ids.AnilistID != 21 {
		t.Errorf("AnilistID = %d, want 21 (decode survived)", ids.AnilistID)
	}
}
