package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/source"
)

// IDResolver resolves the cross-tracker id block for an AniList id. *anizip.Client
// satisfies it; tests stub it. It lives in this package so the adapter can build
// the Hayase options object without importing the manager.
type IDResolver interface {
	GetIDs(ctx context.Context, anilistID int) (anizip.IDs, error)
}

// JSProvider implements source.Provider by delegating to a JS extension loaded
// into a goja VM. Each method call creates a fresh runtime (compile-once,
// run-per-call) for isolation: a crash in one call can't corrupt another's
// state. The cost is the JS execution time, not compilation.
type JSProvider struct {
	id      string
	name    string
	vm      *VM
	logger  *slog.Logger
	extType string // "torrent"
	// resolver fills cross-tracker ids (anidb/tvdb/tmdb/...) from ani.zip so the
	// Hayase options object is fully populated. Nil-safe: when absent the adapter
	// falls back to whatever ids Media already carries.
	resolver IDResolver
	// settings is the resolved per-extension settings passed as the second JS
	// method argument. May be nil (JS receives null/undefined).
	settings map[string]interface{}
	// recordHealth, when set, is invoked at the end of every Search/SmartSearch/
	// Test with the run outcome so a single centralized health record per
	// extension is updated from every execution path. Nil for preview/throwaway
	// providers that aren't installed (their health isn't persisted). Nil-safe.
	recordHealth func(extID string, healthy bool, errMsg string)
}

// SetHealthRecorder installs the centralized health-recording hook. The manager
// calls this only for providers backing INSTALLED extensions; preview providers
// keep a nil recorder (no persistence).
func (p *JSProvider) SetHealthRecorder(fn func(extID string, healthy bool, errMsg string)) {
	p.recordHealth = fn
}

// record reports a run outcome to the centralized health recorder when one is
// wired. A nil recorder (preview/throwaway providers) is a no-op.
func (p *JSProvider) record(err error) {
	if p.recordHealth == nil {
		return
	}
	if err != nil {
		p.recordHealth(p.id, false, err.Error())
		return
	}
	p.recordHealth(p.id, true, "")
}

// NewJSProvider builds a JSProvider from a JS payload string with no resolver or
// settings. Kept for existing callers/tests; delegates to NewJSProviderWithDeps.
func NewJSProvider(extID, name, payload string, httpClient *http.Client, logger *slog.Logger) (*JSProvider, error) {
	return NewJSProviderWithDeps(extID, name, payload, httpClient, nil, nil, logger)
}

// NewJSProviderWithDeps builds a JSProvider wired with an id-resolver and the
// resolved per-extension settings. httpClient backs the fetch() shim.
func NewJSProviderWithDeps(extID, name, payload string, httpClient *http.Client, resolver IDResolver, settings map[string]interface{}, logger *slog.Logger) (*JSProvider, error) {
	vm, err := NewVM(extID, payload, httpClient, logger)
	if err != nil {
		return nil, err
	}
	return &JSProvider{
		id:       extID,
		name:     name,
		vm:       vm,
		logger:   logger,
		resolver: resolver,
		settings: settings,
	}, nil
}

func (p *JSProvider) ID() string { return p.id }

// GetSettings reports the capabilities of a JS extension. Hayase extensions
// accept the full metadata-aware options object, so we advertise the rich filter
// set the adapter now supplies.
func (p *JSProvider) GetSettings() source.Settings {
	return source.Settings{
		CanSmartSearch: true,
		SmartSearchFilters: []source.SmartSearchFilter{
			source.FilterQuery,
			source.FilterEpisodeNumber,
			source.FilterBatch,
			source.FilterResolution,
			source.FilterBestReleases,
		},
		Type: source.ProviderTypeMain,
	}
}

// Search runs an unfiltered search. It builds the Hayase superset options object
// (episode 0, no batch) so extensions that only read titles/query still work,
// and tries the movie/single primary then the legacy search/smartSearch spellings.
func (p *JSProvider) Search(ctx context.Context, opts source.SearchOptions) ([]*source.AnimeTorrent, error) {
	options := p.buildOptions(ctx, source.SmartSearchOptions{Media: opts.Media, Query: opts.Query})
	res, err := p.callAndMarshal(ctx, options, p.methodChain(opts.Media, false)...)
	p.record(err)
	return res, err
}

// SmartSearch runs a metadata-aware search with the full Hayase options object.
func (p *JSProvider) SmartSearch(ctx context.Context, opts source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
	options := p.buildOptions(ctx, opts)
	res, err := p.callAndMarshal(ctx, options, p.methodChain(opts.Media, opts.Batch)...)
	p.record(err)
	return res, err
}

// Test checks the extension's upstream liveness. Hayase extensions implement a
// test() method that fetches the upstream API and throws when it's down; calling
// it and getting a clean resolution means healthy. When test() is absent the
// adapter probes with a SmartSearch for a known title (AniList 21 / One Piece,
// episode 1) and treats any non-throwing run — even zero results — as healthy.
// The outcome is recorded centrally like a real search. Returns nil when healthy,
// the underlying error when dead.
func (p *JSProvider) Test(ctx context.Context) error {
	_, err := p.vm.CallMethod(ctx, "test")
	if err != nil && strings.Contains(err.Error(), "not found or not a function") {
		// No test() — fall back to a probe SmartSearch. Don't double-record: the
		// probe path doesn't run through record() (callAndMarshal is called
		// directly), so record once here with the probe's outcome.
		options := p.buildOptions(ctx, source.SmartSearchOptions{
			Media:         source.Media{ID: 21, RomajiTitle: "One Piece"},
			EpisodeNumber: 1,
		})
		_, err = p.callAndMarshal(ctx, options, p.methodChain(source.Media{ID: 21}, false)...)
	}
	p.record(err)
	return err
}

// methodChain returns the candidate method names in priority order: the Hayase
// primary (movie|batch|single) followed by the legacy hibike spellings as
// fallbacks for older extensions. Absent methods are skipped at call time.
func (p *JSProvider) methodChain(media source.Media, batch bool) []string {
	var primary string
	switch {
	case isMovie(media):
		primary = "movie"
	case batch:
		primary = "batch"
	default:
		primary = "single"
	}
	return []string{primary, "search", "smartSearch"}
}

// isMovie reports whether the media is a movie (Hayase routes movies to movie()).
func isMovie(m source.Media) bool {
	return strings.EqualFold(m.Format, "MOVIE")
}

// buildOptions assembles the Hayase options object passed as the first JS method
// argument. When the resolver is present and an AniList id is known, it resolves
// the cross-tracker ids once (best-effort: on error it logs and proceeds with
// the ids Media already carries). Every key is always present (a 0/empty value
// is fine — extensions branch on truthiness); exclusions defaults to an empty
// slice so JS .map never sees null.
func (p *JSProvider) buildOptions(ctx context.Context, opts source.SmartSearchOptions) map[string]interface{} {
	m := opts.Media

	// Anime-level ids: start from Media, override with resolved values.
	anidbAid := m.AnidbAID
	anidbEid := opts.AnidbEid
	if anidbEid == 0 {
		anidbEid = m.AnidbEID
	}
	tvdbID := m.TvdbID
	tvdbEid := opts.TvdbEid
	tmdbID := m.TmdbID
	malID := 0
	if m.IDMal != nil {
		malID = *m.IDMal
	}

	if m.ID > 0 && p.resolver != nil {
		if ids, err := p.resolver.GetIDs(ctx, m.ID); err != nil {
			p.logger.Debug("extension: id resolve failed, using media ids", "ext", p.id, "anilist", m.ID, "err", err)
		} else {
			anidbAid = preferNonZero(ids.AnidbID, anidbAid)
			tvdbID = preferNonZero(ids.TvdbID, tvdbID)
			tmdbID = preferNonZero(ids.TmdbID, tmdbID)
			malID = preferNonZero(ids.MalID, malID)
			if ep, ok := ids.Episodes[opts.EpisodeNumber]; ok {
				anidbEid = preferNonZero(ep.AnidbEid, anidbEid)
				tvdbEid = preferNonZero(ep.TvdbEid, tvdbEid)
			}
		}
	}

	exclusions := opts.Exclusions
	if exclusions == nil {
		exclusions = []string{}
	}

	titles := m.Titles()
	if titles == nil {
		titles = []string{}
	}

	// A trimmed media object for extensions (e.g. anisearch) that read `media`.
	mediaObj := map[string]interface{}{
		"id":           m.ID,
		"titles":       titles,
		"format":       m.Format,
		"status":       m.Status,
		"episodeCount": m.EpisodeCount,
	}

	return map[string]interface{}{
		"anilistId":    m.ID,
		"anidbAid":     anidbAid,
		"anidbEid":     anidbEid,
		"malId":        malID,
		"tvdbId":       tvdbID,
		"tvdbEId":      tvdbEid, // capital E then Id — matches Hayase exactly.
		"tmdbId":       tmdbID,
		"titles":       titles,
		"episode":      opts.EpisodeNumber,
		"episodeCount": m.EpisodeCount,
		"resolution":   opts.Resolution,
		"exclusions":   exclusions,
		"batch":        opts.Batch,
		"media":        mediaObj,
		"query":        opts.Query,
	}
}

// preferNonZero returns primary when it's non-zero, else fallback.
func preferNonZero(primary, fallback int) int {
	if primary != 0 {
		return primary
	}
	return fallback
}

// GetLatest calls "getLatest" if present; returns empty on absence.
func (p *JSProvider) GetLatest(ctx context.Context) ([]*source.AnimeTorrent, error) {
	raw, err := p.vm.CallMethod(ctx, "getLatest")
	if err != nil {
		if strings.Contains(err.Error(), "not found or not a function") {
			return nil, nil
		}
		return nil, err
	}
	return marshalTorrents(p.id, raw)
}

// GetTorrentMagnetLink returns the magnet link for a torrent. For JS extensions
// the magnet is embedded in the AnimeTorrent (Hayase sets link = magnet URI), so
// we return it directly.
func (p *JSProvider) GetTorrentMagnetLink(_ context.Context, t *source.AnimeTorrent) (string, error) {
	if t.Magnet != "" {
		return t.Magnet, nil
	}
	if strings.HasPrefix(t.Link, "magnet:") {
		return t.Link, nil
	}
	return "", fmt.Errorf("extension %s: no magnet for torrent %q", p.id, t.Name)
}

// GetTorrentInfoHash returns the info hash.
func (p *JSProvider) GetTorrentInfoHash(_ context.Context, t *source.AnimeTorrent) (string, error) {
	if t.InfoHash != "" {
		return t.InfoHash, nil
	}
	return "", fmt.Errorf("extension %s: no info hash for torrent %q", p.id, t.Name)
}

// callAndMarshal calls each candidate method (in order) with (options, settings)
// until one runs. An absent method ("not found or not a function") is skipped to
// the next candidate; a method that runs and rejects returns its error directly
// rather than masking it by falling through to a differently-shaped method. If no
// candidate exists at all, the "none of methods" error is returned.
func (p *JSProvider) callAndMarshal(ctx context.Context, options interface{}, methods ...string) ([]*source.AnimeTorrent, error) {
	seen := map[string]bool{}
	tried := make([]string, 0, len(methods))
	for _, m := range methods {
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		tried = append(tried, m)
		raw, err := p.vm.CallMethod(ctx, m, options, p.settings)
		if err != nil {
			if strings.Contains(err.Error(), "not found or not a function") {
				continue
			}
			return nil, err
		}
		return marshalTorrents(p.id, raw)
	}
	return nil, fmt.Errorf("extension %s: none of methods %v found", p.id, tried)
}

// hayaseTorrent is the intermediate shape Hayase JS extensions return. Field
// names match what Hayase-compatible torrent extensions produce.
type hayaseTorrent struct {
	Title     string      `json:"title"`
	Link      string      `json:"link"`
	Hash      string      `json:"hash"`
	Seeders   json.Number `json:"seeders"`
	Leechers  json.Number `json:"leechers"`
	Downloads json.Number `json:"downloads"`
	Size      json.Number `json:"size"`
	Date      interface{} `json:"date"` // JS Date or string
	// hibike-compatible aliases (in case the ext uses the hibike shape).
	Name         string `json:"name"`
	MagnetLink   string `json:"magnetLink"`
	InfoHash     string `json:"infoHash"`
	Resolution   string `json:"resolution"`
	ReleaseGroup string `json:"releaseGroup"`
}

// marshalTorrents converts the raw JS output ([]interface{} of maps) to
// []*source.AnimeTorrent by round-tripping through JSON so field-name differences
// are handled in the struct tags.
func marshalTorrents(providerID string, raw interface{}) ([]*source.AnimeTorrent, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal JS result: %w", err)
	}

	var items []hayaseTorrent
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("unmarshal JS result: %w", err)
	}

	out := make([]*source.AnimeTorrent, 0, len(items))
	for _, it := range items {
		t := &source.AnimeTorrent{Provider: providerID}

		// Normalise: prefer hibike field names, fall back to Hayase.
		t.Name = firstNonEmpty(it.Name, it.Title)
		t.Magnet = firstNonEmpty(it.MagnetLink, "")
		t.Link = it.Link
		t.InfoHash = firstNonEmpty(it.InfoHash, it.Hash)
		t.Resolution = it.Resolution
		t.ReleaseGroup = it.ReleaseGroup

		// If link is a magnet URI, move it to Magnet.
		if strings.HasPrefix(t.Link, "magnet:") && t.Magnet == "" {
			t.Magnet = t.Link
		}

		if s, err := it.Seeders.Int64(); err == nil {
			t.Seeders = int(s)
		} else {
			t.Seeders = -1
		}

		// Backfill release-group/resolution/episode/info-hash from the name via
		// habari for anything the JS left empty, and ensure a usable magnet.
		source.Enrich(t)

		out = append(out, t)
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
