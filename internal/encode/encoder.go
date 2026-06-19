package encode

import (
	"context"
	"fmt"
	"os"
)

// EncodeRequest is one resolution's encode job: the resolved profile, the target
// resolution, the source and (temporary) output paths.
type EncodeRequest struct {
	Resolved   Resolved
	Resolution int
	Input      string
	Output     string
}

// EncodeResult reports the outcome of one encode: the params snapshot persisted
// for reproducibility and the encoded file size.
type EncodeResult struct {
	Snapshot string
	Size     int64
}

// Encoder runs a single-resolution encode and the post-encode thumbnail pass.
// The real implementation shells out to ffmpeg; tests inject a fake that writes
// placeholder files, so the fan-out/state-machine logic is exercised without
// ffmpeg.
type Encoder interface {
	// Encode encodes one resolution to req.Output, reporting progress via
	// onProgress (percent 0..100, speed string). It returns the snapshot + size.
	Encode(ctx context.Context, req EncodeRequest, onProgress ProgressFunc) (EncodeResult, error)
	// Thumbnails extracts library thumbnails from input into destDir, returning
	// the image paths in order.
	Thumbnails(ctx context.Context, input, destDir string) ([]string, error)
}

// FFmpegEncoder is the production Encoder backed by ffmpeg/ffprobe.
type FFmpegEncoder struct {
	tools Tools
	gpu   *GPUResolver
}

// NewFFmpegEncoder discovers ffmpeg/ffprobe (honoring an override) and returns a
// ready encoder.
func NewFFmpegEncoder(ffmpegOverride string) (*FFmpegEncoder, error) {
	tools, err := DiscoverTools(ffmpegOverride)
	if err != nil {
		return nil, err
	}
	return &FFmpegEncoder{tools: tools, gpu: NewGPUResolver(tools, nil)}, nil
}

// resolveEncoder maps the profile codec to the concrete video encoder: gpu-auto
// probes the hardware lane (falling back to libx265), x265 stays libx265.
func (e *FFmpegEncoder) resolveEncoder(codec string) string {
	if isGPUCodec(codec) {
		name, _ := e.gpu.ResolveGPUEncoder()
		return name
	}
	return cpuEncoder
}

// Encode builds the full ffmpeg arg list for the request and runs it with real
// ffprobe-anchored progress.
func (e *FFmpegEncoder) Encode(ctx context.Context, req EncodeRequest, onProgress ProgressFunc) (EncodeResult, error) {
	tags, err := e.tools.ProbeColorTags(ctx, req.Input)
	if err != nil {
		// Color tags fall back to none (no re-tagging); the encode still runs.
		tags = ColorTags{}
	}
	streams, err := e.tools.ProbeStreams(ctx, req.Input)
	if err != nil {
		// Track selection falls back to the all-passthrough default; the encode
		// still runs (MKV copy-all, no burn).
		streams = nil
	}
	sel := SelectTracks(req.Resolved, streams)
	encoder := e.resolveEncoder(req.Resolved.Codec)
	args, snapshot, err := BuildArgs(req.Resolved, req.Resolution, tags, sel, encoder, req.Input, req.Output)
	if err != nil {
		return EncodeResult{}, err
	}
	dur, err := e.tools.ProbeDuration(ctx, req.Input)
	if err != nil {
		// Progress falls back to indeterminate; the encode still runs.
		dur = 0
	}
	if err := e.tools.Run(ctx, args, dur, onProgress); err != nil {
		return EncodeResult{}, err
	}
	info, err := os.Stat(req.Output)
	if err != nil {
		return EncodeResult{}, fmt.Errorf("stat encoded output: %w", err)
	}
	if info.Size() == 0 {
		return EncodeResult{}, fmt.Errorf("encoded output %s is empty", req.Output)
	}
	return EncodeResult{Snapshot: snapshot, Size: info.Size()}, nil
}

// Thumbnails delegates to the ffmpeg thumbnail pass.
func (e *FFmpegEncoder) Thumbnails(ctx context.Context, input, destDir string) ([]string, error) {
	return e.tools.GenerateThumbnails(ctx, input, destDir)
}

// Command returns the ffmpeg command line that would be run for a request, used
// for logging/verification (proving every knob is wired).
func (e *FFmpegEncoder) Command(req EncodeRequest) (string, error) {
	encoder := e.resolveEncoder(req.Resolved.Codec)
	args, _, err := BuildArgs(req.Resolved, req.Resolution, ColorTags{}, TrackSelection{}, encoder, req.Input, req.Output)
	if err != nil {
		return "", err
	}
	return e.tools.FFmpeg + " " + joinArgs(args), nil
}
