package download

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

// maxRetries bounds how many times a failed episode is requeued before it is
// parked in the error state. retry_count is durable so the bound survives
// restarts.
const maxRetries = 3

// scanInterval is how often the queue scans the DB for newly queued episodes.
// New episodes (from the poller) are picked up within this window even if no
// worker just finished.
const scanInterval = 5 * time.Second

// Store is the narrow store surface the queue needs (kept small for fakes).
type Store interface {
	Read() *store.Queries
	Write() *store.Queries
	WriteTx(ctx context.Context, fn func(*store.Queries) error) error
}

// Queue is the download stage's worker pool. It pulls queued episodes, claims
// each by transitioning queued->downloading inside a write tx (so two workers
// never grab the same row), runs the selected backend, streams progress events,
// and records the terminal state (downloaded, or error after bounded retry).
type Queue struct {
	store    Store
	registry *Registry
	hub      *events.Hub
	logger   *slog.Logger

	// backendFor resolves the backend for an episode. Defaulted to the registry-
	// backed resolver; overridable in tests to inject a fake backend.
	backendFor func(ctx context.Context, ep store.Episode) (Backend, error)

	workers int

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// claimMu serializes the claim transaction so the pool's workers don't race
	// to grab the same queued row under SQLite's single writer.
	claimMu sync.Mutex

	// paused is an atomic flag; workers check it before claiming new work.
	// In-flight downloads are not interrupted — only new claims are blocked.
	paused atomic.Bool
}

// Options configure a Queue.
type Options struct {
	// Workers overrides the pool size (default: settings.concurrency_download,
	// floored at 1).
	Workers int
	// Logger overrides the logger.
	Logger *slog.Logger
}

// New builds a download queue. workers is normally settings.concurrency_download.
func New(st Store, registry *Registry, hub *events.Hub, opts Options) *Queue {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	workers := opts.Workers
	if workers < 1 {
		workers = 1
	}
	q := &Queue{
		store:    st,
		registry: registry,
		hub:      hub,
		logger:   logger,
		workers:  workers,
	}
	q.backendFor = q.resolveBackend
	return q
}

// Start launches the worker pool. Safe to call once; Stop ends it and waits for
// in-flight downloads to unwind.
func (q *Queue) Start() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.started {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	q.cancel = cancel
	q.started = true
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(ctx, i)
	}
	q.logger.Info("download queue started", "workers", q.workers, "backends", q.registry.Kinds())
}

// Stop cancels all workers and waits for them to return. Context cancellation
// propagates to each running backend.Add ctx, so a graceful shutdown leaves
// the orphaned downloading row to be reset to queued on next boot (crash
// recovery), making the stage durable and resumable.
func (q *Queue) Stop() {
	q.mu.Lock()
	cancel, started := q.cancel, q.started
	q.started = false
	q.mu.Unlock()
	if !started {
		return
	}
	cancel()
	q.wg.Wait()
}

// Pause suspends new work claims. In-flight downloads continue to completion.
// Safe to call from any goroutine (including the systray menu handler).
func (q *Queue) Pause() {
	q.paused.Store(true)
	if q.logger != nil {
		q.logger.Info("download queue paused")
	}
}

// Resume re-enables work claims after a Pause.
func (q *Queue) Resume() {
	q.paused.Store(false)
	if q.logger != nil {
		q.logger.Info("download queue resumed")
	}
}

// Paused reports whether the queue is currently paused.
func (q *Queue) Paused() bool { return q.paused.Load() }

// worker runs the claim->download->record loop until the context is cancelled.
// Each iteration claims at most one episode; if none is queued it waits a tick.
func (q *Queue) worker(ctx context.Context, id int) {
	defer q.wg.Done()

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()
	for {
		if q.tryClaimAndProcess(ctx, id) {
			continue // immediately look for more work
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// tryClaimAndProcess claims one episode and processes it, returning whether work
// was done. Its recover is a per-iteration backstop: a panic in claim() (process
// already recovers internally) must not kill the worker and permanently shrink
// the pool — we log it and let the loop continue.
func (q *Queue) tryClaimAndProcess(ctx context.Context, id int) (worked bool) {
	defer func() {
		if r := recover(); r != nil {
			q.logger.Error("download worker iteration panic", "worker", id, "panic", r)
		}
	}()
	ep, ok := q.claim(ctx)
	if !ok {
		return false
	}
	q.process(ctx, ep)
	return true
}

// claim atomically grabs the oldest queued episode and flips it to downloading.
// The claimMu + write tx pair guarantees exclusivity across workers.
// Returns (zero, false) without touching the DB when the queue is paused.
func (q *Queue) claim(ctx context.Context) (store.Episode, bool) {
	if ctx.Err() != nil {
		return store.Episode{}, false
	}
	if q.paused.Load() {
		return store.Episode{}, false
	}
	q.claimMu.Lock()
	defer q.claimMu.Unlock()

	queued, err := q.store.Read().ListQueuedEpisodes(ctx)
	if err != nil {
		q.logger.Error("download queue: list queued", "err", err)
		return store.Episode{}, false
	}
	if len(queued) == 0 {
		return store.Episode{}, false
	}
	ep := queued[0]
	if err := q.store.Write().MarkEpisodeDownloading(ctx, ep.ID); err != nil {
		q.logger.Error("download queue: claim episode", "episode", ep.ID, "err", err)
		return store.Episode{}, false
	}
	ep.Status = "downloading"
	q.emitStatus(ep.ID, ep.SeriesID, "downloading")
	return ep, true
}

// process runs one episode through its backend to terminal state.
func (q *Queue) process(ctx context.Context, ep store.Episode) {
	defer func() {
		if r := recover(); r != nil {
			q.logger.Error("download process panic", "episode", ep.ID, "panic", r)
			q.failEpisode(ep, fmt.Errorf("panic: %v", r))
		}
	}()

	backend, err := q.backendFor(ctx, ep)
	if err != nil {
		q.failEpisode(ep, fmt.Errorf("select backend: %w", err))
		return
	}

	req, err := requestFor(ctx, q.store, ep)
	if err != nil {
		q.failEpisode(ep, err)
		return
	}

	handle, err := backend.Add(ctx, req)
	if err != nil {
		q.failEpisode(ep, fmt.Errorf("start download: %w", err))
		return
	}

	q.logger.Info("download started", "episode", ep.ID, "name", req.Name, "backend", backend.Kind())
	q.runToCompletion(ctx, ep, handle)
}

// runToCompletion polls progress until the handle finishes, then records the
// outcome. On success it stops seeding (keeps files) and marks the episode
// downloaded; on failure it removes the partial data and retries or errors.
func (q *Queue) runToCompletion(ctx context.Context, ep store.Episode, handle Handle) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Graceful shutdown: tear down the live download but leave the DB row
			// as 'downloading' so crash recovery resets it to 'queued' on reboot.
			_ = handle.Remove(true, false)
			return
		case <-ticker.C:
			q.emitProgress(ep, handle.Progress())
		case <-handle.Done():
			if err := handle.Err(); err != nil {
				_ = handle.Remove(true, true) // failed: drop torrent + partial data
				q.failEpisode(ep, err)
				return
			}
			q.succeed(ep, handle)
			return
		}
	}
}

// succeed records a completed download: stop seeding (keep files for the
// encoder), persist source_path/source_size, transition to downloaded.
func (q *Queue) succeed(ep store.Episode, handle Handle) {
	path := handle.SourcePath()
	size := handle.SourceSize()

	// Stop seeding but KEEP the data on disk — Phase 5 deletes it post-archive.
	if err := handle.Remove(true, false); err != nil {
		q.logger.Warn("download: stop-seed failed", "episode", ep.ID, "err", err)
	}

	bgCtx := context.Background()
	var pathPtr *string
	if path != "" {
		clean := filepath.Clean(path)
		pathPtr = &clean
	}
	var sizePtr *int64
	if size > 0 {
		sizePtr = &size
	}
	if err := q.store.Write().MarkEpisodeDownloaded(bgCtx, store.MarkEpisodeDownloadedParams{
		SourcePath: pathPtr,
		SourceSize: sizePtr,
		ID:         ep.ID,
	}); err != nil {
		q.logger.Error("download: mark downloaded", "episode", ep.ID, "err", err)
		q.failEpisode(ep, fmt.Errorf("record completion: %w", err))
		return
	}
	_ = q.store.Write().ClearEpisodeError(bgCtx, ep.ID)

	q.emitProgress(ep, Progress{BytesDone: size, BytesTotal: size, Done: true})
	q.emitStatus(ep.ID, ep.SeriesID, "downloaded")
	q.logger.Info("download complete", "episode", ep.ID, "path", path, "size", size)
}

// failEpisode increments retry_count; if under the bound it requeues for another
// attempt, otherwise it parks the episode in the error state with the cause.
func (q *Queue) failEpisode(ep store.Episode, cause error) {
	bgCtx := context.Background()
	if errors.Is(cause, context.Canceled) {
		// Shutdown, not a real failure: leave the row for crash recovery.
		return
	}
	msg := cause.Error()

	if err := q.store.Write().IncrementEpisodeRetry(bgCtx, ep.ID); err != nil {
		q.logger.Error("download: increment retry", "episode", ep.ID, "err", err)
	}
	attempt := ep.RetryCount + 1 // value after the increment above

	if attempt < maxRetries {
		if err := q.store.Write().SetEpisodeStatus(bgCtx, store.SetEpisodeStatusParams{
			Status: "queued", ID: ep.ID,
		}); err != nil {
			q.logger.Error("download: requeue", "episode", ep.ID, "err", err)
		}
		q.logger.Warn("download failed, requeued", "episode", ep.ID, "attempt", attempt, "err", msg)
		q.emitStatus(ep.ID, ep.SeriesID, "queued")
		return
	}

	if err := q.store.Write().SetEpisodeError(bgCtx, store.SetEpisodeErrorParams{
		ErrorMessage: &msg, ID: ep.ID,
	}); err != nil {
		q.logger.Error("download: set error", "episode", ep.ID, "err", err)
	}
	q.logger.Error("download failed permanently", "episode", ep.ID, "attempts", attempt, "err", msg)
	q.emitStatus(ep.ID, ep.SeriesID, "error")
}

// resolveBackend picks the backend for an episode: the client named by
// settings.download_backend, else the default download_clients row, and builds
// it through the registry (one instance per client id).
func (q *Queue) resolveBackend(ctx context.Context, _ store.Episode) (Backend, error) {
	set, err := q.store.Read().GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}

	var client store.DownloadClient
	if set.DownloadBackend != nil {
		client, err = q.store.Read().GetDownloadClient(ctx, *set.DownloadBackend)
		if err != nil {
			return nil, fmt.Errorf("load download client %d: %w", *set.DownloadBackend, err)
		}
	} else {
		client, err = q.store.Read().GetDefaultDownloadClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("load default download client: %w", err)
		}
	}

	cfg := Config{
		ClientID:            client.ID,
		Kind:                client.Kind,
		Name:                client.Name,
		DownloadRoot:        set.DownloadRoot,
		ConcurrencyDownload: int(set.ConcurrencyDownload),
	}
	if client.Host != nil {
		cfg.Host = *client.Host
	}
	if client.Port != nil {
		cfg.Port = int(*client.Port)
	}
	if client.Username != nil {
		cfg.Username = *client.Username
	}
	if client.Password != nil {
		cfg.Password = *client.Password
	}
	return q.registry.Backend(ctx, cfg)
}

// requestFor builds the backend Request from an episode row.
func requestFor(_ context.Context, _ Store, ep store.Episode) (Request, error) {
	req := Request{EpisodeID: ep.ID}
	if ep.Title != nil {
		req.Name = *ep.Title
	}
	if ep.Magnet != nil {
		req.Magnet = *ep.Magnet
	}
	if ep.SourceUrl != nil {
		req.TorrentURL = *ep.SourceUrl
	}
	if req.Magnet == "" && req.TorrentURL == "" {
		return Request{}, fmt.Errorf("episode %d has no magnet or source URL", ep.ID)
	}
	return req, nil
}

func (q *Queue) emitProgress(ep store.Episode, p Progress) {
	if q.hub == nil {
		return
	}
	q.hub.Broadcast(events.TypeDownloadProgress, map[string]any{
		"episode_id":  ep.ID,
		"series_id":   ep.SeriesID,
		"bytes_done":  p.BytesDone,
		"bytes_total": p.BytesTotal,
		"percent":     p.Percent(),
		"peers":       p.Peers,
		"speed_bps":   p.SpeedBps,
		"done":        p.Done,
	})
}

func (q *Queue) emitStatus(episodeID, seriesID int64, status string) {
	if q.hub == nil {
		return
	}
	q.hub.Broadcast(events.TypeEpisodeStatus, map[string]any{
		"episode_id": episodeID,
		"series_id":  seriesID,
		"status":     status,
	})
}
