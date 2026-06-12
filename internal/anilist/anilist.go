// Package anilist is a minimal AniList GraphQL client. It queries the public
// (no-auth) https://graphql.anilist.co endpoint for anime media metadata used to
// enrich series rows and feed the sourcing layer's SmartSearch. Responses are
// cached in-memory (bounded) and the client backs off politely on HTTP 429.
package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Endpoint is the public AniList GraphQL API.
const Endpoint = "https://graphql.anilist.co"

const (
	defaultTimeout  = 15 * time.Second
	defaultCacheCap = 512
	maxRetries      = 3
)

// maxRespBytes caps the AniList response body. A single trimmed Media record is
// a few KiB; 4 MiB bounds a hostile or runaway upstream from streaming an
// unbounded body into the decoder.
const maxRespBytes = 4 << 20

// Media is the trimmed AniList media metadata this app needs.
type Media struct {
	ID           int      `json:"id"`
	IDMal        *int     `json:"idMal"`
	RomajiTitle  string   `json:"romajiTitle"`
	EnglishTitle string   `json:"englishTitle"`
	NativeTitle  string   `json:"nativeTitle"`
	Format       string   `json:"format"`       // TV|MOVIE|OVA|ONA|SPECIAL|...
	Status       string   `json:"status"`       // RELEASING|FINISHED|NOT_YET_RELEASED|CANCELLED|HIATUS
	EpisodeCount int      `json:"episodeCount"` // 0 when unknown
	Season       string   `json:"season"`       // WINTER|SPRING|SUMMER|FALL
	SeasonYear   int      `json:"seasonYear"`
	CoverImage   string   `json:"coverImage"`
	CoverColor   string   `json:"coverColor"`
	BannerImage  string   `json:"bannerImage"`
	Synonyms     []string `json:"synonyms"`
	IsAdult      bool     `json:"isAdult"`
}

// Client is a cached AniList GraphQL client. Safe for concurrent use.
type Client struct {
	http     *http.Client
	endpoint string

	mu        sync.Mutex
	cache     map[string]Media   // key -> media (by-id fetches)
	order     []string           // insertion order for bounded eviction
	listCache map[string][]Media // key -> media list (search results)
	listOrder []string
	cacheCap  int
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the HTTP client (e.g. to share a transport).
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) {
		if c != nil {
			cl.http = c
		}
	}
}

// WithCacheCap sets the maximum number of cached media (FIFO eviction).
func WithCacheCap(n int) Option {
	return func(cl *Client) {
		if n > 0 {
			cl.cacheCap = n
		}
	}
}

// New builds an AniList client.
func New(opts ...Option) *Client {
	c := &Client{
		http:      &http.Client{Timeout: defaultTimeout},
		endpoint:  Endpoint,
		cache:     make(map[string]Media),
		listCache: make(map[string][]Media),
		cacheCap:  defaultCacheCap,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GetMedia fetches a media by AniList id, serving from cache when present.
func (c *Client) GetMedia(ctx context.Context, id int) (Media, error) {
	key := "id:" + strconv.Itoa(id)
	if m, ok := c.cacheGet(key); ok {
		return m, nil
	}
	m, err := c.query(ctx, mediaByIDQuery, map[string]any{"id": id})
	if err != nil {
		return Media{}, err
	}
	c.cachePut(key, m)
	return m, nil
}

// SearchMedia fetches the top anime media matching a free-text query, ranked by
// search relevance.
func (c *Client) SearchMedia(ctx context.Context, query string) ([]Media, error) {
	key := "search:" + query
	if list, ok := c.listCacheGet(key); ok {
		return list, nil
	}
	body, err := c.fetch(ctx, mediaSearchQuery, map[string]any{"search": query})
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

// batchChunkSize is the AniList Page perPage maximum: at most 50 media per
// request. ids beyond this are split across multiple requests.
const batchChunkSize = 50

// GetMediaBatch fetches multiple media by AniList id in as few requests as
// possible (one per chunk of up to 50 ids), returning a map keyed by media id.
// Zero/duplicate ids are ignored. Each result is also written to the by-id cache
// so subsequent GetMedia calls hit it.
//
// On a rate-limit/network error mid-run, the whole call fails (returns the
// error) rather than returning partial results — the refresher retries the full
// set next tick, which keeps "metadata_refreshed_at" honest (a series is only
// stamped once its data was actually obtained).
func (c *Client) GetMediaBatch(ctx context.Context, ids []int) (map[int]Media, error) {
	chunks := chunkIDs(ids, batchChunkSize)
	out := make(map[int]Media)
	for _, chunk := range chunks {
		body, err := c.fetch(ctx, mediaBatchQuery, map[string]any{"ids": chunk})
		if err != nil {
			return nil, err
		}
		list, err := decodeMediaList(body)
		if err != nil {
			return nil, err
		}
		for _, m := range list {
			out[m.ID] = m
			c.cachePut("id:"+strconv.Itoa(m.ID), m)
		}
	}
	return out, nil
}

// chunkIDs dedupes and drops zero ids, then splits the rest into chunks of size.
func chunkIDs(ids []int, size int) [][]int {
	seen := make(map[int]struct{}, len(ids))
	uniq := make([]int, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	var chunks [][]int
	for start := 0; start < len(uniq); start += size {
		end := start + size
		if end > len(uniq) {
			end = len(uniq)
		}
		chunks = append(chunks, uniq[start:end])
	}
	return chunks
}

// query POSTs a GraphQL request and maps the single Media result.
func (c *Client) query(ctx context.Context, gql string, vars map[string]any) (Media, error) {
	body, err := c.fetch(ctx, gql, vars)
	if err != nil {
		return Media{}, err
	}
	return decodeMedia(body)
}

// fetch POSTs a GraphQL request and returns the raw response body, retrying with
// backoff on 429 (honoring Retry-After when present).
func (c *Client) fetch(ctx context.Context, gql string, vars map[string]any) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{"query": gql, "variables": vars})
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("anilist: request: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := retryAfter(resp.Header.Get("Retry-After"), attempt)
			resp.Body.Close()
			lastErr = fmt.Errorf("anilist: rate limited (429)")
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("anilist: status %s: %s", resp.Status, truncate(body, 200))
		}
		return body, nil
	}
	return nil, lastErr
}

// retryAfter computes the wait before a retry: the Retry-After header (seconds)
// when present, else exponential backoff (1s, 2s, 4s).
func retryAfter(header string, attempt int) time.Duration {
	if header != "" {
		if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Duration(1<<attempt) * time.Second
}

func (c *Client) cacheGet(key string) (Media, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, ok := c.cache[key]
	return m, ok
}

func (c *Client) cachePut(key string, m Media) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.cache[key]; !exists {
		c.order = append(c.order, key)
		for len(c.order) > c.cacheCap {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.cache, oldest)
		}
	}
	c.cache[key] = m
}

func (c *Client) listCacheGet(key string) ([]Media, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	list, ok := c.listCache[key]
	return list, ok
}

func (c *Client) listCachePut(key string, list []Media) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.listCache[key]; !exists {
		c.listOrder = append(c.listOrder, key)
		for len(c.listOrder) > c.cacheCap {
			oldest := c.listOrder[0]
			c.listOrder = c.listOrder[1:]
			delete(c.listCache, oldest)
		}
	}
	c.listCache[key] = list
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
