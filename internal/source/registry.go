package source

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds the set of available providers keyed by id. It is the single
// lookup point for "give me the provider for this feed" — a data map, not a
// switch, so adding a provider is one Register call.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry builds an empty registry; providers are added by the extension
// manager as JS extensions install/enable.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds or replaces a provider under its id.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
}

// Unregister removes the provider registered under id, if any. Safe to call
// when no provider is registered under id.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, id)
}

// Get returns the provider registered under id.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// MustGet returns the provider under id or an error naming it.
func (r *Registry) MustGet(id string) (Provider, error) {
	if p, ok := r.Get(id); ok {
		return p, nil
	}
	return nil, fmt.Errorf("source: no provider registered for %q", id)
}

// List returns every provider id, sorted, for diagnostics and UI.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
