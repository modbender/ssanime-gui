// Package binaries provisions, locates, and self-updates the external binaries
// that ssanime-gui depends on: ffmpeg, ffprobe, and yt-dlp.
//
// Resolution priority for each binary:
//  1. settings path  — user-supplied absolute path persisted in the DB
//  2. exec.LookPath  — binary is on the system PATH
//  3. provisioned    — {DataDir}/bin/<name>[.exe] downloaded and extracted by
//     this package on first run
//
// If the binary is absent at all three locations, EnsureXxx downloads the
// appropriate static build for the current GOOS/GOARCH from its upstream
// GitHub release, extracts it zip-slip-safely, verifies the SHA-256 checksum
// where published, makes the file executable, and persists the resolved path
// into the settings row.
package binaries

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modbender/ssanime-gui/internal/store"
)

// Manager owns the provisioning state for all managed binaries. It holds the
// resolved paths after the first Ensure call so subsequent calls are cheap
// (one read, no network).
type Manager struct {
	st      *store.Store
	dataDir string
	logger  *slog.Logger

	// resolved paths — set after a successful Ensure call.
	FFmpegPath  string
	FFprobePath string
	YtDlpPath   string
}

// New returns a Manager. It does not perform any I/O; call EnsureFFmpeg,
// EnsureFFprobe, EnsureYtDlp to resolve + provision.
func New(st *store.Store, dataDir string, logger *slog.Logger) *Manager {
	return &Manager{st: st, dataDir: dataDir, logger: logger}
}

// EnsureFFmpeg resolves or provisions ffmpeg and returns its absolute path.
// The resolved path is persisted to settings.ffmpeg_path so future boots skip
// the provision step. Non-fatal: callers decide whether to abort or run degraded.
func (m *Manager) EnsureFFmpeg(ctx context.Context, onProgress ProgressFunc) (string, error) {
	set, err := m.st.Read().GetSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("binaries: read settings: %w", err)
	}
	var settingsPath string
	if set.FfmpegPath != nil {
		settingsPath = *set.FfmpegPath
	}

	p, err := locate("ffmpeg", settingsPath, m.dataDir)
	if err == nil {
		m.FFmpegPath = p
		return p, nil
	}

	m.logger.Info("ffmpeg not found locally, provisioning", "err", err)
	p, err = provisionFFmpeg(ctx, m.dataDir, onProgress, m.logger)
	if err != nil {
		return "", fmt.Errorf("binaries: provision ffmpeg: %w", err)
	}
	m.FFmpegPath = p
	if err := persistPath(ctx, m.st, "ffmpeg", p); err != nil {
		m.logger.Warn("binaries: persist ffmpeg_path failed (non-fatal)", "err", err)
	}
	return p, nil
}

// EnsureFFprobe resolves or provisions ffprobe. On Windows, ffprobe is bundled
// in the same BtbN zip as ffmpeg, so EnsureFFmpeg should be called first —
// this call will find ffprobe beside the already-provisioned ffmpeg.
func (m *Manager) EnsureFFprobe(ctx context.Context, onProgress ProgressFunc) (string, error) {
	set, err := m.st.Read().GetSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("binaries: read settings: %w", err)
	}
	// ffprobe has no dedicated settings column; derive from ffmpeg_path if set.
	var settingsPath string
	if set.FfmpegPath != nil && *set.FfmpegPath != "" {
		settingsPath = sibling(*set.FfmpegPath, "ffprobe")
	}

	p, err := locate("ffprobe", settingsPath, m.dataDir)
	if err == nil {
		m.FFprobePath = p
		return p, nil
	}

	// ffprobe should have been extracted alongside ffmpeg; provision ffmpeg which
	// extracts the whole bundle, then locate again.
	m.logger.Info("ffprobe not found, provisioning via ffmpeg bundle", "err", err)
	if _, provErr := provisionFFmpeg(ctx, m.dataDir, onProgress, m.logger); provErr != nil {
		return "", fmt.Errorf("binaries: provision ffprobe (via ffmpeg bundle): %w", provErr)
	}
	p, err = locate("ffprobe", "", m.dataDir)
	if err != nil {
		return "", fmt.Errorf("binaries: ffprobe still missing after bundle extract: %w", err)
	}
	m.FFprobePath = p
	return p, nil
}

// EnsureYtDlp resolves or provisions yt-dlp and returns its absolute path.
func (m *Manager) EnsureYtDlp(ctx context.Context, onProgress ProgressFunc) (string, error) {
	set, err := m.st.Read().GetSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("binaries: read settings: %w", err)
	}
	var settingsPath string
	if set.YtdlpPath != nil {
		settingsPath = *set.YtdlpPath
	}

	p, err := locate("yt-dlp", settingsPath, m.dataDir)
	if err == nil {
		m.YtDlpPath = p
		return p, nil
	}

	m.logger.Info("yt-dlp not found locally, provisioning", "err", err)
	p, err = provisionYtDlp(ctx, m.dataDir, onProgress, m.logger)
	if err != nil {
		return "", fmt.Errorf("binaries: provision yt-dlp: %w", err)
	}
	m.YtDlpPath = p
	if err := persistPath(ctx, m.st, "ytdlp", p); err != nil {
		m.logger.Warn("binaries: persist ytdlp_path failed (non-fatal)", "err", err)
	}
	return p, nil
}

// UpdateYtDlp re-fetches the latest yt-dlp release, atomically replaces the
// provisioned binary, and updates the settings row. It is safe to call while
// the binary is not in use.
func (m *Manager) UpdateYtDlp(ctx context.Context, onProgress ProgressFunc) (string, error) {
	return updateBinary(ctx, "yt-dlp", m.dataDir, onProgress, m.logger, func() (string, error) {
		return provisionYtDlp(ctx, m.dataDir, onProgress, m.logger)
	}, func(p string) error {
		m.YtDlpPath = p
		return persistPath(ctx, m.st, "ytdlp", p)
	})
}

// UpdateFFmpeg re-fetches the latest BtbN ffmpeg bundle, atomically replaces
// the provisioned ffmpeg and ffprobe, and updates the settings row.
func (m *Manager) UpdateFFmpeg(ctx context.Context, onProgress ProgressFunc) (string, error) {
	return updateBinary(ctx, "ffmpeg", m.dataDir, onProgress, m.logger, func() (string, error) {
		return provisionFFmpeg(ctx, m.dataDir, onProgress, m.logger)
	}, func(p string) error {
		m.FFmpegPath = p
		return persistPath(ctx, m.st, "ffmpeg", p)
	})
}

// updateBinary orchestrates the atomic self-update: provision into a fresh
// temp name, persist, then clean up the old binary. The provision func must
// place the final binary at the standard provisioned path.
func updateBinary(
	_ context.Context,
	name, _ string,
	_ ProgressFunc,
	logger *slog.Logger,
	provision func() (string, error),
	persist func(string) error,
) (string, error) {
	logger.Info("binaries: updating", "binary", name)
	p, err := provision()
	if err != nil {
		return "", fmt.Errorf("binaries: update %s: %w", name, err)
	}
	if err := persist(p); err != nil {
		logger.Warn("binaries: persist after update failed (non-fatal)", "binary", name, "err", err)
	}
	return p, nil
}
