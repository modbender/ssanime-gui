// Package download is the torrent-download layer of the pipeline. It owns the
// backend seam — a Downloader interface with one implementation per
// download_clients.kind (embedded anacrolix, qBittorrent, Transmission) wired
// through a data-map registry, not a switch — and a DB-backed worker-pool queue
// that drives queued episodes through queued->downloading->downloaded.
//
// Phase 4 stops at "downloaded": it never deletes the source file. The original
// is removed only after the encoder/archiver finishes (Phase 5). On completion
// the embedded backend stops the torrent (drops it from the client, so it does
// not seed) but keeps the downloaded files on disk for the encoder.
package download

import (
	"context"
	"net/http"
	"time"
)

// Request is what the queue hands a backend to start one download. Exactly one
// of Magnet / TorrentURL / InfoHash is the source; Dir is the per-episode target
// directory the backend should write into (already absolute).
type Request struct {
	// EpisodeID is the durable identity of the work item, used for logging and
	// as a stable per-download directory name for backends that need one.
	EpisodeID int64
	// Name is a human-readable label (the release title) for logs/UI.
	Name string
	// Magnet is a magnet: URI. Preferred source when present.
	Magnet string
	// TorrentURL is an http(s) .torrent URL, used when Magnet is empty.
	TorrentURL string
	// InfoHash is a bare info hash, used when neither Magnet nor TorrentURL is set.
	InfoHash string
	// Dir is the absolute directory the download should land in. The backend may
	// create per-torrent subdirs beneath it.
	Dir string
}

// Progress is a point-in-time snapshot of a running download. It is transient —
// pushed to the events hub, never persisted (durable state lives in the DB).
type Progress struct {
	// BytesDone is the number of verified bytes downloaded so far.
	BytesDone int64
	// BytesTotal is the total size of the selected content, or 0 until known.
	BytesTotal int64
	// Peers is the number of connected/active peers.
	Peers int
	// SpeedBps is the download rate in bytes/sec over the last sample interval.
	SpeedBps int64
	// Done reports whether the download has completed.
	Done bool
}

// Percent returns completion as a 0..100 value (0 when total is unknown).
func (p Progress) Percent() float64 {
	if p.BytesTotal <= 0 {
		return 0
	}
	return float64(p.BytesDone) / float64(p.BytesTotal) * 100
}

// Handle is a running download. The queue polls Progress on a ticker, blocks on
// Done for completion, reads SourcePath/SourceSize on success, and calls Remove
// to tear the download down. Implementations must be safe for concurrent use by
// the polling goroutine and the queue worker.
type Handle interface {
	// Progress returns the current snapshot.
	Progress() Progress
	// Done is closed when the download completes successfully. It is never closed
	// if the download is removed/failed first.
	Done() <-chan struct{}
	// Err returns a terminal error if the download failed, or nil. Valid to read
	// after Done is closed or after Remove.
	Err() error
	// SourcePath returns the absolute path of the primary downloaded file (the
	// largest video file in a multi-file torrent). Valid once Done is closed.
	SourcePath() string
	// SourceSize returns the byte size of the primary downloaded file.
	SourceSize() int64
	// Remove tears the download down. stopSeed drops the torrent so the client
	// stops uploading; deleteData removes the downloaded files from disk. Phase 4
	// always calls Remove(stopSeed=true, deleteData=false) on success so seeding
	// stops but the encoder still has the file.
	Remove(stopSeed, deleteData bool) error
}

// Backend is one download mechanism (embedded anacrolix, an external qBittorrent
// or Transmission client, etc.). One Backend instance is created per enabled
// download_clients row by its Factory and shared across all downloads it runs.
type Backend interface {
	// Kind is the download_clients.kind this backend implements.
	Kind() string
	// Add starts a download and returns its Handle. The returned Handle runs until
	// completion, failure, or Remove.
	Add(ctx context.Context, req Request) (Handle, error)
	// Close releases the backend's resources (the embedded client, HTTP sessions).
	Close() error
}

// Config is the per-client configuration a Factory needs to build a Backend.
// It is assembled by the queue from a download_clients row + settings.
type Config struct {
	// ClientID is the download_clients.id this backend was built for.
	ClientID int64
	// Kind is the download_clients.kind.
	Kind string
	// Name is the client's display name.
	Name string
	// DownloadRoot is settings.download_root, the base directory for all data.
	DownloadRoot string
	// ConcurrencyDownload is settings.concurrency_download, also used as the
	// embedded client's listen/connection budget hint.
	ConcurrencyDownload int

	// Host/Port/Username/Password configure external clients (qBittorrent,
	// Transmission). Unused by the embedded backend.
	Host     string
	Port     int
	Username string
	Password string

	// Dialer, when set, is used by external HTTP backends so requests can be
	// routed through DoH if needed. Nil = default transport.
	HTTPTransport http.RoundTripper
}

// Factory builds a Backend from a Config. Each kind registers exactly one.
type Factory func(ctx context.Context, cfg Config) (Backend, error)

// pollInterval is how often the queue samples a Handle's Progress while a
// download runs. Chosen to feel live in the UI without flooding the events hub.
const pollInterval = time.Second
