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
