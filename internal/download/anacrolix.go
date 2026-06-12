package download

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
	"github.com/anacrolix/torrent/types"
)

// videoExts is the set of container extensions we treat as the "video" file when
// picking the largest source out of a multi-file torrent. A data set, not a
// switch — extend by adding an entry.
var videoExts = map[string]struct{}{
	".mkv": {}, ".mp4": {}, ".avi": {}, ".m4v": {},
	".mov": {}, ".ts": {}, ".webm": {}, ".flv": {}, ".wmv": {},
}

// gotInfoTimeout bounds how long Add waits for torrent metadata (the magnet ->
// info resolution) before giving up.
const gotInfoTimeout = 2 * time.Minute

// maxDownloadBytes caps the declared size of a single downloaded file. An anime
// episode is at most a few GiB even at high bitrate; 64 GiB leaves headroom for
// remuxes/movies while refusing a magnet that declares an absurd size to fill
// the disk. A package constant (not a config field) until a settings seam exists.
const maxDownloadBytes = 64 << 30

// embeddedBackend wraps a single anacrolix/torrent client. It supports N
// concurrent downloads (one *Torrent each) and never seeds: the client is
// configured with Seed=false + NoUpload=true, and each download is Dropped on
// completion while its files are left on disk for the encoder.
type embeddedBackend struct {
	client *atorrent.Client
	root   string
}

// newEmbeddedBackend builds the embedded anacrolix backend. Files land under
// {DownloadRoot}/{infohash}/ via storage.NewFileByInfoHash, so concurrent
// torrents never collide and a per-episode dir is implicit.
func newEmbeddedBackend(_ context.Context, cfg Config) (Backend, error) {
	ac := atorrent.NewDefaultClientConfig()
	ac.DataDir = cfg.DownloadRoot
	ac.DefaultStorage = storage.NewFileByInfoHash(cfg.DownloadRoot)
	// Download-to-archive, not stream-and-seed: never upload, never seed.
	ac.Seed = false
	ac.NoUpload = true
	ac.DisableAggressiveUpload = true
	// Quietly drop the library's default logger; the queue logs at its level.
	ac.Debug = false

	client, err := atorrent.NewClient(ac)
	if err != nil {
		return nil, fmt.Errorf("new anacrolix client: %w", err)
	}
	return &embeddedBackend{client: client, root: cfg.DownloadRoot}, nil
}

func (b *embeddedBackend) Kind() string { return KindEmbedded }

// Add resolves the request to a magnet/infohash, adds it to the client, waits
// for info, selects the largest video file, and starts the download. The
// returned handle tracks completion in a goroutine.
func (b *embeddedBackend) Add(ctx context.Context, req Request) (Handle, error) {
	magnet, err := magnetFor(req)
	if err != nil {
		return nil, err
	}

	t, err := b.client.AddMagnet(magnet)
	if err != nil {
		return nil, fmt.Errorf("add magnet: %w", err)
	}

	// Wait for metadata (info dict) so we know the file list and total size.
	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		t.Drop()
		return nil, ctx.Err()
	case <-time.After(gotInfoTimeout):
		t.Drop()
		return nil, fmt.Errorf("timed out waiting for torrent metadata after %s", gotInfoTimeout)
	}

	primary := largestVideoFile(t)
	if primary == nil {
		t.Drop()
		return nil, fmt.Errorf("torrent %q has no downloadable files", t.Name())
	}

	// Reject before downloading a single byte: a declared file larger than the
	// ceiling, or one that won't fit on the download volume, is a disk-fill DoS.
	size := primary.Length()
	if size > maxDownloadBytes {
		t.Drop()
		return nil, fmt.Errorf("torrent file is %d bytes, exceeds max download size %d", size, maxDownloadBytes)
	}
	if free, err := freeDiskBytes(b.root); err == nil && size > free {
		t.Drop()
		return nil, fmt.Errorf("torrent file is %d bytes, only %d free on download volume", size, free)
	}

	// Download only the chosen file: set every file to None, raise the primary.
	for _, f := range t.Files() {
		f.SetPriority(types.PiecePriorityNone)
	}
	primary.SetPriority(types.PiecePriorityNormal)
	primary.Download()

	sourcePath, err := safeSourcePath(b.root, t.InfoHash().HexString(), primary.Path())
	if err != nil {
		t.Drop()
		return nil, err
	}

	h := &embeddedHandle{
		backend:    b,
		torrent:    t,
		total:      primary.Length(),
		sourcePath: sourcePath,
		done:       make(chan struct{}),
		lastSample: time.Now(),
	}
	go h.watch(ctx, primary)
	return h, nil
}

// Close closes the underlying client, which drops all torrents.
func (b *embeddedBackend) Close() error {
	errs := b.client.Close()
	if len(errs) > 0 {
		return fmt.Errorf("close anacrolix client: %w", errs[0])
	}
	return nil
}

// safeSourcePath derives the on-disk path of the primary file under
// {root}/{infohash}, using the same primitive the anacrolix file storage uses to
// place data (storage.ToSafeFilePath) so the two computations can't drift. File
// path components come from the (untrusted) torrent info dict; ToSafeFilePath
// rejects any that escape into a parent directory, and we additionally assert
// the joined result stays within root before recording it. fileComponents is
// File.Path() — already "name/.../file" with components joined by '/', matching
// the storage layer's {infohash dir}/{name}/{file} layout.
func safeSourcePath(root, infohashHex, fileComponents string) (string, error) {
	rel, err := storage.ToSafeFilePath(filepath.FromSlash(fileComponents))
	if err != nil {
		return "", fmt.Errorf("unsafe torrent file path %q: %w", fileComponents, err)
	}
	full := filepath.Join(root, infohashHex, rel)
	if !isSubpath(root, full) {
		return "", fmt.Errorf("torrent file path %q escapes download root", fileComponents)
	}
	return full, nil
}

// isSubpath reports whether sub is base or lies beneath it, mirroring anacrolix's
// own storage.isSubFilepath guard (filepath.Rel + ".." prefix check).
func isSubpath(base, sub string) bool {
	rel, err := filepath.Rel(base, sub)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// magnetFor derives a magnet URI from the request. A bare info hash is wrapped
// into a magnet so AddMagnet can take it. TorrentURL is treated as a webseed/
// xs source on the magnet.
func magnetFor(req Request) (string, error) {
	switch {
	case strings.TrimSpace(req.Magnet) != "":
		return req.Magnet, nil
	case strings.TrimSpace(req.InfoHash) != "":
		return "magnet:?xt=urn:btih:" + strings.TrimSpace(req.InfoHash), nil
	case strings.TrimSpace(req.TorrentURL) != "":
		// anacrolix accepts the .torrent URL as a magnet exact-source param.
		return "magnet:?xs=" + req.TorrentURL, nil
	default:
		return "", fmt.Errorf("request has no magnet, infohash, or torrent URL")
	}
}

// largestVideoFile returns the biggest file with a known video extension, or the
// biggest file overall if none match (covers single-file torrents and odd names).
func largestVideoFile(t *atorrent.Torrent) *atorrent.File {
	var best, biggestAny *atorrent.File
	for _, f := range t.Files() {
		if biggestAny == nil || f.Length() > biggestAny.Length() {
			biggestAny = f
		}
		ext := strings.ToLower(filepath.Ext(f.Path()))
		if _, ok := videoExts[ext]; ok {
			if best == nil || f.Length() > best.Length() {
				best = f
			}
		}
	}
	if best != nil {
		return best
	}
	return biggestAny
}

// embeddedHandle is the per-download handle over one anacrolix torrent + the
// selected file.
type embeddedHandle struct {
	backend    *embeddedBackend
	torrent    *atorrent.Torrent
	total      int64
	sourcePath string

	done     chan struct{}
	doneOnce sync.Once

	mu         sync.Mutex
	bytesDone  int64
	peers      int
	speedBps   int64
	lastBytes  int64
	lastSample time.Time
	err        error
	removed    bool
}

// watch polls the torrent until the selected file completes, the context is
// cancelled, or the handle is removed. It updates the progress snapshot and,
// on completion, signals Done. It does NOT drop the torrent here: the queue
// calls Remove(stopSeed=true) after recording success, which keeps the file on
// disk for the encoder while stopping the (already-disabled) upload.
func (h *embeddedHandle) watch(ctx context.Context, file *atorrent.File) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			h.fail(ctx.Err())
			return
		case <-h.torrent.Closed():
			h.fail(fmt.Errorf("torrent dropped before completion"))
			return
		case <-ticker.C:
			if h.sample(file) {
				h.complete()
				return
			}
		}
	}
}

// sample updates the progress snapshot and returns true when the file is done.
func (h *embeddedHandle) sample(file *atorrent.File) bool {
	done := file.BytesCompleted()
	stats := h.torrent.Stats()
	now := time.Now()

	h.mu.Lock()
	if h.removed {
		h.mu.Unlock()
		return false
	}
	elapsed := now.Sub(h.lastSample).Seconds()
	if elapsed > 0 {
		h.speedBps = int64(float64(done-h.lastBytes) / elapsed)
		if h.speedBps < 0 {
			h.speedBps = 0
		}
	}
	h.lastBytes = done
	h.lastSample = now
	h.bytesDone = done
	h.peers = stats.ActivePeers
	complete := h.total > 0 && done >= h.total
	h.mu.Unlock()
	return complete
}

func (h *embeddedHandle) complete() {
	h.doneOnce.Do(func() {
		h.mu.Lock()
		h.bytesDone = h.total
		h.mu.Unlock()
		close(h.done)
	})
}

func (h *embeddedHandle) fail(err error) {
	h.doneOnce.Do(func() {
		h.mu.Lock()
		h.err = err
		h.mu.Unlock()
		// Drop so a failed/cancelled download stops consuming peers immediately.
		h.torrent.Drop()
		// done stays open: callers distinguish completion (Done closed) from
		// failure (Err non-nil) by checking Err after Done or after Remove.
		close(h.done)
	})
}

func (h *embeddedHandle) Progress() Progress {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Progress{
		BytesDone:  h.bytesDone,
		BytesTotal: h.total,
		Peers:      h.peers,
		SpeedBps:   h.speedBps,
		Done:       h.total > 0 && h.bytesDone >= h.total && h.err == nil,
	}
}

func (h *embeddedHandle) Done() <-chan struct{} { return h.done }

func (h *embeddedHandle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

func (h *embeddedHandle) SourcePath() string { return h.sourcePath }

func (h *embeddedHandle) SourceSize() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.total
}

// Remove tears the download down. stopSeed drops the torrent from the client
// (anacrolix already never uploads, but Drop also frees peers/goroutines).
// deleteData removes the on-disk files — Phase 4 always passes false so the
// encoder keeps the source; Phase 5 cleanup passes true.
func (h *embeddedHandle) Remove(stopSeed, deleteData bool) error {
	h.mu.Lock()
	if h.removed {
		h.mu.Unlock()
		return nil
	}
	h.removed = true
	h.mu.Unlock()

	if stopSeed {
		h.torrent.Drop()
	}
	if deleteData {
		dir := filepath.Join(h.backend.root, h.torrent.InfoHash().HexString())
		if err := removeAll(dir); err != nil {
			return fmt.Errorf("delete torrent data: %w", err)
		}
	}
	return nil
}
