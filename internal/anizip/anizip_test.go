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

// logoBody mirrors the real ani.zip /mappings "images" array: a flat list of
// artwork entries discriminated by coverType, with a transparent Clearlogo on
// the allowlisted TVDB host.
const logoBody = `{
  "images": [
    {"coverType": "Banner", "url": "https://artworks.thetvdb.com/banners/v4/series/81797/banners/x.jpg"},
    {"coverType": "Clearlogo", "url": "https://artworks.thetvdb.com/banners/v4/series/81797/clearlogo/abc.png"},
    {"coverType": "Poster", "url": "https://artworks.thetvdb.com/banners/v4/series/81797/posters/y.jpg"}
  ],
  "episodes": {}
}`

func TestGetClearLogoExtractsClearlogo(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(logoBody))
	})
	logo, err := c.GetClearLogo(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetClearLogo: %v", err)
	}
	want := "https://artworks.thetvdb.com/banners/v4/series/81797/clearlogo/abc.png"
	if logo != want {
		t.Errorf("logo = %q, want %q", logo, want)
	}
}

func TestGetClearLogoNoLogoIsEmpty(t *testing.T) {
	// Same payload minus the Clearlogo entry: must yield "" (no error).
	body := `{"images":[{"coverType":"Banner","url":"https://artworks.thetvdb.com/banners/v4/series/1/banners/x.jpg"}],"episodes":{}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	logo, err := c.GetClearLogo(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetClearLogo: %v", err)
	}
	if logo != "" {
		t.Errorf("logo = %q, want empty when no Clearlogo present", logo)
	}
}

func TestGetClearLogoNonAllowlistedHostDropped(t *testing.T) {
	// A Clearlogo served from a non-allowlisted host is dropped by safeImageURL.
	body := `{"images":[{"coverType":"Clearlogo","url":"https://evil.example.com/logo.png"}],"episodes":{}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	logo, err := c.GetClearLogo(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetClearLogo: %v", err)
	}
	if logo != "" {
		t.Errorf("logo = %q, want empty for non-allowlisted host", logo)
	}
}

func TestGetClearLogoNotFoundIsEmpty(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	logo, err := c.GetClearLogo(context.Background(), 999999)
	if err != nil {
		t.Fatalf("404 should not be an error: %v", err)
	}
	if logo != "" {
		t.Errorf("logo = %q, want empty for unmapped id", logo)
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
