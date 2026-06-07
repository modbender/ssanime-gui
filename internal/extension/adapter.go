package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modbender/ssanime-gui/internal/source"
)

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
}

// NewJSProvider builds a JSProvider from a JS payload string.
// httpClient is used for the fetch() shim; pass the DoH client for nyaa.
func NewJSProvider(extID, name, payload string, httpClient *http.Client, logger *slog.Logger) (*JSProvider, error) {
	vm, err := NewVM(extID, payload, httpClient, logger)
	if err != nil {
		return nil, err
	}
	return &JSProvider{
		id:     extID,
		name:   name,
		vm:     vm,
		logger: logger,
	}, nil
}

func (p *JSProvider) ID() string { return p.id }

// GetSettings returns a basic Settings for JS extensions. JS extensions in the
// Hayase ecosystem don't expose a getSettings() method; we provide sensible
// defaults so the registry treats them uniformly with native providers.
func (p *JSProvider) GetSettings() source.Settings {
	return source.Settings{
		CanSmartSearch:     true,
		SmartSearchFilters: []source.SmartSearchFilter{source.FilterQuery, source.FilterEpisodeNumber},
		Type:               source.ProviderTypeMain,
	}
}

// Search calls the JS extension's "single" method (Hayase convention) with a
// map{titles, episode}. If the extension exposes a "search" method that takes
// (title, episode) we fall back to that.
func (p *JSProvider) Search(ctx context.Context, opts source.SearchOptions) ([]*source.AnimeTorrent, error) {
	args := map[string]interface{}{
		"titles":  opts.Media.Titles(),
		"episode": 0,
		"query":   opts.Query,
	}
	return p.callAndMarshal(ctx, args, "single", "search")
}

// SmartSearch calls "single" with episode filtering.
func (p *JSProvider) SmartSearch(ctx context.Context, opts source.SmartSearchOptions) ([]*source.AnimeTorrent, error) {
	args := map[string]interface{}{
		"titles":  opts.Media.Titles(),
		"episode": opts.EpisodeNumber,
		"query":   opts.Query,
		"batch":   opts.Batch,
	}
	return p.callAndMarshal(ctx, args, "single", "search", "smartSearch")
}

// GetLatest calls "getLatest" or "latest" if present; returns empty on absence.
func (p *JSProvider) GetLatest(ctx context.Context) ([]*source.AnimeTorrent, error) {
	raw, err := p.vm.CallMethod(ctx, "getLatest")
	if err != nil {
		// Extension may not implement getLatest — return empty, not error.
		if strings.Contains(err.Error(), "not found or not a function") {
			return nil, nil
		}
		return nil, err
	}
	return marshalTorrents(p.id, raw)
}

// GetTorrentMagnetLink returns the magnet link for a torrent. For JS
// extensions the magnet is embedded in the AnimeTorrent (Hayase sets
// link = magnet URI), so we return it directly.
func (p *JSProvider) GetTorrentMagnetLink(_ context.Context, t *source.AnimeTorrent) (string, error) {
	if t.Magnet != "" {
		return t.Magnet, nil
	}
	// Some extensions put the magnet in Link.
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

// callAndMarshal tries each method name in order, returning on the first that
// exists on the JS object.
func (p *JSProvider) callAndMarshal(ctx context.Context, args interface{}, methods ...string) ([]*source.AnimeTorrent, error) {
	var lastErr error
	for _, m := range methods {
		raw, err := p.vm.CallMethod(ctx, m, args)
		if err != nil {
			if strings.Contains(err.Error(), "not found or not a function") {
				continue
			}
			lastErr = err
			continue
		}
		return marshalTorrents(p.id, raw)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("extension %s: none of methods %v found", p.id, methods)
}

// hayaseTorrent is the intermediate shape Hayase JS extensions return.
// Field names match what nyaa.js and sukebei.js produce.
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
// []*source.AnimeTorrent by round-tripping through JSON so field-name
// differences are handled in the struct tags.
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
		t.EpisodeNumber = -1

		// If link is a magnet URI, move it to Magnet.
		if strings.HasPrefix(t.Link, "magnet:") && t.Magnet == "" {
			t.Magnet = t.Link
		}

		if s, err := it.Seeders.Int64(); err == nil {
			t.Seeders = int(s)
		} else {
			t.Seeders = -1
		}

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
