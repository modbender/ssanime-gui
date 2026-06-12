package binaries

import (
	"context"
	"fmt"

	"github.com/modbender/ssanime-gui/internal/store"
)

// persistPath writes the resolved binary path into the settings row.
// name must be "ffmpeg" or "ytdlp".
func persistPath(ctx context.Context, st *store.Store, name, path string) error {
	set, err := st.Read().GetSettings(ctx)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	p := &path
	params := store.UpdateSettingsParams{
		DownloadRoot:        set.DownloadRoot,
		EncodedRoot:         set.EncodedRoot,
		CleanupPolicy:       set.CleanupPolicy,
		ProcessedDir:        set.ProcessedDir,
		NamingTemplate:      set.NamingTemplate,
		DownloadBackend:     set.DownloadBackend,
		DefaultProfileID:    set.DefaultProfileID,
		ConcurrencyDownload: set.ConcurrencyDownload,
		ConcurrencyEncode:   set.ConcurrencyEncode,
		FfmpegPath:          set.FfmpegPath,
		YtdlpPath:           set.YtdlpPath,
		Port:                set.Port,
		DohEnabled:          set.DohEnabled,
	}
	switch name {
	case "ffmpeg":
		params.FfmpegPath = p
	case "ytdlp":
		params.YtdlpPath = p
	default:
		return fmt.Errorf("unknown binary name %q", name)
	}
	_, err = st.Write().UpdateSettings(ctx, params)
	return err
}
