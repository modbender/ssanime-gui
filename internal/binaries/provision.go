package binaries

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// provisionFFmpeg downloads the BtbN static ffmpeg/ffprobe bundle for the
// current OS/arch into {dataDir}/bin/ and returns the absolute path to ffmpeg.
// Both ffmpeg and ffprobe are extracted; callers can locate ffprobe via
// locate("ffprobe", "", dataDir) after this returns.
func provisionFFmpeg(ctx context.Context, dataDir string, onProgress ProgressFunc, logger *slog.Logger) (string, error) {
	assetName, err := currentAsset(ffmpegSpec)
	if err != nil {
		return "", err
	}

	logger.Info("binaries: fetching ffmpeg release info", "repo", ffmpegSpec.repo)
	rel, err := fetchLatestRelease(ctx, ffmpegSpec.repo)
	if err != nil {
		return "", fmt.Errorf("fetch ffmpeg release: %w", err)
	}
	logger.Info("binaries: found ffmpeg release", "tag", rel.TagName, "asset", assetName)

	assetURL, assetSize, err := findAsset(rel, assetName)
	if err != nil {
		return "", err
	}

	binDir := filepath.Join(dataDir, "bin")
	archivePath := filepath.Join(binDir, assetName)

	logger.Info("binaries: downloading ffmpeg", "url", assetURL, "bytes", assetSize)
	if err := downloadToFile(ctx, assetURL, archivePath, assetSize, onProgress); err != nil {
		return "", fmt.Errorf("download ffmpeg: %w", err)
	}

	// Extract into a sibling directory named after the asset (strip extension).
	extractDir := filepath.Join(binDir, archiveBaseName(assetName))
	if err := extractArchive(assetName, archivePath, extractDir, logger); err != nil {
		os.Remove(archivePath)
		return "", fmt.Errorf("extract ffmpeg: %w", err)
	}
	os.Remove(archivePath)

	// BtbN archives have structure: <archiveBase>/bin/ffmpeg[.exe].
	// extractArchive unpacks the whole zip under extractDir, so the binaries
	// land at extractDir/<archiveBase>/bin/ffmpeg[.exe].
	// We copy both ffmpeg and ffprobe into {dataDir}/bin/ at the top level so
	// locate() finds them consistently without knowing the archive version.
	archiveBase := archiveBaseName(assetName)
	tools := []string{"ffmpeg", "ffprobe"}
	var ffmpegFinal string
	for _, tool := range tools {
		// extractedBin(".", tool) → "./bin/ffmpeg" — we want it relative to
		// the inner archive-named subdirectory inside extractDir.
		relBin := ffmpegSpec.extractedBin(runtime.GOOS, archiveBase, tool)
		src := filepath.Join(extractDir, filepath.FromSlash(relBin))

		dst := provisionedPath(tool, dataDir)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := atomicCopyFile(src, dst); err != nil {
			return "", fmt.Errorf("install %s: %w", tool, err)
		}
		if err := makeExecutable(dst); err != nil {
			return "", fmt.Errorf("chmod %s: %w", tool, err)
		}
		if !fileExists(dst) {
			return "", fmt.Errorf(
				"%s was written to %s but is no longer present — likely quarantined by antivirus. "+
					"Add %s to your antivirus exclusions and retry.",
				tool, dst, filepath.Dir(dst))
		}
		logger.Info("binaries: installed", "binary", tool, "path", dst)
		if tool == "ffmpeg" {
			ffmpegFinal = dst
		}
	}

	// Clean up extracted directory.
	os.RemoveAll(extractDir)

	return ffmpegFinal, nil
}

// provisionYtDlp downloads the yt-dlp binary for the current OS/arch into
// {dataDir}/bin/ and returns its absolute path. SHA-256 is verified against
// the published SHA2-256SUMS file.
func provisionYtDlp(ctx context.Context, dataDir string, onProgress ProgressFunc, logger *slog.Logger) (string, error) {
	assetName, err := currentAsset(ytdlpSpec)
	if err != nil {
		return "", err
	}

	logger.Info("binaries: fetching yt-dlp release info", "repo", ytdlpSpec.repo)
	rel, err := fetchLatestRelease(ctx, ytdlpSpec.repo)
	if err != nil {
		return "", fmt.Errorf("fetch yt-dlp release: %w", err)
	}
	logger.Info("binaries: found yt-dlp release", "tag", rel.TagName, "asset", assetName)

	assetURL, assetSize, err := findAsset(rel, assetName)
	if err != nil {
		return "", err
	}

	// Fetch checksums before downloading the binary.
	var expectedHash string
	if ytdlpSpec.checksumAsset != "" {
		sums, sumErr := checksumLines(ctx, rel, ytdlpSpec.checksumAsset)
		if sumErr != nil {
			logger.Warn("binaries: could not fetch yt-dlp checksums (skipping verification)", "err", sumErr)
		} else {
			expectedHash = parseChecksum(sums, assetName)
		}
	}

	binDir := filepath.Join(dataDir, "bin")
	archivePath := filepath.Join(binDir, assetName)

	logger.Info("binaries: downloading yt-dlp", "url", assetURL, "bytes", assetSize)
	if err := downloadToFile(ctx, assetURL, archivePath, assetSize, onProgress); err != nil {
		return "", fmt.Errorf("download yt-dlp: %w", err)
	}

	// Verify checksum immediately after download.
	if expectedHash != "" {
		logger.Info("binaries: verifying yt-dlp sha256")
		if err := verifyFile(archivePath, expectedHash); err != nil {
			os.Remove(archivePath)
			return "", fmt.Errorf("yt-dlp sha256 verification failed: %w", err)
		}
		logger.Info("binaries: yt-dlp sha256 OK")
	}

	dst := provisionedPath("yt-dlp", dataDir)

	// yt-dlp on Windows/arm64 is a zip; otherwise it's a single binary.
	isArchive := strings.HasSuffix(assetName, ".zip")
	if isArchive {
		extractDir := filepath.Join(binDir, archiveBaseName(assetName))
		if err := extractArchive(assetName, archivePath, extractDir, logger); err != nil {
			os.Remove(archivePath)
			return "", fmt.Errorf("extract yt-dlp: %w", err)
		}
		os.Remove(archivePath)

		// The zip puts the binary at the root of the archive.
		src := filepath.Join(extractDir, binaryFileName("yt-dlp"))
		if err := atomicCopyFile(src, dst); err != nil {
			return "", fmt.Errorf("install yt-dlp: %w", err)
		}
		os.RemoveAll(extractDir)
	} else {
		// Single-file binary: atomic rename from download path to final path.
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := atomicCopyFile(archivePath, dst); err != nil {
			os.Remove(archivePath)
			return "", fmt.Errorf("install yt-dlp: %w", err)
		}
		os.Remove(archivePath)
	}

	if err := makeExecutable(dst); err != nil {
		return "", fmt.Errorf("chmod yt-dlp: %w", err)
	}
	// Final existence check: on Windows, security software (Defender) may
	// quarantine the binary immediately after write. Surface a clear error.
	if !fileExists(dst) {
		return "", fmt.Errorf(
			"yt-dlp was written to %s but is no longer present — likely quarantined by antivirus. "+
				"Add %s to your antivirus exclusions and retry, or install yt-dlp manually and set the path in settings.",
			dst, filepath.Dir(dst))
	}
	logger.Info("binaries: installed yt-dlp", "path", dst)
	return dst, nil
}

// archiveBaseName strips the extension(s) from an archive filename.
// "ffmpeg-master-latest-win64-gpl.zip" → "ffmpeg-master-latest-win64-gpl"
// "ffmpeg-master-latest-linux64-gpl.tar.xz" → "ffmpeg-master-latest-linux64-gpl"
func archiveBaseName(name string) string {
	// Strip .tar.xz or .tar.gz first, then any remaining single extension.
	for _, multi := range []string{".tar.xz", ".tar.gz"} {
		if strings.HasSuffix(name, multi) {
			return name[:len(name)-len(multi)]
		}
	}
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)]
}

// extractArchive dispatches to the appropriate extractor by file extension.
func extractArchive(assetName, archivePath, destDir string, logger *slog.Logger) error {
	logger.Info("binaries: extracting", "archive", archivePath, "dest", destDir)
	switch {
	case strings.HasSuffix(assetName, ".zip"):
		_, err := extractZip(archivePath, destDir)
		return err
	case strings.HasSuffix(assetName, ".tar.xz"):
		_, err := extractTarXz(archivePath, destDir)
		return err
	case strings.HasSuffix(assetName, ".tar.gz"):
		_, err := extractTarGz(archivePath, destDir)
		return err
	default:
		return fmt.Errorf("unsupported archive format: %s", assetName)
	}
}

// binaryFileName returns the OS-appropriate filename for a binary name.
func binaryFileName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// atomicCopyFile copies src to dst, overwriting dst if it exists.
// It writes to a temp file in dst's directory first, then renames for
// atomicity. On Windows, if Rename fails (e.g. AV scan holding the file),
// it falls back to a copy-then-delete of the old file.
func atomicCopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-binary-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := copyStream(in, tmp); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	// Close before rename — required on Windows (file must not be open).
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	// Preserve source permissions.
	if info, err := os.Stat(src); err == nil {
		os.Chmod(tmpName, info.Mode())
	}
	// Rename is atomic on POSIX; on Windows it fails if dst exists and is open.
	// Remove dst first so Rename can succeed, then rename.
	os.Remove(dst) // best-effort; ignore error
	if err := os.Rename(tmpName, dst); err != nil {
		// Fallback: direct copy if rename is denied (e.g. AV scan on tmpName).
		if copyErr := directCopy(src, dst); copyErr != nil {
			os.Remove(tmpName)
			return fmt.Errorf("rename %s→%s: %w (fallback copy also failed: %v)", tmpName, dst, err, copyErr)
		}
		os.Remove(tmpName)
	}
	return nil
}

func copyStream(src interface{ Read([]byte) (int, error) }, dst interface{ Write([]byte) (int, error) }) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			written, werr := dst.Write(buf[:n])
			total += int64(written)
			if werr != nil {
				return total, werr
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
}

// directCopy writes src directly to dst (no temp file). Used as a fallback
// on Windows when rename is denied by AV or other security software.
func directCopy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, statErr := os.Stat(src)
	perm := os.FileMode(0o755)
	if statErr == nil {
		perm = info.Mode().Perm()
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = copyStream(in, out)
	return err
}
