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
	"strings"
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
// "images" is a flat array of artwork entries discriminated by coverType
// (Banner|Poster|Fanart|Clearlogo|...), each a TVDB artwork URL. "mappings"
// carries the cross-tracker id block (anidb/mal/tvdb/tmdb/...).
type rawResponse struct {
	Episodes map[string]rawEpisode `json:"episodes"`
	Images   []rawImage            `json:"images"`
	Mappings rawMappings           `json:"mappings"`
}

// rawMappings is ani.zip's top-level id block. Most ids decode as numbers, but
// "themoviedb_id" (and occasionally others) arrive as JSON strings; flexInt
// tolerates both so a single string id never fails the whole decode.
type rawMappings struct {
	AnidbID     flexInt `json:"anidb_id"`
	MalID       flexInt `json:"mal_id"`
	TvdbID      flexInt `json:"thetvdb_id"`
	TmdbID      flexInt `json:"themoviedb_id"`
	AnilistID   flexInt `json:"anilist_id"`
	KitsuID     flexInt `json:"kitsu_id"`
	AnisearchID flexInt `json:"anisearch_id"`
}

// flexInt decodes a JSON value that may be a number or a numeric string into an
// int, defaulting to 0 on null/empty/non-numeric input rather than erroring the
// surrounding decode. ani.zip is inconsistent: themoviedb_id is often a string.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	// Strip surrounding quotes if present (string-encoded id).
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
		s = strings.TrimSpace(s)
	}
	if s == "" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		// Tolerate floats ("123.0") and any other noise by extracting the
		// leading integer; on total failure default to 0 without erroring.
		if g, gErr := strconv.ParseFloat(s, 64); gErr == nil {
			*f = flexInt(int(g))
			return nil
		}
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

// rawImage is one ani.zip artwork entry. coverType discriminates the artwork
// kind ("Clearlogo" is the transparent series logo the home hero uses).
type rawImage struct {
	CoverType string `json:"coverType"`
	URL       string `json:"url"`
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
	// AnidbEid is the per-episode AniDB id; TvdbID is the episode-level TheTVDB
	// id (distinct from the show-level thetvdb_id in mappings). Both feed the
	// extension id-resolver and tolerate string-encoded values.
	AnidbEid flexInt `json:"anidbEid"`
	TvdbID   flexInt `json:"tvdbId"`
}

// GetEpisodes fetches the per-episode metadata for one AniList id, returning the
// episodes sorted by number. A missing/empty mapping yields a nil slice with no
// error (the id simply has no ani.zip coverage). Network/HTTP errors propagate so
// the caller can degrade to an AniList-only payload.
func (c *Client) GetEpisodes(ctx context.Context, anilistID int) ([]Episode, error) {
	raw, ok, err := c.fetchMappings(ctx, anilistID)
	if err != nil || !ok {
		return nil, err
	}
	return mapEpisodes(raw.Episodes), nil
}

// GetHeroArt fetches the mappings for one AniList id ONCE and returns both the
// transparent series logo (the "Clearlogo" artwork entry) and the ordered wide
// hero artwork URLs for the home hero carousel. Every URL is passed through
// safeImageURL so only allowlisted (CSP-pinned) hosts survive.
//
// wide collects only "Fanart" entries — the full 1920x1080 background artwork
// (TVDB /backgrounds/) suited to a full-bleed hero — in source order, deduped.
// The "Banner" cover type is deliberately excluded: it is a 758x140 graphical
// strip (TVDB /banners/graphical/) that pixelates badly when stretched to the
// hero. Poster and Clearlogo are excluded too. A missing logo, non-allowlisted
// host, or a 404 mapping yields ("", nil, nil) — only network/HTTP/decode
// failures propagate so the caller can degrade silently.
func (c *Client) GetHeroArt(ctx context.Context, anilistID int) (clearLogo string, wide []string, err error) {
	raw, ok, err := c.fetchMappings(ctx, anilistID)
	if err != nil || !ok {
		return "", nil, err
	}
	seen := map[string]bool{}
	for _, img := range raw.Images {
		switch {
		case strings.EqualFold(img.CoverType, "Clearlogo"):
			if clearLogo == "" {
				clearLogo = safeImageURL(img.URL)
			}
		case strings.EqualFold(img.CoverType, "Fanart"):
			if u := safeImageURL(img.URL); u != "" && !seen[u] {
				seen[u] = true
				wide = append(wide, u)
			}
		}
	}
	return clearLogo, wide, nil
}

// IDs is the cross-tracker id block ani.zip resolves for one AniList id, plus a
// per-episode id map. Any field is 0 when ani.zip has no value. Episodes is
// keyed by (parsed) episode number.
type IDs struct {
	AnilistID   int
	AnidbID     int
	MalID       int
	TvdbID      int
	TmdbID      int
	KitsuID     int
	AnisearchID int
	Episodes    map[int]EpisodeIDs
}

// EpisodeIDs are the per-episode ids extensions key off of (AniDB episode id and
// the episode-level TheTVDB id).
type EpisodeIDs struct {
	AnidbEid int
	TvdbEid  int
}

// GetIDs resolves the cross-tracker id block for one AniList id via a single
// ani.zip fetch. A missing mapping (HTTP 404 / empty) yields a zero-value IDs
// with a nil error so callers degrade to whatever ids they already carry; only
// network/HTTP/decode failures propagate.
func (c *Client) GetIDs(ctx context.Context, anilistID int) (IDs, error) {
	raw, ok, err := c.fetchMappings(ctx, anilistID)
	if err != nil || !ok {
		return IDs{}, err
	}
	out := IDs{
		AnilistID:   int(raw.Mappings.AnilistID),
		AnidbID:     int(raw.Mappings.AnidbID),
		MalID:       int(raw.Mappings.MalID),
		TvdbID:      int(raw.Mappings.TvdbID),
		TmdbID:      int(raw.Mappings.TmdbID),
		KitsuID:     int(raw.Mappings.KitsuID),
		AnisearchID: int(raw.Mappings.AnisearchID),
		Episodes:    make(map[int]EpisodeIDs, len(raw.Episodes)),
	}
	// The mappings block may omit anilist_id; backfill the requested id so a
	// caller that only checks ids.AnilistID still sees it.
	if out.AnilistID == 0 {
		out.AnilistID = anilistID
	}
	for key, e := range raw.Episodes {
		num := e.EpisodeNumber
		if num == 0 {
			if n, err := strconv.Atoi(key); err == nil {
				num = n
			}
		}
		if num == 0 {
			continue
		}
		out.Episodes[num] = EpisodeIDs{
			AnidbEid: int(e.AnidbEid),
			TvdbEid:  int(e.TvdbID),
		}
	}
	return out, nil
}

// fetchMappings GETs and decodes the ani.zip mappings envelope for one id. The
// bool is false (with nil error) when the id has no mapping (HTTP 404), so
// callers degrade to empty coverage; network/HTTP/decode errors propagate.
func (c *Client) fetchMappings(ctx context.Context, anilistID int) (rawResponse, bool, error) {
	u := c.endpoint + "?" + url.Values{"anilist_id": {strconv.Itoa(anilistID)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return rawResponse{}, false, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return rawResponse{}, false, fmt.Errorf("anizip: request: %w", err)
	}
	defer resp.Body.Close()

	// 404 = no mapping for this id; treat as empty coverage, not an error.
	if resp.StatusCode == http.StatusNotFound {
		return rawResponse{}, false, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if err != nil {
		return rawResponse{}, false, err
	}
	if resp.StatusCode != http.StatusOK {
		return rawResponse{}, false, fmt.Errorf("anizip: status %s", resp.Status)
	}

	var raw rawResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return rawResponse{}, false, fmt.Errorf("anizip: decode: %w", err)
	}
	return raw, true, nil
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
