package encode

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// cpuEncoder is the libx265 fallback name used when no hardware HEVC encoder
// probes successfully. It is also the name recorded in the snapshot on fallback.
const cpuEncoder = "libx265"

// hwCandidates lists the OS-specific HEVC hardware encoders to try, in priority
// order (best/most-common first). The probe walks this list and the first that
// produces real output wins. Adding a platform encoder is one entry.
// hevc_vaapi is deliberately absent: it needs a -vaapi_device + a
// format=nv12,hwupload (and scale_vaapi) filter chain that the current
// software-filter pipeline does not build, so it cannot encode as-is. Tracked
// for proper support in the roadmap; Linux AMD falls back to libx265 until then.
var hwCandidates = map[string][]string{
	"windows": {"hevc_nvenc", "hevc_qsv", "hevc_amf"},
	"linux":   {"hevc_nvenc", "hevc_qsv"},
	"darwin":  {"hevc_videotoolbox"},
}

// candidatesForOS returns the HEVC hardware-encoder candidates for an OS, or nil
// when the platform has no known hardware HEVC path.
func candidatesForOS(goos string) []string {
	return hwCandidates[goos]
}

// hwProbeArgs builds the throwaway 1-frame encode that proves an encoder is
// functional (compiled-in is not enough — the GPU/driver must actually accept
// the stream). A 256x256 source is used because some encoders (NVENC) reject
// dimensions below their minimum. The encoder-specific quality flags are kept
// minimal; this only verifies the encoder opens.
func hwProbeArgs(encoder string) []string {
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "nullsrc=s=256x256:d=0.1",
		"-pix_fmt", "yuv420p",
		"-c:v", encoder,
		"-f", "null", "-",
	}
}

// probeFunc runs one probe and reports whether the encoder is functional. The
// production implementation shells out to ffmpeg; tests inject a fake.
type probeFunc func(ctx context.Context, encoder string) bool

// GPUResolver resolves the virtual "gpu-auto" codec to a concrete HEVC encoder
// by probing the OS candidates once and caching the winner for the process. A
// fresh process re-probes (drivers/hardware can change across restarts).
type GPUResolver struct {
	probe  probeFunc
	goos   string
	logger *slog.Logger

	once     sync.Once
	resolved string // concrete encoder name, or cpuEncoder on fallback
}

// NewGPUResolver builds a resolver that probes via the given ffmpeg Tools.
func NewGPUResolver(tools Tools, logger *slog.Logger) *GPUResolver {
	if logger == nil {
		logger = slog.Default()
	}
	return &GPUResolver{
		probe:  ffmpegProbe(tools),
		goos:   runtime.GOOS,
		logger: logger,
	}
}

// ffmpegProbe returns a probeFunc that runs the throwaway encode through ffmpeg.
// A non-error exit (and thus a written null output) means the encoder works.
func ffmpegProbe(tools Tools) probeFunc {
	return func(ctx context.Context, encoder string) bool {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		cmd := execCommand(ctx, tools.FFmpeg, hwProbeArgs(encoder)...)
		return cmd.Run() == nil
	}
}

// ResolveGPUEncoder returns the concrete HEVC encoder for "gpu-auto" and whether
// it fell back to the CPU encoder (libx265). The probe runs at most once per
// process; subsequent calls return the cached result.
func (r *GPUResolver) ResolveGPUEncoder() (name string, cpuFallback bool) {
	r.once.Do(func() {
		r.resolved = r.firstWorking(context.Background())
	})
	return r.resolved, r.resolved == cpuEncoder
}

// firstWorking probes the OS candidates in order and returns the first that
// works, else the CPU fallback (logged so a GPU preset still produces output).
func (r *GPUResolver) firstWorking(ctx context.Context) string {
	for _, enc := range candidatesForOS(r.goos) {
		if r.probe(ctx, enc) {
			r.logger.Info("gpu encoder resolved", "encoder", enc)
			return enc
		}
		r.logger.Debug("gpu encoder probe failed", "encoder", enc)
	}
	r.logger.Warn("no hardware HEVC encoder available; falling back to libx265")
	return cpuEncoder
}
