package binaries

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	xzdecomp "github.com/ulikunitz/xz"
)

// maxExtractSize caps the total uncompressed data extracted from any archive to
// guard against zip-bomb attacks.
const maxExtractSize = 2 << 30 // 2 GiB

// resolveEntryPath joins destDir and entryName, then verifies the result is
// still inside destDir (zip-slip protection). Returns an error for traversals.
func resolveEntryPath(destDir, entryName string) (string, error) {
	// Reject absolute paths before filepath.Join can neutralise them.
	if filepath.IsAbs(filepath.FromSlash(entryName)) {
		return "", fmt.Errorf("zip-slip rejected: %q is an absolute path", entryName)
	}
	// Also reject the Unix-style absolute prefix before conversion.
	if strings.HasPrefix(entryName, "/") {
		return "", fmt.Errorf("zip-slip rejected: %q is an absolute path", entryName)
	}
	// Normalise slashes so filepath.Join works consistently cross-platform.
	clean := filepath.Clean(filepath.Join(destDir, filepath.FromSlash(entryName)))
	// The resolved path must be strictly under destDir.
	prefix := filepath.Clean(destDir) + string(os.PathSeparator)
	if !strings.HasPrefix(clean+string(os.PathSeparator), prefix) {
		return "", fmt.Errorf("zip-slip rejected: %q would escape %q", entryName, destDir)
	}
	return clean, nil
}

// extractZip extracts archivePath into destDir (created if necessary) and
// returns the set of extracted file paths. It rejects symlinks, absolute paths,
// and any entry whose resolved path would escape destDir (zip-slip).
func extractZip(archivePath, destDir string) ([]string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open zip %s: %w", archivePath, err)
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}

	var extracted []string
	var totalSize int64
	for _, f := range r.File {
		mode := f.Mode()
		// Reject symlinks and anything that isn't a regular file or directory.
		if mode&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("zip contains symlink %q — rejected", f.Name)
		}
		if !mode.IsRegular() && !f.FileInfo().IsDir() {
			// skip device nodes, named pipes, etc.
			continue
		}

		fpath, err := resolveEntryPath(destDir, f.Name)
		if err != nil {
			return nil, err
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0o755); err != nil {
				return nil, err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
			return nil, err
		}

		perm := f.Mode().Perm()
		if perm == 0 {
			perm = 0o644
		}
		out, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
		if err != nil {
			return nil, fmt.Errorf("create %s: %w", fpath, err)
		}

		rc, err := f.Open()
		if err != nil {
			out.Close()
			return nil, err
		}

		n, copyErr := io.Copy(out, io.LimitReader(rc, maxExtractSize-totalSize))
		out.Close()
		rc.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("extract %s: %w", f.Name, copyErr)
		}
		totalSize += n
		if totalSize >= maxExtractSize {
			return nil, errors.New("archive exceeds maximum extract size — possible zip bomb")
		}
		extracted = append(extracted, fpath)
	}
	return extracted, nil
}

// extractTarXz extracts a .tar.xz archive into destDir.
func extractTarXz(archivePath, destDir string) ([]string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	xzr, err := xzdecomp.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("xz reader: %w", err)
	}
	return extractTar(tar.NewReader(xzr), destDir)
}

// extractTarGz extracts a .tar.gz archive into destDir.
func extractTarGz(archivePath, destDir string) ([]string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()

	return extractTar(tar.NewReader(gzr), destDir)
}

// extractTar extracts entries from tr into destDir with zip-slip protection.
func extractTar(tr *tar.Reader, destDir string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}

	var extracted []string
	var totalSize int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar next: %w", err)
		}

		fpath, err := resolveEntryPath(destDir, hdr.Name)
		if err != nil {
			return nil, err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(fpath, 0o755); err != nil {
				return nil, err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
				return nil, err
			}
			perm := os.FileMode(hdr.Mode).Perm()
			if perm == 0 {
				perm = 0o644
			}
			out, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
			if err != nil {
				return nil, fmt.Errorf("create %s: %w", fpath, err)
			}
			n, copyErr := io.Copy(out, io.LimitReader(tr, maxExtractSize-totalSize))
			out.Close()
			if copyErr != nil {
				return nil, fmt.Errorf("extract %s: %w", hdr.Name, copyErr)
			}
			totalSize += n
			if totalSize >= maxExtractSize {
				return nil, errors.New("archive exceeds maximum extract size — possible zip bomb")
			}
			extracted = append(extracted, fpath)
		case tar.TypeSymlink, tar.TypeLink:
			// Reject hard/soft links — they can be used to escape the dest dir.
			return nil, fmt.Errorf("tar contains link %q → %q — rejected", hdr.Name, hdr.Linkname)
		case tar.TypeXHeader, tar.TypeXGlobalHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			// Metadata-only entries; tar library already consumed them.
			continue
		default:
			// Skip device nodes, fifos, etc. silently.
			continue
		}
	}
	return extracted, nil
}

// verifyFile computes the SHA-256 of p and compares it to expectedHex.
func verifyFile(p, expectedHex string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(expectedHex) {
		return fmt.Errorf("sha256 mismatch for %s: want %s got %s", p, expectedHex, got)
	}
	return nil
}

// makeExecutable sets the executable bit on p. On Windows, chmod is meaningless
// (executability is determined by the .exe extension), so this is a no-op there.
func makeExecutable(p string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	return os.Chmod(p, info.Mode()|0o111)
}
