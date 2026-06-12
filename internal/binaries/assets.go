package binaries

import (
	"fmt"
	"runtime"
)

// binarySpec describes one managed binary's release source.
type binarySpec struct {
	// repo is "owner/repo" on GitHub.
	repo string
	// assetName returns the asset file name for a given GOOS/GOARCH pair.
	// An empty string means "not provisioned on this platform".
	assetName func(goos, goarch string) string
	// checksumAsset is the release asset that contains SHA-256 sums, or "".
	checksumAsset string
	// extractedBin is a function that returns the path of the target binary
	// *inside* the extracted archive directory. The argument is the name
	// stripped of any .exe suffix.
	extractedBin func(goos, archiveDir, name string) string
}

// ffmpegSpec describes BtbN/FFmpeg-Builds static GPL releases.
// Asset naming pattern (verified 2026-06-06 against the GitHub release):
//
//	windows/amd64 → ffmpeg-master-latest-win64-gpl.zip   (contains bin\ffmpeg.exe, bin\ffprobe.exe)
//	windows/arm64 → ffmpeg-master-latest-winarm64-gpl.zip
//	linux/amd64   → ffmpeg-master-latest-linux64-gpl.tar.xz (contains bin/ffmpeg, bin/ffprobe)
//	linux/arm64   → ffmpeg-master-latest-linuxarm64-gpl.tar.xz
//	darwin/*      → not available from BtbN; rely on PATH (brew install ffmpeg)
//
// The "master" prefix means "nightly HEAD" — always the latest build.
var ffmpegSpec = binarySpec{
	repo: "BtbN/FFmpeg-Builds",
	assetName: func(goos, goarch string) string {
		type key struct{ os, arch string }
		m := map[key]string{
			{"windows", "amd64"}: "ffmpeg-master-latest-win64-gpl.zip",
			{"windows", "arm64"}: "ffmpeg-master-latest-winarm64-gpl.zip",
			{"linux", "amd64"}:   "ffmpeg-master-latest-linux64-gpl.tar.xz",
			{"linux", "arm64"}:   "ffmpeg-master-latest-linuxarm64-gpl.tar.xz",
		}
		return m[key{goos, goarch}]
	},
	// BtbN publishes one combined sha256sum-format file ("<hash>  <asset>")
	// covering every asset in the release; parseChecksum matches our asset by name.
	checksumAsset: "checksums.sha256",
	// BtbN archives: top-level dir is the archive name without extension, then bin/
	// e.g. ffmpeg-master-latest-win64-gpl/bin/ffmpeg.exe
	extractedBin: func(goos, archiveDir, name string) string {
		if goos == "windows" {
			return archiveDir + "/bin/" + name + ".exe"
		}
		return archiveDir + "/bin/" + name
	},
}

// ytdlpSpec describes yt-dlp/yt-dlp single-binary releases.
// Asset naming (verified 2026-06-06):
//
//	windows/amd64 → yt-dlp.exe
//	linux/amd64   → yt-dlp_linux
//	linux/arm64   → yt-dlp_linux_aarch64
//	darwin/amd64  → yt-dlp_macos
//	darwin/arm64  → yt-dlp_macos (universal binary)
var ytdlpSpec = binarySpec{
	repo: "yt-dlp/yt-dlp",
	assetName: func(goos, goarch string) string {
		type key struct{ os, arch string }
		m := map[key]string{
			{"windows", "amd64"}: "yt-dlp.exe",
			{"windows", "arm64"}: "yt-dlp_win.zip",
			{"linux", "amd64"}:   "yt-dlp_linux",
			{"linux", "arm64"}:   "yt-dlp_linux_aarch64",
			{"darwin", "amd64"}:  "yt-dlp_macos",
			{"darwin", "arm64"}:  "yt-dlp_macos",
		}
		return m[key{goos, goarch}]
	},
	checksumAsset: "SHA2-256SUMS",
	// yt-dlp assets are either a single binary or a zip containing the binary
	// at the root. extractedBin is the path after extraction (single-file
	// assets copy directly; zips are extracted to a dir).
	extractedBin: func(goos, archiveDir, name string) string {
		if goos == "windows" {
			return archiveDir + "/" + name + ".exe"
		}
		return archiveDir + "/" + name
	},
}

// currentAsset returns the release asset name for the given spec on the
// running OS/arch. Returns an error if no asset is mapped (e.g. darwin/ffmpeg).
func currentAsset(spec binarySpec) (string, error) {
	name := spec.assetName(runtime.GOOS, runtime.GOARCH)
	if name == "" {
		return "", fmt.Errorf("no provisioned asset for %s/%s — install manually and set the path in settings", runtime.GOOS, runtime.GOARCH)
	}
	return name, nil
}
