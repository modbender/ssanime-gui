package download

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/store"
)

// fakeBackend is a network-free Backend for unit tests. It hands out fakeHandles
// whose outcome (succeed / fail / how many bytes) is scripted per call.
type fakeBackend struct {
	adds   int32
	script func(call int, req Request) *fakeHandle
}

func (b *fakeBackend) Kind() string { return KindEmbedded }

func (b *fakeBackend) Add(_ context.Context, req Request) (Handle, error) {
	n := atomic.AddInt32(&b.adds, 1)
	h := b.script(int(n), req)
	if h == nil {
		return nil, errors.New("fake: scripted add error")
	}
	h.start()
	return h, nil
}

func (b *fakeBackend) Close() error { return nil }

// fakeHandle completes (or fails) after a short delay, recording Remove calls so
// tests can assert the no-seed / keep-data contract.
type fakeHandle struct {
	total    int64
	failWith error
	path     string
	done     chan struct{}

	mu         sync.Mutex
	removed    bool
	removeStop bool
	removeData bool
	bytesDone  int64
}

func (h *fakeHandle) start() {
	go func() {
		// Emit a couple of progress steps, then finish.
		for i := 1; i <= 2; i++ {
			time.Sleep(2 * time.Millisecond)
			h.mu.Lock()
			h.bytesDone = h.total * int64(i) / 2
			h.mu.Unlock()
		}
		close(h.done)
	}()
}

func (h *fakeHandle) Progress() Progress {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Progress{BytesDone: h.bytesDone, BytesTotal: h.total, Peers: 3, SpeedBps: 1000}
}
func (h *fakeHandle) Done() <-chan struct{} { return h.done }
func (h *fakeHandle) Err() error            { return h.failWith }
func (h *fakeHandle) SourcePath() string    { return h.path }
func (h *fakeHandle) SourceSize() int64     { return h.total }
func (h *fakeHandle) Remove(stopSeed, deleteData bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removed = true
	h.removeStop = stopSeed
	h.removeData = deleteData
	return nil
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: filepath.Join(dir, "test.db"), Port: config.DefaultPort}
	s, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// seedEpisode creates a series and a queued torrent episode, returning its id.
func seedEpisode(t *testing.T, st *store.Store, magnet string) int64 {
	t.Helper()
	ctx := context.Background()
	series, err := st.Write().CreateSeries(ctx, store.CreateSeriesParams{
		Uuid: "s-" + magnet, Title: "Series " + magnet, SeasonNumber: 1,
	})
	if err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}
	mag := magnet
	ep, err := st.Write().CreateEpisode(ctx, store.CreateEpisodeParams{
		Uuid: "e-" + magnet, SeriesID: series.ID, SourceKind: "torrent",
		Magnet: &mag, Status: "queued",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	return ep.ID
}

// newTestQueue wires a queue whose backend is the supplied fake (registered under
// the seeded embedded kind so resolveBackend selects it).
func newTestQueue(t *testing.T, st *store.Store, fb *fakeBackend) *Queue {
	t.Helper()
	reg := NewRegistry()
	reg.Register(KindEmbedded, func(_ context.Context, _ Config) (Backend, error) { return fb, nil })
	return New(st, reg, nil, Options{Workers: 2})
}

func waitForStatus(t *testing.T, st *store.Store, id int64, want string, timeout time.Duration) store.Episode {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ep, err := st.Read().GetEpisode(context.Background(), id)
		if err != nil {
			t.Fatalf("GetEpisode: %v", err)
		}
		if ep.Status == want {
			return ep
		}
		time.Sleep(5 * time.Millisecond)
	}
	ep, _ := st.Read().GetEpisode(context.Background(), id)
	t.Fatalf("episode %d status = %q, want %q (after %s)", id, ep.Status, want, timeout)
	return store.Episode{}
}

func TestQueueDownloadsToCompletion(t *testing.T) {
	st := openTestStore(t)
	id := seedEpisode(t, st, "abc")

	var h *fakeHandle
	fb := &fakeBackend{script: func(_ int, _ Request) *fakeHandle {
		h = &fakeHandle{total: 1000, path: "/data/downloads/abc/video.mkv", done: make(chan struct{})}
		return h
	}}
	q := newTestQueue(t, st, fb)
	q.Start()
	defer q.Stop()

	ep := waitForStatus(t, st, id, "downloaded", 3*time.Second)
	if ep.SourcePath == nil || *ep.SourcePath != filepath.Clean("/data/downloads/abc/video.mkv") {
		t.Errorf("source_path = %v, want the downloaded file", ep.SourcePath)
	}
	if ep.SourceSize == nil || *ep.SourceSize != 1000 {
		t.Errorf("source_size = %v, want 1000", ep.SourceSize)
	}
	// No-seed + keep-data contract: Remove(stopSeed=true, deleteData=false).
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.removed || !h.removeStop || h.removeData {
		t.Errorf("Remove(stop=%v,data=%v), want stop=true data=false", h.removeStop, h.removeData)
	}
}

func TestQueueRetriesThenErrors(t *testing.T) {
	st := openTestStore(t)
	id := seedEpisode(t, st, "fail")

	fb := &fakeBackend{script: func(_ int, _ Request) *fakeHandle {
		return &fakeHandle{total: 500, failWith: errors.New("no peers"), done: make(chan struct{})}
	}}
	q := newTestQueue(t, st, fb)
	q.Start()
	defer q.Stop()

	ep := waitForStatus(t, st, id, "error", 5*time.Second)
	if ep.ErrorMessage == nil || *ep.ErrorMessage != "no peers" {
		t.Errorf("error_message = %v, want 'no peers'", ep.ErrorMessage)
	}
	if ep.RetryCount != maxRetries {
		t.Errorf("retry_count = %d, want %d", ep.RetryCount, maxRetries)
	}
	if got := atomic.LoadInt32(&fb.adds); got != int32(maxRetries) {
		t.Errorf("backend.Add called %d times, want %d (bounded retry)", got, maxRetries)
	}
}

func TestQueueRecoversAfterTransientFailure(t *testing.T) {
	st := openTestStore(t)
	id := seedEpisode(t, st, "flaky")

	fb := &fakeBackend{script: func(call int, _ Request) *fakeHandle {
		if call == 1 {
			return &fakeHandle{total: 100, failWith: errors.New("blip"), done: make(chan struct{})}
		}
		return &fakeHandle{total: 100, path: "/d/flaky/v.mkv", done: make(chan struct{})}
	}}
	q := newTestQueue(t, st, fb)
	q.Start()
	defer q.Stop()

	ep := waitForStatus(t, st, id, "downloaded", 5*time.Second)
	if ep.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1 (one failure then success)", ep.RetryCount)
	}
	if ep.ErrorMessage != nil {
		t.Errorf("error_message = %v, want nil after recovery", ep.ErrorMessage)
	}
}

func TestRegistryDispatchesByKind(t *testing.T) {
	reg := NewRegistry()
	kinds := reg.Kinds()
	want := map[string]bool{KindEmbedded: false, KindQBittorrent: false, KindTransmission: false}
	for _, k := range kinds {
		if _, ok := want[k]; ok {
			want[k] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("registry missing builtin backend kind %q", k)
		}
	}

	var built int32
	reg.Register("fake", func(_ context.Context, _ Config) (Backend, error) {
		atomic.AddInt32(&built, 1)
		return &fakeBackend{}, nil
	})
	ctx := context.Background()
	cfg := Config{ClientID: 7, Kind: "fake"}
	b1, err := reg.Backend(ctx, cfg)
	if err != nil {
		t.Fatalf("Backend: %v", err)
	}
	b2, err := reg.Backend(ctx, cfg)
	if err != nil {
		t.Fatalf("Backend(2): %v", err)
	}
	if b1 != b2 {
		t.Error("registry should cache one backend per client id")
	}
	if built != 1 {
		t.Errorf("factory called %d times, want 1 (cached)", built)
	}
}

func TestInfoHashFromMagnet(t *testing.T) {
	const ih = "c9e15763f722f23e98a29decdfae341b98d53056"
	got, err := infoHashFor(Request{Magnet: "magnet:?xt=urn:btih:" + ih})
	if err != nil {
		t.Fatalf("infoHashFor: %v", err)
	}
	if got != ih {
		t.Errorf("infoHash = %q, want %q", got, ih)
	}
}

func TestLargestVideoFileSelectionUnit(t *testing.T) {
	// Pure helper test of the extension/size ranking without anacrolix types:
	// exercised indirectly here via videoExts membership.
	for _, ext := range []string{".mkv", ".mp4", ".avi"} {
		if _, ok := videoExts[ext]; !ok {
			t.Errorf("expected %q to be a recognized video extension", ext)
		}
	}
	if _, ok := videoExts[".txt"]; ok {
		t.Error(".txt should not be a video extension")
	}
}
