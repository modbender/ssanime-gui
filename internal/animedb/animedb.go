// Package animedb is an offline, in-memory search index over the
// manami-project/anime-offline-database dataset. It exists to take the
// add-series "search as you type / submit" lookup off AniList's GraphQL API
// (throttled to 30 req/min): the local index answers title queries with zero
// network calls, leaving only the single by-id metadata fetch at add time on
// AniList.
//
// The dataset (~40k entries, ~11 MiB zstd-compressed) is downloaded once into
// app-data, refreshed weekly, and decoded by streaming so the raw bytes and a
// fully-parsed tree never coexist in memory. Only entries carrying an AniList
// source are indexed; everything else is dropped on load.
package animedb

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

// dataFileName is the on-disk name of the cached compressed dataset.
const dataFileName = "anime-offline-database-minified.json.zst"

// downloadURL is the manami-project release asset (zstd-compressed minified
// dataset). The /releases/latest/download/ path always resolves to the newest
// weekly release, so we never hard-code a tag. GitHub serves this over normal
// DNS+HTTPS — it must NOT be routed through the DoH resolver (that is reserved
// for the DNS-blocked nyaa.si).
const downloadURL = "https://github.com/manami-project/anime-offline-database/releases/latest/download/anime-offline-database-minified.json.zst"

// maxDownloadBytes hard-caps the download. The compressed asset is ~11 MiB; a
// 200 MiB ceiling leaves generous headroom for dataset growth while bounding a
// hostile/runaway response.
const maxDownloadBytes int64 = 200 << 20

// maxDecodedBytes caps the decompressed stream the JSON decoder will read, so a
// zip-bomb-style compressed payload can't exhaust memory during decode.
const maxDecodedBytes int64 = 1 << 30 // 1 GiB

// staleAfter is how long a cached dataset is trusted before a background
// refresh is triggered on the next Start.
const staleAfter = 7 * 24 * time.Hour

// Result is one search hit, already mapped toward the AniList-style shape the
// HTTP layer needs. Status/Type use AniList vocabulary (see mapStatus/mapType).
type Result struct {
	AniListID int
	Title     string
	Synonyms  []string
	Type      string // FORMAT: TV, MOVIE, OVA, ONA, SPECIAL ("" if UNKNOWN)
	Status    string // AniList-style: FINISHED, RELEASING, NOT_YET_RELEASED ("" if UNKNOWN)
	Episodes  int
	Picture   string // source CDN url (often cdn.myanimelist.net)
	Season    string // SPRING/SUMMER/FALL/WINTER ("" if UNDEFINED)
	Year      int
}

// record is the compact indexed form of one anime entry. normTitle and
// normSynonyms are precomputed normalized strings so Search does no per-query
// normalization of the corpus.
type record struct {
	Result
	normTitle    string
	normSynonyms []string
}

// DB is the offline index. The index slice is swapped wholesale under mu on
// every (re)load, so Search never observes a half-built corpus.
type DB struct {
	dataDir    string
	httpClient *http.Client
	logger     *slog.Logger

	mu    sync.RWMutex
	index []record
	ready bool

	stopOnce sync.Once
	cancel   context.CancelFunc
}

// Option configures a DB.
type Option func(*DB)

// WithHTTPClient sets the client used for the dataset download. A plain client
// is expected (normal DNS) — do not pass the DoH-guarded client.
func WithHTTPClient(c *http.Client) Option {
	return func(d *DB) { d.httpClient = c }
}

// WithLogger sets the structured logger.
func WithLogger(l *slog.Logger) Option {
	return func(d *DB) { d.logger = l }
}

// New constructs a DB rooted at dataDir/animedb. The dataset is not loaded
// until Start is called.
func New(dataDir string, opts ...Option) *DB {
	d := &DB{
		dataDir:    filepath.Join(dataDir, "animedb"),
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		logger:     slog.Default(),
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// dataPath is the absolute path of the cached compressed dataset file.
func (d *DB) dataPath() string {
	return filepath.Join(d.dataDir, dataFileName)
}

// Ready reports whether the in-memory index has been loaded at least once.
func (d *DB) Ready() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ready
}

// Start loads the index. If a fresh local copy exists it is loaded
// synchronously-fast (a local file decode); if the copy is missing or stale the
// download + load runs in a background goroutine so daemon startup never blocks
// on a multi-MiB fetch. The provided ctx bounds the background work.
func (d *DB) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	fresh := d.localFresh()
	if fresh {
		// A fresh cache is cheap to decode; do it inline so Ready() is true the
		// moment Start returns when possible. Still guarded by ctx.
		if err := d.loadFromFile(ctx); err != nil {
			d.logger.Warn("animedb: load cached dataset failed, refreshing", "err", err)
			go d.refresh(ctx)
		}
		return
	}

	go d.refresh(ctx)
}

// Stop cancels any in-flight background refresh.
func (d *DB) Stop() {
	d.stopOnce.Do(func() {
		if d.cancel != nil {
			d.cancel()
		}
	})
}

// refresh downloads a fresh dataset (respecting ctx) and loads it. On any
// failure it falls back to whatever local copy exists, so a network blip during
// first boot still yields a working index once a cache is present.
func (d *DB) refresh(ctx context.Context) {
	if err := d.download(ctx); err != nil {
		d.logger.Warn("animedb: dataset download failed", "err", err)
		// Try whatever stale copy we have rather than leaving the index empty.
		if err := d.loadFromFile(ctx); err != nil {
			d.logger.Warn("animedb: no usable local dataset", "err", err)
		}
		return
	}
	if err := d.loadFromFile(ctx); err != nil {
		d.logger.Warn("animedb: load downloaded dataset failed", "err", err)
	}
}
