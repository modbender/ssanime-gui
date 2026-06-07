package source

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/modbender/ssanime-gui/internal/doh"
)

// fetchTimeout bounds a single provider HTTP fetch.
const fetchTimeout = 25 * time.Second

// Registry holds the set of available providers keyed by id. It is the single
// lookup point for "give me the provider for this feed" — a data map, not a
// switch, so adding a provider is one Register call.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider

	// directClient reaches hosts that are not DNS-blocked (e.g. subsplease.org).
	directClient *http.Client
	// dohClient resolves via DNS-over-HTTPS to defeat the ISP block on nyaa.si.
	dohClient *http.Client
}

// NewRegistry builds a registry wired with a DoH-backed HTTP client (for nyaa)
// and a direct client (for reachable hosts), then registers the native
// providers. resolver may be nil, in which case both clients fall back to the
// default transport (useful in tests).
func NewRegistry(resolver *doh.Resolver) *Registry {
	r := &Registry{
		providers:    make(map[string]Provider),
		directClient: &http.Client{Timeout: fetchTimeout},
	}
	if resolver != nil {
		r.dohClient = resolver.HTTPClient(fetchTimeout)
	} else {
		r.dohClient = &http.Client{Timeout: fetchTimeout}
	}

	// Native builtin providers. Adding a provider is one line here.
	r.Register(NewSubsPlease(r.directClient))
	r.Register(NewNyaa(r.dohClient))
	return r
}

// Register adds or replaces a provider under its id.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
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
