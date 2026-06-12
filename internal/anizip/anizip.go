// Package anizip is a minimal client for the public ani.zip mappings API
// (https://api.ani.zip/mappings?anilist_id=N), the same per-episode metadata
// source Hayase uses. It returns TVDB-sourced episode thumbnails, titles, air
// dates, runtimes, and overviews keyed by AniList id. It is a best-effort
// third-party source: callers degrade gracefully when it errors (the detail
// page still renders from AniList alone).
package anizip

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// Endpoint is the public ani.zip mappings API base.
const Endpoint = "https://api.ani.zip/mappings"

const defaultTimeout = 10 * time.Second

// maxRespBytes caps the response body. A full long-running show's episode map is
// a few hundred KiB; 8 MiB bounds a hostile or runaway upstream.
const maxRespBytes = 8 << 20

// imageHostAllow is the set of hosts ani.zip serves episode thumbnails from,
// mirrored in the server CSP img-src. A thumbnail URL on any other host is
// dropped before it reaches the frontend.
var imageHostAllow = map[string]bool{
	"artworks.thetvdb.com":    true,
	"img1.ak.crunchyroll.com": true,
}

// Episode is the trimmed per-episode metadata the detail page needs. Number is
// the parsed episode key; the rest are best-effort (any may be empty/zero).
type Episode struct {
	Number     int
	Title      string
	Thumbnail  string
	AirDate    string // ISO date, e.g. "1999-10-20"
	Overview   string
	RuntimeMin int
}

// Client fetches ani.zip mappings. Safe for concurrent use.
type Client struct {
	http     *http.Client
	endpoint string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) {
		if c != nil {
			cl.http = c
		}
	}
}

// New builds an ani.zip client.
func New(opts ...Option) *Client {
	c := &Client{
		http:     &http.Client{Timeout: defaultTimeout},
		endpoint: Endpoint,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// rawResponse is the ani.zip mappings envelope. "episodes" is an object keyed by
// episode number as a string; absolute-numbered specials use non-integer keys.
type rawResponse struct {
	Episodes map[string]rawEpisode `json:"episodes"`
}

// rawEpisode covers the field-name inconsistencies in ani.zip's payload: title
// is a language map (prefer "en", fall back to "x-jat"); both airDate/airdate
// and runtime/length appear; overview and summary both carry descriptions.
type rawEpisode struct {
	EpisodeNumber int               `json:"episodeNumber"`
	Episode       string            `json:"episode"`
	Title         map[string]string `json:"title"`
	AirDate       string            `json:"airDate"`
	AirDateLower  string            `json:"airdate"`
	Runtime       int               `json:"runtime"`
	Length        int               `json:"length"`
	Overview      string            `json:"overview"`
	Summary       string            `json:"summary"`
	Image         string            `json:"image"`
}

// GetEpisodes fetches the per-episode metadata for one AniList id, returning the
// episodes sorted by number. A missing/empty mapping yields a nil slice with no
// error (the id simply has no ani.zip coverage). Network/HTTP errors propagate so
// the caller can degrade to an AniList-only payload.
func (c *Client) GetEpisodes(ctx context.Context, anilistID int) ([]Episode, error) {
	u := c.endpoint + "?" + url.Values{"anilist_id": {strconv.Itoa(anilistID)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anizip: request: %w", err)
	}
	defer resp.Body.Close()

	// 404 = no mapping for this id; treat as empty coverage, not an error.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anizip: status %s", resp.Status)
	}

	var raw rawResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("anizip: decode: %w", err)
	}
	return mapEpisodes(raw.Episodes), nil
}

// mapEpisodes flattens the keyed episode object into a sorted slice, resolving
// the field-name inconsistencies and dropping entries without a usable integer
// number (the keyed map's non-integer specials).
func mapEpisodes(in map[string]rawEpisode) []Episode {
	out := make([]Episode, 0, len(in))
	for key, e := range in {
		num := e.EpisodeNumber
		if num == 0 {
			// Fall back to the map key, then the "episode" string field.
			if n, err := strconv.Atoi(key); err == nil {
				num = n
			} else if n, err := strconv.Atoi(e.Episode); err == nil {
				num = n
			}
		}
		if num == 0 {
			continue
		}
		ep := Episode{
			Number:     num,
			Title:      pickTitle(e.Title),
			Thumbnail:  safeImageURL(e.Image),
			AirDate:    pick(e.AirDate, e.AirDateLower),
			Overview:   pick(e.Overview, e.Summary),
			RuntimeMin: pickInt(e.Runtime, e.Length),
		}
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// pickTitle prefers the English title, then the romanized Japanese ("x-jat"),
// then any present title.
func pickTitle(m map[string]string) string {
	if m == nil {
		return ""
	}
	if v := m["en"]; v != "" {
		return v
	}
	if v := m["x-jat"]; v != "" {
		return v
	}
	for _, v := range m {
		if v != "" {
			return v
		}
	}
	return ""
}

func pick(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func pickInt(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

// safeImageURL returns raw only if it is an https URL on an allowlisted episode-
// thumbnail host (the same hosts the server CSP whitelists); else "".
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
