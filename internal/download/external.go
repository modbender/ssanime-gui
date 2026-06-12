package download

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

// externalHandle is the shared Handle implementation for HTTP-API backends
// (qBittorrent, Transmission). The backend supplies poll/remove/sourcePath
// closures; this type owns the watch loop, progress snapshot, and completion
// signalling identically to the embedded handle.
type externalHandle struct {
	backend Backend
	hash    string

	// poll returns the current progress for the torrent identified by hash.
	poll func(ctx context.Context, hash string) (Progress, error)
	// remove deletes the torrent (deleteFiles controls on-disk data).
	remove func(ctx context.Context, hash string, deleteFiles bool) error
	// resolvePath, when set, is called once on completion to learn the primary
	// downloaded file's absolute path. May be nil (path then stays empty).
	resolvePath func(ctx context.Context, hash string) (string, int64, error)

	done     chan struct{}
	doneOnce sync.Once

	mu         sync.Mutex
	progress   Progress
	sourcePath string
	sourceSize int64
	err        error
	removed    bool
}

func (h *externalHandle) watch(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			h.fail(ctx.Err())
			return
		case <-ticker.C:
			p, err := h.poll(ctx, h.hash)
			if err != nil {
				// Transient API errors shouldn't kill the download; keep polling.
				continue
			}
			h.mu.Lock()
			if h.removed {
				h.mu.Unlock()
				return
			}
			h.progress = p
			h.mu.Unlock()
			if p.Done {
				h.complete(ctx)
				return
			}
		}
	}
}

func (h *externalHandle) complete(ctx context.Context) {
	h.doneOnce.Do(func() {
		if h.resolvePath != nil {
			if path, size, err := h.resolvePath(ctx, h.hash); err == nil {
				h.mu.Lock()
				h.sourcePath = path
				if size > 0 {
					h.sourceSize = size
				}
				h.mu.Unlock()
			}
		}
		close(h.done)
	})
}

func (h *externalHandle) fail(err error) {
	h.doneOnce.Do(func() {
		h.mu.Lock()
		h.err = err
		h.mu.Unlock()
		close(h.done)
	})
}

func (h *externalHandle) Progress() Progress {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.progress
}

func (h *externalHandle) Done() <-chan struct{} { return h.done }

func (h *externalHandle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

func (h *externalHandle) SourcePath() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sourcePath
}

func (h *externalHandle) SourceSize() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sourceSize > 0 {
		return h.sourceSize
	}
	return h.progress.BytesTotal
}

func (h *externalHandle) Remove(stopSeed, deleteData bool) error {
	h.mu.Lock()
	if h.removed {
		h.mu.Unlock()
		return nil
	}
	h.removed = true
	h.mu.Unlock()
	// stopSeed without deleteData still means "remove the torrent from the
	// external client" so it stops uploading; the files stay because deleteData
	// is false.
	if !stopSeed && !deleteData {
		return nil
	}
	return h.remove(context.Background(), h.hash, deleteData)
}

// infoHashFor extracts the bare info hash from a request: directly from
// InfoHash, or parsed out of the magnet's btih xt. External clients key
// everything on the hash, so we need it up front.
func infoHashFor(req Request) (string, error) {
	if h := strings.TrimSpace(req.InfoHash); h != "" {
		return strings.ToLower(h), nil
	}
	if m := strings.TrimSpace(req.Magnet); m != "" {
		mag, err := metainfo.ParseMagnetUri(m)
		if err != nil {
			return "", fmt.Errorf("parse magnet: %w", err)
		}
		return strings.ToLower(mag.InfoHash.HexString()), nil
	}
	return "", fmt.Errorf("cannot determine info hash: request has no infohash or magnet")
}
