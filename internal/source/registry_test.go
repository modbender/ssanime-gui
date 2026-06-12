package source

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// fakeProvider is a no-op Provider used to exercise the registry.
type fakeProvider struct{ id string }

func (f fakeProvider) ID() string { return f.id }
func (f fakeProvider) Search(context.Context, SearchOptions) ([]*AnimeTorrent, error) {
	return nil, nil
}
func (f fakeProvider) SmartSearch(context.Context, SmartSearchOptions) ([]*AnimeTorrent, error) {
	return nil, nil
}
func (f fakeProvider) GetLatest(context.Context) ([]*AnimeTorrent, error) { return nil, nil }
func (f fakeProvider) GetTorrentMagnetLink(context.Context, *AnimeTorrent) (string, error) {
	return "", nil
}
func (f fakeProvider) GetTorrentInfoHash(context.Context, *AnimeTorrent) (string, error) {
	return "", nil
}
func (f fakeProvider) GetSettings() Settings { return Settings{} }

func TestRegistryEmptyByDefault(t *testing.T) {
	r := NewRegistry()
	if got := r.List(); len(got) != 0 {
		t.Fatalf("new registry not empty: %v", got)
	}
	if _, ok := r.Get("nope"); ok {
		t.Error("Get on empty registry returned ok=true")
	}
}

func TestRegistryRegisterUnregister(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeProvider{id: "ext-a"})

	p, ok := r.Get("ext-a")
	if !ok {
		t.Fatal("Get after Register returned ok=false")
	}
	if p.ID() != "ext-a" {
		t.Errorf("provider id = %q, want ext-a", p.ID())
	}
	if list := r.List(); len(list) != 1 || list[0] != "ext-a" {
		t.Errorf("List = %v, want [ext-a]", list)
	}

	r.Unregister("ext-a")
	if _, ok := r.Get("ext-a"); ok {
		t.Error("Get after Unregister returned ok=true")
	}
	if list := r.List(); len(list) != 0 {
		t.Errorf("List after Unregister = %v, want empty", list)
	}

	// Unregister is safe when nothing is registered under the id.
	r.Unregister("ghost")
}

func TestRegistryRegisterConcurrent(t *testing.T) {
	r := NewRegistry()
	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			r.Register(fakeProvider{id: fmt.Sprintf("ext-%d", i)})
		}(i)
	}
	wg.Wait()
	if got := len(r.List()); got != n {
		t.Errorf("List length = %d, want %d", got, n)
	}
}
