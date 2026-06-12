package download

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Registry maps a download_clients.kind to the Factory that builds its backend.
// It is the data-map seam the spec requires: adding a backend is one Register
// call, never a new switch case. Built backends are cached per ClientID so one
// embedded anacrolix client (or one external session) is shared across all
// downloads routed to that client row.
type Registry struct {
	mu        sync.Mutex
	factories map[string]Factory
	built     map[int64]Backend
}

// NewRegistry returns a registry seeded with the built-in backends (embedded,
// qbittorrent, transmission). Tests can construct an empty one with
// &Registry{...} and Register a fake.
func NewRegistry() *Registry {
	r := &Registry{
		factories: make(map[string]Factory),
		built:     make(map[int64]Backend),
	}
	for kind, f := range builtinFactories {
		r.factories[kind] = f
	}
	return r
}

// builtinFactories is the data list of shipped backends. One entry per kind.
var builtinFactories = map[string]Factory{
	KindEmbedded:     newEmbeddedBackend,
	KindQBittorrent:  newQBittorrentBackend,
	KindTransmission: newTransmissionBackend,
}

// download_clients.kind values.
const (
	KindEmbedded     = "embedded"
	KindQBittorrent  = "qbittorrent"
	KindTransmission = "transmission"
)

// Register adds or overrides the factory for a kind. Used by tests to inject a
// fake backend under a custom kind.
func (r *Registry) Register(kind string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[kind] = f
}

// Kinds returns the registered kinds, sorted, for diagnostics.
func (r *Registry) Kinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.factories))
	for k := range r.factories {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Backend returns the backend for cfg, building it once per ClientID and caching
// it. Subsequent calls for the same ClientID return the cached instance.
func (r *Registry) Backend(ctx context.Context, cfg Config) (Backend, error) {
	r.mu.Lock()
	if b, ok := r.built[cfg.ClientID]; ok {
		r.mu.Unlock()
		return b, nil
	}
	f, ok := r.factories[cfg.Kind]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("download: no backend registered for kind %q", cfg.Kind)
	}

	b, err := f(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("download: build %s backend: %w", cfg.Kind, err)
	}

	r.mu.Lock()
	// Another goroutine may have built it first; prefer the existing one.
	if existing, ok := r.built[cfg.ClientID]; ok {
		r.mu.Unlock()
		_ = b.Close()
		return existing, nil
	}
	r.built[cfg.ClientID] = b
	r.mu.Unlock()
	return b, nil
}

// Close closes every built backend and clears the cache. Called on shutdown.
func (r *Registry) Close() error {
	r.mu.Lock()
	built := r.built
	r.built = make(map[int64]Backend)
	r.mu.Unlock()

	var firstErr error
	for _, b := range built {
		if err := b.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
