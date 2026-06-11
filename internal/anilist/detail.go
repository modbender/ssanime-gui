package anilist

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
)

// detailFields is the full Media selection set the series-detail page needs,
// beyond the trimmed mediaFields used for cards and refresh. It pulls synopsis,
// genres, score, studios, source/season metadata, the trailer, AniList's own
// streamingEpisodes (the ani.zip thumbnail fallback), the next airing episode,
// and the relation/recommendation graphs (each carrying enough to render a card
// and navigate to that title's preview).
const detailFields = `
    id
    idMal
    description(asHtml: false)
    genres
    averageScore
    source
    season
    seasonYear
    duration
    episodes
    format
    status
    title { romaji english native }
    coverImage { large extraLarge color }
    bannerImage
    studios(isMain: true) { nodes { name isAnimationStudio } }
    trailer { id site thumbnail }
    streamingEpisodes { title thumbnail }
    nextAiringEpisode { episode airingAt }
    relations {
      edges {
        relationType(version: 2)
        node {
          id format status
          title { romaji english }
          coverImage { large color }
        }
      }
    }
    recommendations(sort: RATING_DESC, perPage: 12) {
      nodes {
        mediaRecommendation {
          id format status
          title { romaji english }
          coverImage { large color }
        }
      }
    }`

var detailByIDQuery = `query ($id: Int) {
  Media(id: $id, type: ANIME) {` + detailFields + `
  }
}`

// MediaDetail is the full AniList detail this app surfaces on the series page.
// Description is already HTML-stripped. Image URLs are CSP-pinned.
type MediaDetail struct {
	ID                int
	Description       string
	Genres            []string
	AverageScore      int
	Studio            string
	Source            string
	Season            string
	SeasonYear        int
	Duration          int
	EpisodeCount      int
	Format            string
	Status            string
	RomajiTitle       string
	EnglishTitle      string
	CoverImage        string
	CoverColor        string
	BannerImage       string
	Trailer           *Trailer
	StreamingEpisodes []StreamingEpisode
	NextAiring        *AiringEpisode
	Relations         []RelatedMedia
	Recommendations   []RelatedMedia
}

// Trailer is an external trailer reference (the app opens it in a new tab; it is
// never embedded, to keep the CSP frame-free).
type Trailer struct {
	Site      string // "youtube" | "dailymotion"
	VideoID   string
	Thumbnail string
}

// StreamingEpisode is one AniList streaming-episode entry, used as the episode
// thumbnail fallback when ani.zip has no coverage.
type StreamingEpisode struct {
	Title     string
	Thumbnail string
}

// AiringEpisode is the next-airing episode number and its unix airing time.
type AiringEpisode struct {
	Episode  int
	AiringAt int64
}

// RelatedMedia is a relation or recommendation node, carrying enough to render a
// discovery card and navigate to that title's preview.
type RelatedMedia struct {
	AnilistID    int
	RelationType string // empty for recommendations
	EnglishTitle string
	RomajiTitle  string
	CoverImage   string
	CoverColor   string
	Format       string
	Status       string
}

// GetDetail fetches the full Media detail for one AniList id. Unlike the trimmed
// GetMedia, this is not cached in the in-memory client cache — the server caches
// the merged detail payload durably in SQLite instead.
func (c *Client) GetDetail(ctx context.Context, id int) (MediaDetail, error) {
	body, err := c.fetch(ctx, detailByIDQuery, map[string]any{"id": id})
	if err != nil {
		return MediaDetail{}, err
	}
	return decodeDetail(body)
}

// rawDetail is the GraphQL Media shape for the detail query.
type rawDetail struct {
	ID           int      `json:"id"`
	Description  string   `json:"description"`
	Genres       []string `json:"genres"`
	AverageScore int      `json:"averageScore"`
	Source       string   `json:"source"`
	Season       string   `json:"season"`
	SeasonYear   int      `json:"seasonYear"`
	Duration     int      `json:"duration"`
	Episodes     int      `json:"episodes"`
	Format       string   `json:"format"`
	Status       string   `json:"status"`
	Title        struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
	CoverImage struct {
		Large      string `json:"large"`
		ExtraLarge string `json:"extraLarge"`
		Color      string `json:"color"`
	} `json:"coverImage"`
	BannerImage string `json:"bannerImage"`
	Studios     struct {
		Nodes []struct {
			Name              string `json:"name"`
			IsAnimationStudio bool   `json:"isAnimationStudio"`
		} `json:"nodes"`
	} `json:"studios"`
	Trailer *struct {
		ID        string `json:"id"`
		Site      string `json:"site"`
		Thumbnail string `json:"thumbnail"`
	} `json:"trailer"`
	StreamingEpisodes []struct {
		Title     string `json:"title"`
		Thumbnail string `json:"thumbnail"`
	} `json:"streamingEpisodes"`
	NextAiringEpisode *struct {
		Episode  int   `json:"episode"`
		AiringAt int64 `json:"airingAt"`
	} `json:"nextAiringEpisode"`
	Relations struct {
		Edges []struct {
			RelationType string          `json:"relationType"`
			Node         *rawRelatedNode `json:"node"`
		} `json:"edges"`
	} `json:"relations"`
	Recommendations struct {
		Nodes []struct {
			MediaRecommendation *rawRelatedNode `json:"mediaRecommendation"`
		} `json:"nodes"`
	} `json:"recommendations"`
}

type rawRelatedNode struct {
	ID     int    `json:"id"`
	Format string `json:"format"`
	Status string `json:"status"`
	Title  struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
	} `json:"title"`
	CoverImage struct {
		Large string `json:"large"`
		Color string `json:"color"`
	} `json:"coverImage"`
}

type detailResponse struct {
	Data struct {
		Media *rawDetail `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// decodeDetail maps a detail GraphQL response to a MediaDetail, surfacing
// GraphQL errors (which arrive with HTTP 200).
func decodeDetail(body []byte) (MediaDetail, error) {
	var r detailResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return MediaDetail{}, fmt.Errorf("anilist: decode detail: %w", err)
	}
	if len(r.Errors) > 0 {
		return MediaDetail{}, fmt.Errorf("anilist: %s", r.Errors[0].Message)
	}
	if r.Data.Media == nil {
		return MediaDetail{}, fmt.Errorf("anilist: no media found")
	}
	return mapDetail(r.Data.Media), nil
}

func mapDetail(m *rawDetail) MediaDetail {
	cover := safeImageURL(m.CoverImage.ExtraLarge)
	if cover == "" {
		cover = safeImageURL(m.CoverImage.Large)
	}
	d := MediaDetail{
		ID:           m.ID,
		Description:  stripHTML(m.Description),
		Genres:       m.Genres,
		AverageScore: m.AverageScore,
		Source:       m.Source,
		Season:       m.Season,
		SeasonYear:   m.SeasonYear,
		Duration:     m.Duration,
		EpisodeCount: m.Episodes,
		Format:       m.Format,
		Status:       m.Status,
		RomajiTitle:  m.Title.Romaji,
		EnglishTitle: m.Title.English,
		CoverImage:   cover,
		CoverColor:   m.CoverImage.Color,
		BannerImage:  safeImageURL(m.BannerImage),
		Studio:       pickStudio(m.Studios.Nodes),
	}
	if m.Trailer != nil && m.Trailer.ID != "" {
		d.Trailer = &Trailer{
			Site:      m.Trailer.Site,
			VideoID:   m.Trailer.ID,
			Thumbnail: safeTrailerThumb(m.Trailer.Thumbnail),
		}
	}
	for _, se := range m.StreamingEpisodes {
		d.StreamingEpisodes = append(d.StreamingEpisodes, StreamingEpisode{
			Title:     se.Title,
			Thumbnail: safeStreamThumb(se.Thumbnail),
		})
	}
	if m.NextAiringEpisode != nil {
		d.NextAiring = &AiringEpisode{
			Episode:  m.NextAiringEpisode.Episode,
			AiringAt: m.NextAiringEpisode.AiringAt,
		}
	}
	for _, e := range m.Relations.Edges {
		if e.Node == nil {
			continue
		}
		rel := mapRelatedNode(e.Node)
		rel.RelationType = e.RelationType
		d.Relations = append(d.Relations, rel)
	}
	for _, n := range m.Recommendations.Nodes {
		if n.MediaRecommendation == nil {
			continue
		}
		d.Recommendations = append(d.Recommendations, mapRelatedNode(n.MediaRecommendation))
	}
	return d
}

func mapRelatedNode(n *rawRelatedNode) RelatedMedia {
	return RelatedMedia{
		AnilistID:    n.ID,
		EnglishTitle: n.Title.English,
		RomajiTitle:  n.Title.Romaji,
		CoverImage:   safeImageURL(n.CoverImage.Large),
		CoverColor:   n.CoverImage.Color,
		Format:       n.Format,
		Status:       n.Status,
	}
}

// pickStudio returns the first animation studio's name, falling back to the
// first studio of any kind (isMain already filtered the set upstream).
func pickStudio(nodes []struct {
	Name              string `json:"name"`
	IsAnimationStudio bool   `json:"isAnimationStudio"`
}) string {
	for _, n := range nodes {
		if n.IsAnimationStudio && n.Name != "" {
			return n.Name
		}
	}
	for _, n := range nodes {
		if n.Name != "" {
			return n.Name
		}
	}
	return ""
}

// trailerThumbHosts and streamThumbHosts pin the CSP-allowed thumbnail hosts for
// the trailer (YouTube) and AniList streamingEpisodes (Crunchyroll) respectively.
var trailerThumbHosts = map[string]bool{"i.ytimg.com": true}
var streamThumbHosts = map[string]bool{"img1.ak.crunchyroll.com": true}

func safeTrailerThumb(raw string) string { return safeImageHost(raw, trailerThumbHosts) }
func safeStreamThumb(raw string) string  { return safeImageHost(raw, streamThumbHosts) }

func safeImageHost(raw string, allow map[string]bool) string {
	if raw == "" {
		return ""
	}
	i := strings.Index(raw, "://")
	if i < 0 {
		return ""
	}
	rest := raw[i+3:]
	host := rest
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		host = rest[:j]
	}
	if !strings.HasPrefix(raw, "https://") || !allow[host] {
		return ""
	}
	return raw
}

// htmlTagRe matches HTML tags AniList embeds in descriptions (<br>, <i>, <b>...).
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// stripHTML removes HTML tags from an AniList description and unescapes entities,
// collapsing <br> runs to newlines so the plain-text synopsis keeps paragraphs.
func stripHTML(s string) string {
	if s == "" {
		return ""
	}
	// Normalize <br> variants to newlines before stripping the rest.
	s = brRe.ReplaceAllString(s, "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	// Collapse 3+ newlines to a paragraph break; trim surrounding whitespace.
	s = excessNewlineRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

var (
	brRe            = regexp.MustCompile(`(?i)<br\s*/?>`)
	excessNewlineRe = regexp.MustCompile(`\n{3,}`)
)
