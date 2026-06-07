package encode

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// commonFFmpegPaths are checked when ffmpeg/ffprobe are not on PATH (Windows
// installs commonly drop them here). The %s is replaced with the tool name.
var commonFFmpegDirs = []string{
	`C:\ffmpeg\bin`,
	`C:\Program Files\ffmpeg\bin`,
	`C:\Program Files (x86)\ffmpeg\bin`,
}

// Tools holds the resolved absolute paths to ffmpeg and ffprobe.
type Tools struct {
	FFmpeg  string
	FFprobe string
}

// DiscoverTools resolves ffmpeg and ffprobe, honoring an explicit ffmpegPath
// override (settings.ffmpeg_path) first, then PATH, then common Windows install
// dirs. ffprobe is assumed to sit beside ffmpeg.
func DiscoverTools(ffmpegOverride string) (Tools, error) {
	ffmpeg, err := discover("ffmpeg", ffmpegOverride)
	if err != nil {
		return Tools{}, err
	}
	// ffprobe lives next to ffmpeg; reuse the override's directory if given.
	var probeOverride string
	if ffmpegOverride != "" {
		probeOverride = beside(ffmpegOverride, "ffprobe")
	}
	ffprobe, err := discover("ffprobe", probeOverride)
	if err != nil {
		// Fall back to the directory ffmpeg resolved into.
		if cand := beside(ffmpeg, "ffprobe"); fileExists(cand) {
			ffprobe = cand
		} else {
			return Tools{}, err
		}
	}
	return Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}, nil
}

// discover resolves one tool: explicit override, then PATH, then common dirs.
func discover(tool, override string) (string, error) {
	if override != "" {
		if fileExists(override) {
			return override, nil
		}
		return "", fmt.Errorf("%s not found at %q", tool, override)
	}
	if path, err := exec.LookPath(tool); err == nil {
		return path, nil
	}
	exe := tool
	if isWindows() {
		exe = tool + ".exe"
	}
	for _, dir := range commonFFmpegDirs {
		cand := dir + string(os.PathSeparator) + exe
		if fileExists(cand) {
			return cand, nil
		}
	}
	return "", fmt.Errorf("%s not found on PATH or common install dirs", tool)
}

// beside returns the sibling tool path next to a known tool path, preserving the
// .exe suffix on Windows.
func beside(known, tool string) string {
	dir := known
	if i := strings.LastIndexAny(known, `/\`); i >= 0 {
		dir = known[:i]
	} else {
		dir = "."
	}
	exe := tool
	if isWindows() {
		exe = tool + ".exe"
	}
	return dir + string(os.PathSeparator) + exe
}

// execCommand is a thin alias over exec.CommandContext kept so call sites read
// uniformly and are easy to grep.
func execCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func isWindows() bool { return os.PathSeparator == '\\' }

// ProbeDuration returns the media duration in seconds via ffprobe. It is the
// anchor for real percent progress (out_time / duration).
func (t Tools) ProbeDuration(ctx context.Context, input string) (float64, error) {
	cmd := exec.CommandContext(ctx, t.FFprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		input,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe duration: %w", err)
	}
	s := strings.TrimSpace(string(out))
	dur, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe duration %q: %w", s, err)
	}
	if dur <= 0 {
		return 0, errors.New("ffprobe reported non-positive duration")
	}
	return dur, nil
}

// ProgressFunc receives encode progress updates (0..100 percent and speed).
type ProgressFunc func(percent float64, speed string)

// Run executes ffmpeg with the given args, parsing the -progress pipe:1 stream
// to drive real percentage progress against totalSeconds. It is context-
// cancellable: on cancel the process is killed and a wrapped context error is
// returned. stderr is captured so a non-zero exit reports the real ffmpeg cause.
func (t Tools) Run(ctx context.Context, args []string, totalSeconds float64, onProgress ProgressFunc) error {
	cmd := exec.CommandContext(ctx, t.FFmpeg, args...)
	// Kill the whole process on context cancel rather than waiting for a clean
	// exit, matching the ported Wails model.
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return cmd.Process.Kill()
		}
		return nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	parseProgress(stdout, totalSeconds, onProgress)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("encode cancelled: %w", ctx.Err())
		}
		tail := lastLines(stderr.String(), 5)
		return fmt.Errorf("ffmpeg failed: %w: %s", err, tail)
	}
	if onProgress != nil {
		onProgress(100, "")
	}
	return nil
}

// parseProgress reads ffmpeg's -progress key=value stream and converts out_time_us
// (microseconds) into a percentage against totalSeconds. ffmpeg historically
// reports out_time_ms in microseconds too; out_time_us is the unambiguous field,
// so it is preferred with out_time_ms as a fallback. A "progress=end" line marks
// completion.
func parseProgress(r interface{ Read([]byte) (int, error) }, totalSeconds float64, onProgress ProgressFunc) {
	scanner := bufio.NewScanner(r)
	var speed string
	// ffmpeg emits out_time_us and out_time_ms (both microseconds) in the same
	// progress block; prefer out_time_us and ignore out_time_ms once we've seen
	// it, so each tick fires onProgress once rather than twice.
	var sawMicros bool
	for scanner.Scan() {
		line := scanner.Text()
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "speed":
			speed = val
		case "out_time_us", "out_time_ms":
			if key == "out_time_us" {
				sawMicros = true
			} else if sawMicros {
				continue // out_time_ms duplicate of the out_time_us we already used
			}
			if onProgress == nil || totalSeconds <= 0 {
				continue
			}
			us, err := strconv.ParseFloat(val, 64)
			if err != nil || us < 0 {
				continue
			}
			pct := (us / 1e6) / totalSeconds * 100
			if pct > 100 {
				pct = 100
			}
			onProgress(pct, speed)
		case "progress":
			if val == "end" && onProgress != nil {
				onProgress(100, speed)
			}
		}
	}
}

// lastLines returns the last n non-empty lines of s, joined by " | ", for compact
// error context.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	var kept []string
	for i := len(lines) - 1; i >= 0 && len(kept) < n; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			kept = append([]string{l}, kept...)
		}
	}
	return strings.Join(kept, " | ")
}
