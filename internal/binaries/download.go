package binaries

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProgressFunc receives download progress: bytesReceived and totalBytes (-1 if
// unknown). Callers may use this to drive a first-run provisioning UI.
type ProgressFunc func(bytesReceived, totalBytes int64)

// githubRelease is the minimal subset of the GitHub releases API response.
type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []githubAsset  `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

var githubClient = &http.Client{Timeout: 30 * time.Second}

// fetchLatestRelease queries the GitHub releases API for the latest release of
// "owner/repo" and returns the parsed response.
func fetchLatestRelease(ctx context.Context, repo string) (*githubRelease, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := githubClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release %s: %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch release %s: HTTP %d", repo, resp.StatusCode)
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release %s: %w", repo, err)
	}
	return &rel, nil
}

// findAsset locates the named asset in a release, returning its URL and size.
func findAsset(rel *githubRelease, assetName string) (url string, size int64, err error) {
	for _, a := range rel.Assets {
		if a.Name == assetName {
			return a.BrowserDownloadURL, a.Size, nil
		}
	}
	return "", 0, fmt.Errorf("asset %q not found in release %s", assetName, rel.TagName)
}

// downloadToFile streams url into destPath, reporting progress via onProgress.
// The file is created fresh (truncated). On error the partial file is removed.
func downloadToFile(ctx context.Context, url, destPath string, expectedSize int64, onProgress ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(destPath), err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := githubClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", destPath, err)
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(destPath)
		}
	}()

	total := expectedSize
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}

	var received int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				err = fmt.Errorf("write %s: %w", destPath, writeErr)
				return err
			}
			received += int64(n)
			if onProgress != nil {
				onProgress(received, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			err = fmt.Errorf("read download body: %w", readErr)
			return err
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			return err
		}
	}
	return nil
}

// resolveExpectedHash returns the expected SHA-256 hex digest for assetName.
// When the spec declares a checksumAsset, the checksum file MUST be fetchable
// and MUST contain an entry for the asset — both are hard errors, so an attacker
// who blocks or strips the checksum can't downgrade the install to unverified.
// A spec with no checksumAsset returns ("", nil): verification is opted out.
func resolveExpectedHash(ctx context.Context, rel *githubRelease, spec binarySpec, assetName string) (string, error) {
	if spec.checksumAsset == "" {
		return "", nil
	}
	sums, err := checksumLines(ctx, rel, spec.checksumAsset)
	if err != nil {
		return "", fmt.Errorf("fetch %q: %w", spec.checksumAsset, err)
	}
	hash := parseChecksum(sums, assetName)
	if hash == "" {
		return "", fmt.Errorf("no entry for %q in %s", assetName, spec.checksumAsset)
	}
	return hash, nil
}

// checksumLines downloads the checksum file asset from rel and returns the
// raw text (BSD/sha256sum format: "<hash>  <filename>").
func checksumLines(ctx context.Context, rel *githubRelease, checksumAsset string) (string, error) {
	url, _, err := findAsset(rel, checksumAsset)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := githubClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch checksums: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// parseChecksum scans a sha256sum-format file and returns the expected hex
// digest for targetName, or "" if not found.
func parseChecksum(sums, targetName string) string {
	for _, line := range strings.Split(sums, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Two formats:  "<hash>  <name>"  or  "<hash> *<name>"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		filename := strings.TrimLeft(parts[len(parts)-1], "*")
		if filename == targetName {
			return parts[0]
		}
	}
	return ""
}
