package anilist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient points a Client at a test server serving the given handler.
func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(WithHTTPClient(srv.Client()))
	c.endpoint = srv.URL
	return c
}

func serveFixture(t *testing.T, file string) http.HandlerFunc {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}

func TestGetMediaDecodesFixture(t *testing.T) {
	c := newTestClient(t, serveFixture(t, "frieren.json"))

	m, err := c.GetMedia(context.Background(), 154587)
	if err != nil {
		t.Fatalf("GetMedia: %v", err)
	}
	if m.ID != 154587 {
		t.Errorf("id = %d, want 154587", m.ID)
	}
	if m.RomajiTitle != "Sousou no Frieren" {
		t.Errorf("romaji = %q", m.RomajiTitle)
	}
	if m.EnglishTitle != "Frieren: Beyond Journey's End" {
		t.Errorf("english = %q", m.EnglishTitle)
	}
	if m.Status != "FINISHED" {
		t.Errorf("status = %q, want FINISHED", m.Status)
	}
	if m.EpisodeCount != 28 {
		t.Errorf("episodes = %d, want 28", m.EpisodeCount)
	}
	if m.Season != "FALL" || m.SeasonYear != 2023 {
		t.Errorf("season = %q %d, want FALL 2023", m.Season, m.SeasonYear)
	}
	if m.CoverImage == "" || m.BannerImage == "" {
		t.Error("expected cover and banner image URLs")
	}
	if len(m.Synonyms) == 0 {
		t.Error("expected synonyms")
	}
	if m.IDMal == nil || *m.IDMal != 52991 {
		t.Errorf("idMal = %v, want 52991", m.IDMal)
	}
}

func TestGetMediaCaches(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		serveFixture(t, "frieren.json")(w, r)
	})

	for i := 0; i < 3; i++ {
		if _, err := c.GetMedia(context.Background(), 154587); err != nil {
			t.Fatalf("GetMedia: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server hit %d times, want 1 (cache miss only once)", got)
	}
}

func TestQueryBacksOffOn429(t *testing.T) {
	var calls int32
	fixture := serveFixture(t, "frieren.json")
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fixture(w, r)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := c.GetMedia(ctx, 154587)
	if err != nil {
		t.Fatalf("GetMedia after 429: %v", err)
	}
	if m.ID != 154587 {
		t.Errorf("id = %d after retry", m.ID)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server hit %d times, want 2 (one 429 then success)", got)
	}
}

func TestSearchMediaDecodesPagedList(t *testing.T) {
	const body = `{"data":{"Page":{"media":[
		{"id":1,"title":{"romaji":"One","english":"One EN"},"episodes":12,"coverImage":{"large":"https://s4.anilist.co/a.jpg"}},
		{"id":2,"title":{"romaji":"Two"},"episodes":24}
	]}}}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	list, err := c.SearchMedia(context.Background(), "anything")
	if err != nil {
		t.Fatalf("SearchMedia: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].ID != 1 || list[0].RomajiTitle != "One" {
		t.Errorf("first = %+v", list[0])
	}
	if list[0].CoverImage != "https://s4.anilist.co/a.jpg" {
		t.Errorf("cover = %q (allowlisted host should pass through)", list[0].CoverImage)
	}
	if list[1].ID != 2 || list[1].EpisodeCount != 24 {
		t.Errorf("second = %+v", list[1])
	}
}

func TestGraphQLErrorSurfaced(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"Not Found."}],"data":{"Media":null}}`))
	})
	if _, err := c.GetMedia(context.Background(), 999999999); err == nil {
		t.Error("expected a GraphQL error to be surfaced")
	}
}
