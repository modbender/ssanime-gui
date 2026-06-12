//go:build smoke

package binaries

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveProvisionYtDlp downloads real yt-dlp from GitHub and verifies the
// binary is executable. Run with: go test -tags smoke ./internal/binaries/...
// This test requires network access.
//
// On Windows, Windows Defender may quarantine the downloaded binary from the
// system temp directory. In that case the test uses %LOCALAPPDATA%/ssanime-smoke-test
// as the data dir (more likely to survive AV scanning in CI exclusion lists).
func TestLiveProvisionYtDlp(t *testing.T) {
	// Prefer a stable path that is less likely to be subject to AV real-time
	// scanning compared to the system temp directory.
	dataDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "ssanime-smoke-test")
	if dataDir == "ssanime-smoke-test" { // LOCALAPPDATA not set (non-Windows)
		dataDir = t.TempDir()
	}
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var lastRecv, lastTotal int64
	progress := func(recv, total int64) {
		lastRecv, lastTotal = recv, total
		t.Logf("  download progress: %d / %d bytes (%.1f%%)", recv, total,
			func() float64 {
				if total <= 0 {
					return 0
				}
				return float64(recv) / float64(total) * 100
			}())
	}

	t.Log("provisioning yt-dlp from GitHub releases ...")
	path, err := provisionYtDlp(ctx, dataDir, progress, noopLogger())
	if err != nil {
		t.Fatalf("provisionYtDlp: %v", err)
	}

	t.Logf("provisioned to: %s (received %d / %d bytes)", path, lastRecv, lastTotal)

	// Verify file exists and is non-empty.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("binary at %s is empty", path)
	}
	t.Logf("binary size: %d bytes", info.Size())

	// Run yt-dlp --version to prove it's actually executable.
	cmd := exec.CommandContext(ctx, path, "--version")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("yt-dlp --version failed: %v", err)
	}
	version := string(out)
	t.Logf("yt-dlp --version: %s", version)

	// Confirm locate() finds it in the provisioned dir.
	found, err := locate("yt-dlp", "", dataDir)
	if err != nil {
		t.Fatalf("locate yt-dlp after provision: %v", err)
	}
	if found != path {
		t.Errorf("locate returned %q, want %q", found, path)
	}

	// Also verify the provisioned path is in the expected location.
	expected := provisionedPath("yt-dlp", dataDir)
	if path != expected {
		t.Errorf("provisioned path = %q, want %q", path, expected)
	}
	_ = filepath.Base(path) // used for lint
}
