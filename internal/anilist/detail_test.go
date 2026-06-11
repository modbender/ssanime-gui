package anilist

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const detailBody = `{
  "data": {
    "Media": {
      "id": 21,
      "description": "A pirate <b>adventure</b>.<br><br>Second paragraph &amp; more.",
      "genres": ["Action", "Adventure"],
      "averageScore": 88,
      "source": "MANGA",
      "season": "FALL",
      "seasonYear": 1999,
      "duration": 24,
      "episodes": 0,
      "format": "TV",
      "status": "RELEASING",
      "title": {"romaji": "One Piece", "english": "One Piece", "native": "ONE PIECE"},
      "coverImage": {"large": "https://s4.anilist.co/cover.jpg", "extraLarge": "https://s4.anilist.co/coverxl.jpg", "color": "#e49335"},
      "bannerImage": "https://s4.anilist.co/banner.jpg",
      "studios": {"nodes": [{"name": "Toei Animation", "isAnimationStudio": true}]},
      "trailer": {"id": "abc123", "site": "youtube", "thumbnail": "https://i.ytimg.com/vi/abc123/hq.jpg"},
      "streamingEpisodes": [
        {"title": "Episode 1", "thumbnail": "https://img1.ak.crunchyroll.com/e1.jpg"},
        {"title": "Episode 2", "thumbnail": "https://evil.example.com/e2.jpg"}
      ],
      "nextAiringEpisode": {"episode": 1100, "airingAt": 1700000000},
      "relations": {
        "edges": [
          {"relationType": "PREQUEL", "node": {"id": 99, "format": "MOVIE", "status": "FINISHED", "title": {"romaji": "Rel R", "english": "Rel E"}, "coverImage": {"large": "https://s4.anilist.co/rel.jpg", "color": "#fff"}}},
          {"relationType": "ADAPTATION", "node": null}
        ]
      },
      "recommendations": {
        "nodes": [
          {"mediaRecommendation": {"id": 100, "format": "TV", "status": "RELEASING", "title": {"romaji": "Rec R", "english": "Rec E"}, "coverImage": {"large": "https://s4.anilist.co/rec.jpg", "color": "#000"}}},
          {"mediaRecommendation": null}
        ]
      }
    }
  }
}`

func TestGetDetailDecodes(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(detailBody))
	})

	d, err := c.GetDetail(context.Background(), 21)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	if d.ID != 21 {
		t.Errorf("id = %d", d.ID)
	}
	// HTML stripped, entities unescaped, <br><br> collapsed to a paragraph break.
	if strings.Contains(d.Description, "<") || strings.Contains(d.Description, "&amp;") {
		t.Errorf("description not stripped: %q", d.Description)
	}
	if !strings.Contains(d.Description, "adventure") || !strings.Contains(d.Description, "& more") {
		t.Errorf("description content lost: %q", d.Description)
	}
	if len(d.Genres) != 2 || d.AverageScore != 88 || d.Source != "MANGA" {
		t.Errorf("scalar fields wrong: %+v", d)
	}
	if d.Studio != "Toei Animation" {
		t.Errorf("studio = %q", d.Studio)
	}
	if d.Duration != 24 || d.Season != "FALL" || d.SeasonYear != 1999 {
		t.Errorf("season/duration wrong: %+v", d)
	}
	if d.Trailer == nil || d.Trailer.Site != "youtube" || d.Trailer.VideoID != "abc123" {
		t.Fatalf("trailer = %+v", d.Trailer)
	}
	if d.Trailer.Thumbnail == "" {
		t.Error("youtube trailer thumbnail should survive host pinning")
	}
	if d.NextAiring == nil || d.NextAiring.Episode != 1100 {
		t.Fatalf("next airing = %+v", d.NextAiring)
	}
	if len(d.StreamingEpisodes) != 2 {
		t.Fatalf("streaming episodes = %d", len(d.StreamingEpisodes))
	}
	if d.StreamingEpisodes[0].Thumbnail == "" {
		t.Error("crunchyroll streaming thumbnail should survive")
	}
	if d.StreamingEpisodes[1].Thumbnail != "" {
		t.Error("non-allowlisted streaming thumbnail should be dropped")
	}
	// Null nodes are skipped.
	if len(d.Relations) != 1 || d.Relations[0].RelationType != "PREQUEL" || d.Relations[0].AnilistID != 99 {
		t.Fatalf("relations = %+v", d.Relations)
	}
	if len(d.Recommendations) != 1 || d.Recommendations[0].AnilistID != 100 {
		t.Fatalf("recommendations = %+v", d.Recommendations)
	}
}

func TestGetDetailSurfacesGraphQLError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"Too Many Requests."}]}`))
	})
	if _, err := c.GetDetail(context.Background(), 21); err == nil {
		t.Fatal("expected a GraphQL error to surface")
	}
}

func TestStripHTML(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"plain text", "plain text"},
		{"<i>tag</i> only", "tag only"},
		{"line<br>break", "line\nbreak"},
		{"a&amp;b", "a&b"},
		{"x<br><br><br>y", "x\n\ny"},
	}
	for _, c := range cases {
		if got := stripHTML(c.in); got != c.want {
			t.Errorf("stripHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
