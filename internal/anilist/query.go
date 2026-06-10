package anilist

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// imageHostAllow is the set of hosts AniList serves cover/banner images from.
// Image URLs come from an untrusted upstream response and are stored, then
// rendered by the SPA, so we pin them to known CDN hosts before persisting —
// the same hosts the server CSP img-src whitelists. Add a host with one entry.
var imageHostAllow = map[string]bool{
	"s4.anilist.co": true,
	"img.anili.st":  true,
}

// safeImageURL returns raw if it is an https URL on an allowlisted AniList CDN
// host, else "" — dropping anything that could be a javascript:/data: payload or
// a redirect to an attacker host.
func safeImageURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !imageHostAllow[u.Hostname()] {
		return ""
	}
	return raw
}

// mediaFields is the shared selection set for both queries.
const mediaFields = `
    id
    idMal
    format
    status
    episodes
    season
    seasonYear
    isAdult
    title { romaji english native }
    coverImage { large extraLarge color }
    bannerImage
    synonyms`

var (
	mediaByIDQuery = `query ($id: Int) {
  Media(id: $id, type: ANIME) {` + mediaFields + `
  }
}`

	mediaSearchQuery = `query ($search: String) {
  Media(search: $search, type: ANIME, sort: SEARCH_MATCH) {` + mediaFields + `
  }
}`
)

// graphQLResponse is the AniList envelope. Errors come back with HTTP 200, so the
// errors array must be checked even on success.
type graphQLResponse struct {
	Data struct {
		Media *rawMedia `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type rawMedia struct {
	ID         int    `json:"id"`
	IDMal      *int   `json:"idMal"`
	Format     string `json:"format"`
	Status     string `json:"status"`
	Episodes   int    `json:"episodes"`
	Season     string `json:"season"`
	SeasonYear int    `json:"seasonYear"`
	IsAdult    bool   `json:"isAdult"`
	Title      struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
	CoverImage struct {
		Large      string `json:"large"`
		ExtraLarge string `json:"extraLarge"`
		Color      string `json:"color"`
	} `json:"coverImage"`
	BannerImage string   `json:"bannerImage"`
	Synonyms    []string `json:"synonyms"`
}

// decodeMedia maps a GraphQL response body to a Media, surfacing GraphQL errors.
func decodeMedia(body []byte) (Media, error) {
	var r graphQLResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Media{}, fmt.Errorf("anilist: decode: %w", err)
	}
	if len(r.Errors) > 0 {
		return Media{}, fmt.Errorf("anilist: %s", r.Errors[0].Message)
	}
	if r.Data.Media == nil {
		return Media{}, fmt.Errorf("anilist: no media found")
	}
	m := r.Data.Media
	cover := safeImageURL(m.CoverImage.ExtraLarge)
	if cover == "" {
		cover = safeImageURL(m.CoverImage.Large)
	}
	return Media{
		ID:           m.ID,
		IDMal:        m.IDMal,
		RomajiTitle:  m.Title.Romaji,
		EnglishTitle: m.Title.English,
		NativeTitle:  m.Title.Native,
		Format:       m.Format,
		Status:       m.Status,
		EpisodeCount: m.Episodes,
		Season:       m.Season,
		SeasonYear:   m.SeasonYear,
		CoverImage:   cover,
		CoverColor:   m.CoverImage.Color,
		BannerImage:  safeImageURL(m.BannerImage),
		Synonyms:     m.Synonyms,
		IsAdult:      m.IsAdult,
	}, nil
}
