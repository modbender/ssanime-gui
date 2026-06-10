package animedb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// localFresh reports whether a cached dataset exists and is younger than
// staleAfter. A missing or unreadable file is treated as not fresh.
func (d *DB) localFresh() bool {
	info, err := os.Stat(d.dataPath())
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	return time.Since(info.ModTime()) < staleAfter
}

// download streams the compressed dataset into the cache path. The body is
// bounded by maxDownloadBytes via io.LimitReader so a hostile or runaway
// response can't fill the disk; the write goes to a temp file and is renamed
// into place so a partial download never replaces a good cache. ctx
// cancellation aborts the transfer.
func (d *DB) download(ctx context.Context) error {
	if err := os.MkdirAll(d.dataDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", d.dataDir, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download dataset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download dataset: HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(d.dataDir, ".tmp-animedb-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Clean up the temp file on any error path; harmless if we already renamed.
	defer os.Remove(tmpName)

	limited := io.LimitReader(resp.Body, maxDownloadBytes+1)
	n, err := io.Copy(tmp, limited)
	if cerr := tmp.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("write dataset: %w", err)
	}
	if n > maxDownloadBytes {
		return fmt.Errorf("dataset exceeds %d byte cap", maxDownloadBytes)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// os.Rename overwrites on POSIX; on Windows it fails if dst exists, so remove
	// first. The window is acceptable — a crash here just forces a re-download.
	dst := d.dataPath()
	os.Remove(dst)
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("install dataset: %w", err)
	}
	d.logger.Info("animedb: dataset downloaded", "path", dst, "bytes", n)
	return nil
}

// openCached opens the cached compressed dataset for reading.
func (d *DB) openCached() (*os.File, error) {
	return os.Open(filepath.Clean(d.dataPath()))
}
