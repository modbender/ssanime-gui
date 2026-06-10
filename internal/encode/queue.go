package encode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

// scanInterval is how often an idle worker rescans for newly downloaded episodes.
const scanInterval = 5 * time.Second

// Store is the narrow store surface the queue needs (kept small for fakes).
type Store interface {
	Read() *store.Queries
	Write() *store.Queries
	WriteTx(ctx context.Context, fn func(*store.Queries) error) error
}

// Queue is the encode stage's worker pool. It claims downloaded episodes, fans
// each into one encoded_outputs row per target resolution, drives every output
// through encoding->encoded->thumbnailing->archived (encode to a temp file,
// thumbnail pass, then move into the Jellyfin library path), and cleans up the
// original once every output is archived.
type Queue struct {
	store    Store
	encoder  Encoder
	resolver *ProfileResolver
	hub      *events.Hub
	logger   *slog.Logger
	dataDir  string // app-data dir; thumbnails land under <dataDir>/thumbnails

	workers int

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// claimMu serializes the claim transaction so workers don't grab the same row.
	claimMu sync.Mutex

	// paused is an atomic flag; workers check it before claiming new work.
	// In-flight encodes are not interrupted — only new claims are blocked.
	paused atomic.Bool
}

// Options configure a Queue.
type Options struct {
	// Workers overrides the pool size (default settings.concurrency_encode, >=1).
	Workers int
	// DataDir is the app-data dir for thumbnail storage.
	DataDir string
	// Logger overrides the logger.
	Logger *slog.Logger
}

// New builds an encode queue. workers is normally settings.concurrency_encode.
func New(st Store, enc Encoder, hub *events.Hub, opts Options) *Queue {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	workers := opts.Workers
	if workers < 1 {
		workers = 1
	}
	return &Queue{
		store:    st,
		encoder:  enc,
		resolver: NewProfileResolver(st),
		hub:      hub,
		logger:   logger,
		dataDir:  opts.DataDir,
		workers:  workers,
	}
}

// Start launches the worker pool. Safe to call once; Stop ends it.
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
	q.logger.Info("encode queue started", "workers", q.workers)
}

// Stop cancels all workers and waits for them to return. In-flight ffmpeg
// processes are killed via context; the half-done DB rows are reset to queued on
// next boot by crash recovery, keeping the stage durable.
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

// Pause suspends new work claims. In-flight encodes continue to completion.
// Safe to call from any goroutine (including the systray menu handler).
func (q *Queue) Pause() {
	q.paused.Store(true)
	if q.logger != nil {
		q.logger.Info("encode queue paused")
	}
}

// Resume re-enables work claims after a Pause.
func (q *Queue) Resume() {
	q.paused.Store(false)
	if q.logger != nil {
		q.logger.Info("encode queue resumed")
	}
}

// Paused reports whether the queue is currently paused.
func (q *Queue) Paused() bool { return q.paused.Load() }

func (q *Queue) worker(ctx context.Context, id int) {
	defer q.wg.Done()

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()
	for {
		if q.tryClaimAndProcess(ctx, id) {
			continue
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
			q.logger.Error("encode worker iteration panic", "worker", id, "panic", r)
		}
	}()
	ep, ok := q.claim(ctx)
	if !ok {
		return false
	}
	q.process(ctx, ep)
	return true
}

// claim atomically grabs the oldest downloaded episode and flips it to encoding.
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

	ready, err := q.store.Read().ListEpisodesByStatus(ctx, "downloaded")
	if err != nil {
		q.logger.Error("encode queue: list downloaded", "err", err)
		return store.Episode{}, false
	}
	if len(ready) == 0 {
		return store.Episode{}, false
	}
	ep := ready[0]
	if err := q.store.Write().MarkEpisodeEncoding(ctx, store.MarkEpisodeEncodingParams{ID: ep.ID}); err != nil {
		q.logger.Error("encode queue: claim episode", "episode", ep.ID, "err", err)
		return store.Episode{}, false
	}
	ep.Status = "encoding"
	q.emitStatus(ep.ID, ep.SeriesID, "encoding")
	return ep, true
}

// process runs one episode through the full fan-out: resolve profile, create the
// per-resolution outputs, encode/thumbnail/archive each, then clean up.
func (q *Queue) process(ctx context.Context, ep store.Episode) {
	defer func() {
		if r := recover(); r != nil {
			q.logger.Error("encode process panic", "episode", ep.ID, "panic", r)
			q.failEpisode(ep, fmt.Errorf("panic: %v", r))
		}
	}()

	if ep.SourcePath == nil || *ep.SourcePath == "" {
		q.failEpisode(ep, errors.New("episode has no source_path to encode"))
		return
	}

	profileID, err := q.profileFor(ctx, ep)
	if err != nil {
		q.failEpisode(ep, err)
		return
	}
	resolved, err := q.resolver.Resolve(ctx, profileID)
	if err != nil {
		q.failEpisode(ep, err)
		return
	}

	series, err := q.store.Read().GetSeries(ctx, ep.SeriesID)
	if err != nil {
		q.failEpisode(ep, fmt.Errorf("load series: %w", err))
		return
	}
	set, err := q.store.Read().GetSettings(ctx)
	if err != nil {
		q.failEpisode(ep, fmt.Errorf("load settings: %w", err))
		return
	}

	outputs, err := q.ensureOutputs(ctx, ep, resolved)
	if err != nil {
		q.failEpisode(ep, err)
		return
	}

	for _, out := range outputs {
		if ctx.Err() != nil {
			return // shutdown: leave rows for crash recovery
		}
		if out.Status == "archived" {
			continue // resume: already done
		}
		q.processOutput(ctx, ep, series, set, resolved, out)
	}

	q.finalizeEpisode(ctx, ep, set)
}

// profileFor picks the encode profile for an episode: its explicit profile_id,
// else the series default, else the global settings default.
func (q *Queue) profileFor(ctx context.Context, ep store.Episode) (int64, error) {
	if ep.ProfileID != nil {
		return *ep.ProfileID, nil
	}
	series, err := q.store.Read().GetSeries(ctx, ep.SeriesID)
	if err == nil && series.DefaultProfileID != nil {
		return *series.DefaultProfileID, nil
	}
	set, err := q.store.Read().GetSettings(ctx)
	if err != nil {
		return 0, fmt.Errorf("load settings for default profile: %w", err)
	}
	if set.DefaultProfileID == nil {
		return 0, errors.New("no profile on episode/series and no default profile set")
	}
	return *set.DefaultProfileID, nil
}

// ensureOutputs creates one encoded_outputs row per target resolution if absent
// (idempotent on resume), returning the current rows ordered highest-res first.
func (q *Queue) ensureOutputs(ctx context.Context, ep store.Episode, res Resolved) ([]store.EncodedOutput, error) {
	existing, err := q.store.Read().ListEncodedOutputsByEpisode(ctx, ep.ID)
	if err != nil {
		return nil, fmt.Errorf("list outputs: %w", err)
	}
	have := make(map[int64]bool, len(existing))
	for _, o := range existing {
		have[o.Resolution] = true
	}
	profileID := res.ProfileID
	for _, r := range res.OutputResolutions {
		if have[int64(r)] {
			continue
		}
		if _, err := q.store.Write().CreateEncodedOutput(ctx, store.CreateEncodedOutputParams{
			Uuid:       uuid.NewString(),
			EpisodeID:  ep.ID,
			Resolution: int64(r),
			ProfileID:  &profileID,
			Status:     "queued",
		}); err != nil {
			return nil, fmt.Errorf("create output %dp: %w", r, err)
		}
	}
	return q.store.Read().ListEncodedOutputsByEpisode(ctx, ep.ID)
}

// processOutput drives one resolution through encoding->encoded->thumbnailing->
// archived. On any error the output is parked in 'error' (keeping the original
// for retry) and the rest of the episode continues.
func (q *Queue) processOutput(ctx context.Context, ep store.Episode, series store.Series, set store.Setting, res Resolved, out store.EncodedOutput) {
	resolution := int(out.Resolution)
	libPath := q.libraryPath(series, ep, set, resolution, res.Container)
	tmpPath := libPath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(libPath), 0o755); err != nil {
		q.failOutput(out, fmt.Errorf("create library dir: %w", err))
		return
	}

	// --- encoding ---
	q.setOutputStatus(out, "encoding")
	result, err := q.encoder.Encode(ctx, EncodeRequest{
		Resolved:   res,
		Resolution: resolution,
		Input:      *ep.SourcePath,
		Output:     tmpPath,
	}, func(pct float64, speed string) {
		q.emitProgress(ep, out, resolution, pct, speed)
	})
	if err != nil {
		_ = os.Remove(tmpPath)
		if ctx.Err() != nil {
			return // shutdown
		}
		q.failOutput(out, fmt.Errorf("encode %dp: %w", resolution, err))
		return
	}

	// --- encoded ---
	snap := result.Snapshot
	size := result.Size
	if err := q.store.Write().MarkEncodedOutputEncoded(context.Background(), store.MarkEncodedOutputEncodedParams{
		EncodedPath: &tmpPath,
		EncodedSize: &size,
		ID:          out.ID,
	}); err != nil {
		q.logger.Error("encode: mark encoded", "output", out.ID, "err", err)
	}
	_ = q.store.Write().SetEncodedOutputSnapshot(context.Background(), store.SetEncodedOutputSnapshotParams{
		EncodedParamsSnapshot: &snap, ID: out.ID,
	})
	q.emitOutputStatus(ep, out, resolution, "encoded")

	// --- thumbnailing (highest-res output only) ---
	q.setOutputStatus(out, "thumbnailing")
	q.emitOutputStatus(ep, out, resolution, "thumbnailing")
	if q.isHighestRes(res, resolution) {
		if err := q.generateThumbnails(ctx, ep, series, tmpPath); err != nil {
			q.logger.Warn("encode: thumbnail pass failed", "episode", ep.ID, "err", err)
		}
	}

	// --- archive: move temp -> final library path ---
	if err := moveFile(tmpPath, libPath); err != nil {
		q.failOutput(out, fmt.Errorf("move into library: %w", err))
		return
	}
	if err := q.store.Write().MarkEncodedOutputEncoded(context.Background(), store.MarkEncodedOutputEncodedParams{
		EncodedPath: &libPath,
		EncodedSize: &size,
		ID:          out.ID,
	}); err != nil {
		q.logger.Error("encode: update encoded path", "output", out.ID, "err", err)
	}
	if err := q.store.Write().MarkEncodedOutputArchived(context.Background(), out.ID); err != nil {
		q.logger.Error("encode: mark archived", "output", out.ID, "err", err)
	}
	q.emitOutputStatus(ep, out, resolution, "archived")
	q.logger.Info("encode output archived", "episode", ep.ID, "res", resolution, "path", libPath, "size", size)
}

// isHighestRes reports whether resolution is the largest in the profile's set.
func (q *Queue) isHighestRes(res Resolved, resolution int) bool {
	max := 0
	for _, r := range res.OutputResolutions {
		if r > max {
			max = r
		}
	}
	return resolution == max
}

// generateThumbnails runs the thumbnail pass and records screenshots rows.
func (q *Queue) generateThumbnails(ctx context.Context, ep store.Episode, series store.Series, encoded string) error {
	destDir := filepath.Join(q.dataDir, "thumbnails", ep.Uuid)
	paths, err := q.encoder.Thumbnails(ctx, encoded, destDir)
	if err != nil {
		return err
	}
	_ = q.store.Write().DeleteScreenshotsByEpisode(context.Background(), ep.ID)
	for i, p := range paths {
		if _, err := q.store.Write().CreateScreenshot(context.Background(), store.CreateScreenshotParams{
			Uuid:      uuid.NewString(),
			EpisodeID: ep.ID,
			SeriesID:  series.ID,
			Path:      p,
			Ordinal:   int64(i),
		}); err != nil {
			q.logger.Error("encode: create screenshot", "episode", ep.ID, "err", err)
		}
	}
	return nil
}

// finalizeEpisode marks the episode encoded/archived and triggers original
// cleanup once every output is archived with no errors.
func (q *Queue) finalizeEpisode(ctx context.Context, ep store.Episode, set store.Setting) {
	bg := context.Background()
	unarchived, err := q.store.Read().CountUnarchivedOutputs(ctx, ep.ID)
	if err != nil {
		q.logger.Error("encode: count unarchived", "episode", ep.ID, "err", err)
		return
	}
	errored, err := q.store.Read().CountErroredOutputs(ctx, ep.ID)
	if err != nil {
		q.logger.Error("encode: count errored", "episode", ep.ID, "err", err)
		return
	}

	if errored > 0 {
		msg := fmt.Sprintf("%d output(s) failed to encode", errored)
		_ = q.store.Write().SetEpisodeError(bg, store.SetEpisodeErrorParams{ErrorMessage: &msg, ID: ep.ID})
		q.emitStatus(ep.ID, ep.SeriesID, "error")
		q.logger.Warn("encode: episode kept (output errors)", "episode", ep.ID, "errored", errored)
		return
	}
	if unarchived > 0 {
		// Some outputs still pending (shutdown mid-run); leave episode 'encoding'.
		return
	}

	if err := q.store.Write().MarkEpisodeEncoded(bg, ep.ID); err != nil {
		q.logger.Error("encode: mark episode encoded", "episode", ep.ID, "err", err)
	}
	q.emitStatus(ep.ID, ep.SeriesID, "encoded")

	if err := q.cleanupOriginal(ep, set); err != nil {
		q.logger.Warn("encode: original cleanup failed", "episode", ep.ID, "err", err)
	}

	if err := q.store.Write().MarkEpisodeArchived(bg, ep.ID); err != nil {
		q.logger.Error("encode: mark episode archived", "episode", ep.ID, "err", err)
	}
	q.emitStatus(ep.ID, ep.SeriesID, "archived")
	q.logger.Info("episode fully archived", "episode", ep.ID)
}

// cleanupOriginal applies settings.cleanup_policy to the source files once every
// output is archived: delete removes the source dir, move relocates it to
// processed_dir, keep is a no-op. source_path is updated to match.
func (q *Queue) cleanupOriginal(ep store.Episode, set store.Setting) error {
	bg := context.Background()
	if ep.SourcePath == nil || *ep.SourcePath == "" {
		return nil
	}
	src := *ep.SourcePath
	dir := filepath.Dir(src)

	switch set.CleanupPolicy {
	case "keep":
		return nil
	case "move":
		dest := set.ProcessedDir
		if dest == nil || *dest == "" {
			return errors.New("cleanup_policy=move but processed_dir is unset")
		}
		target := filepath.Join(*dest, filepath.Base(dir))
		if err := os.MkdirAll(*dest, 0o755); err != nil {
			return err
		}
		if err := moveFile(dir, target); err != nil {
			return err
		}
		newPath := filepath.Join(target, filepath.Base(src))
		return q.store.Write().SetEpisodeSourcePath(bg, store.SetEpisodeSourcePathParams{
			SourcePath: &newPath, ID: ep.ID,
		})
	default: // "delete"
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		return q.store.Write().SetEpisodeSourcePath(bg, store.SetEpisodeSourcePathParams{
			SourcePath: nil, ID: ep.ID,
		})
	}
}

// libraryPath builds the Jellyfin/Plex destination path for one output.
func (q *Queue) libraryPath(series store.Series, ep store.Episode, set store.Setting, resolution int, container string) string {
	title := seriesDisplayTitle(series)
	episodeNo := 0
	special := ep.EpisodeNo == nil
	if ep.EpisodeNo != nil {
		episodeNo = int(*ep.EpisodeNo)
	}
	return LibraryPath(PathParams{
		EncodedRoot: set.EncodedRoot,
		Series:      title,
		Season:      int(series.SeasonNumber),
		Episode:     episodeNo,
		IsSpecial:   special,
		Resolution:  resolution,
		Ext:         container,
		Template:    set.NamingTemplate,
	})
}

// seriesDisplayTitle picks the title for library naming: English, then romaji,
// then the canonical title.
func seriesDisplayTitle(s store.Series) string {
	if s.EnglishTitle != nil && *s.EnglishTitle != "" {
		return *s.EnglishTitle
	}
	if s.RomajiTitle != nil && *s.RomajiTitle != "" {
		return *s.RomajiTitle
	}
	return s.Title
}

// --- terminal-state helpers ---

func (q *Queue) setOutputStatus(out store.EncodedOutput, status string) {
	if err := q.store.Write().SetEncodedOutputStatus(context.Background(), store.SetEncodedOutputStatusParams{
		Status: status, ID: out.ID,
	}); err != nil {
		q.logger.Error("encode: set output status", "output", out.ID, "status", status, "err", err)
	}
}

func (q *Queue) failOutput(out store.EncodedOutput, cause error) {
	if errors.Is(cause, context.Canceled) {
		return
	}
	msg := cause.Error()
	if err := q.store.Write().SetEncodedOutputError(context.Background(), store.SetEncodedOutputErrorParams{
		ErrorMessage: &msg, ID: out.ID,
	}); err != nil {
		q.logger.Error("encode: set output error", "output", out.ID, "err", err)
	}
	q.logger.Error("encode output failed", "output", out.ID, "res", out.Resolution, "err", msg)
}

func (q *Queue) failEpisode(ep store.Episode, cause error) {
	if errors.Is(cause, context.Canceled) {
		return
	}
	msg := cause.Error()
	if err := q.store.Write().SetEpisodeError(context.Background(), store.SetEpisodeErrorParams{
		ErrorMessage: &msg, ID: ep.ID,
	}); err != nil {
		q.logger.Error("encode: set episode error", "episode", ep.ID, "err", err)
	}
	q.logger.Error("encode episode failed", "episode", ep.ID, "err", msg)
	q.emitStatus(ep.ID, ep.SeriesID, "error")
}

// --- events ---

func (q *Queue) emitProgress(ep store.Episode, out store.EncodedOutput, resolution int, pct float64, speed string) {
	if q.hub == nil {
		return
	}
	q.hub.Broadcast(events.TypeEncodeProgress, map[string]any{
		"episode_id": ep.ID,
		"series_id":  ep.SeriesID,
		"output_id":  out.ID,
		"resolution": resolution,
		"percent":    pct,
		"speed":      speed,
	})
}

func (q *Queue) emitOutputStatus(ep store.Episode, out store.EncodedOutput, resolution int, status string) {
	if q.hub == nil {
		return
	}
	q.hub.Broadcast(events.TypeEncodeProgress, map[string]any{
		"episode_id": ep.ID,
		"series_id":  ep.SeriesID,
		"output_id":  out.ID,
		"resolution": resolution,
		"status":     status,
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
