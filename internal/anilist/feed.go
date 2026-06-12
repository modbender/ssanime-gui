package anilist

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// defaultFeedPerPage is how many media a discovery feed query returns. ~24 fills
// a couple of carousel screens without paying for a full 50-item page.
const defaultFeedPerPage = 24

// FeedSort is an AniList MediaSort value used to order a discovery feed.
type FeedSort string

const (
	// SortTrending orders by AniList's trending signal (recent activity).
	SortTrending FeedSort = "TRENDING_DESC"
	// SortPopularity orders by all-time popularity (member count).
	SortPopularity FeedSort = "POPULARITY_DESC"
)

// FeedSpec describes one discovery feed query: a sort plus optional season/genre
// constraints. The zero value (sort only) is a valid all-anime feed. A FeedSpec
// is the single unit the discovery cache iterates over.
type FeedSpec struct {
	// Sort is the MediaSort applied (TRENDING_DESC, POPULARITY_DESC).
	Sort FeedSort
	// Genre, when non-empty, restricts to media tagged with this AniList genre.
	Genre string
	// Season + SeasonYear, when both set, restrict to one airing season. Use
	// CurrentSeason to fill them with the live season.
	Season     string
	SeasonYear int
	// PerPage overrides the default page size when > 0.
	PerPage int
}

// feedQuery is the one shared query shape for every discovery feed. AniList
// ignores null variables, so a single parameterized query serves trending,
// popular, seasonal, and genre feeds. isAdult is pinned false to keep the home
// SFW.
const feedQuery = `query ($page: Int, $perPage: Int, $sort: [MediaSort], $season: MediaSeason, $seasonYear: Int, $genre: String) {
  Page(page: $page, perPage: $perPage) {
    media(type: ANIME, sort: $sort, season: $season, seasonYear: $seasonYear, genre: $genre, isAdult: false) {` + mediaFields + `
    }
  }
}`

// ListByFeed fetches one discovery feed's media via the shared paged query,
// serving from the list cache when present. The cache key encodes the full spec
// so trending/seasonal/genre feeds never collide. Errors (rate limit, network)
// propagate to the caller, which decides whether to serve a previously-cached
// slice.
func (c *Client) ListByFeed(ctx context.Context, spec FeedSpec) ([]Media, error) {
	key := feedCacheKey(spec)
	if list, ok := c.listCacheGet(key); ok {
		return list, nil
	}

	perPage := spec.PerPage
	if perPage <= 0 {
		perPage = defaultFeedPerPage
	}
	vars := map[string]any{
		"page":    1,
		"perPage": perPage,
		"sort":    []string{string(spec.Sort)},
	}
	if spec.Season != "" && spec.SeasonYear > 0 {
		vars["season"] = strings.ToUpper(spec.Season)
		vars["seasonYear"] = spec.SeasonYear
	}
	if spec.Genre != "" {
		vars["genre"] = spec.Genre
	}

	body, err := c.fetch(ctx, feedQuery, vars)
	if err != nil {
		return nil, err
	}
	list, err := decodeMediaList(body)
	if err != nil {
		return nil, err
	}
	c.listCachePut(key, list)
	return list, nil
}

// feedCacheKey builds a stable cache key from a spec's discriminating fields.
func feedCacheKey(spec FeedSpec) string {
	var b strings.Builder
	b.WriteString("feed:")
	b.WriteString(string(spec.Sort))
	if spec.Genre != "" {
		b.WriteString("|g=" + spec.Genre)
	}
	if spec.Season != "" && spec.SeasonYear > 0 {
		b.WriteString("|s=" + strings.ToUpper(spec.Season) + strconv.Itoa(spec.SeasonYear))
	}
	return b.String()
}

// CurrentSeason returns the AniList MediaSeason and year for the given time, so a
// seasonal feed tracks the live season without a hardcoded date.
func CurrentSeason(t time.Time) (season string, year int) {
	switch t.Month() {
	case time.December, time.January, time.February:
		season = "WINTER"
	case time.March, time.April, time.May:
		season = "SPRING"
	case time.June, time.July, time.August:
		season = "SUMMER"
	default:
		season = "FALL"
	}
	year = t.Year()
	// December belongs to the upcoming year's WINTER season on AniList.
	if t.Month() == time.December {
		year++
	}
	return season, year
}

// DescribeFeed is a human-readable label for a spec, used in logs.
func DescribeFeed(spec FeedSpec) string {
	if spec.Genre != "" {
		return fmt.Sprintf("genre:%s", spec.Genre)
	}
	if spec.Season != "" {
		return fmt.Sprintf("season:%s%d", spec.Season, spec.SeasonYear)
	}
	return string(spec.Sort)
}
