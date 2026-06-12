package encode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// thumbnailCount is how many evenly-spaced frames the thumbnail pass extracts.
const thumbnailCount = 4

// thumbMargin keeps extraction away from the very start/end (titles, black
// frames) by sampling within the inner fraction of the runtime.
const thumbMargin = 0.05

// GenerateThumbnails extracts thumbnailCount evenly-spaced frames from the
// encoded file into destDir, returning the absolute image paths in order. It
// probes the duration to place seeks; failures on individual frames abort the
// pass (a half-set of thumbnails is treated as an error so the caller can retry).
func (t Tools) GenerateThumbnails(ctx context.Context, input, destDir string) ([]string, error) {
	dur, err := t.ProbeDuration(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("thumbnail duration probe: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("thumbnail dir: %w", err)
	}

	span := dur * (1 - 2*thumbMargin)
	start := dur * thumbMargin
	paths := make([]string, 0, thumbnailCount)
	for i := 0; i < thumbnailCount; i++ {
		// Evenly distribute the N frames across the inner span.
		frac := float64(i) + 0.5
		ts := start + span*frac/float64(thumbnailCount)
		out := filepath.Join(destDir, fmt.Sprintf("%02d.jpg", i))
		if err := t.extractFrame(ctx, input, ts, out); err != nil {
			return nil, fmt.Errorf("extract thumbnail %d: %w", i, err)
		}
		paths = append(paths, out)
	}
	return paths, nil
}

// extractFrame grabs a single frame at ts seconds. The seek is placed before -i
// (input seeking) for speed, with -frames:v 1 to emit exactly one image.
func (t Tools) extractFrame(ctx context.Context, input string, ts float64, out string) error {
	cmd := execCommand(ctx, t.FFmpeg,
		"-hide_banner", "-nostdin",
		"-ss", strconv.FormatFloat(ts, 'f', 3, 64),
		"-i", input,
		"-frames:v", "1",
		"-q:v", "2",
		"-y", out,
	)
	if combined, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg frame extract: %w: %s", err, lastLines(string(combined), 3))
	}
	if !fileExists(out) {
		return fmt.Errorf("ffmpeg produced no thumbnail at %s", out)
	}
	return nil
}
