package binaries

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// locate resolves the absolute path of name (e.g. "ffmpeg") in priority order:
//  1. settingsPath — non-empty override from the settings row
//  2. exec.LookPath — binary is on the system PATH
//  3. provisioned  — {dataDir}/bin/<name>[.exe]
//
// Returns an error when none of the three locations yields a usable file.
func locate(name, settingsPath, dataDir string) (string, error) {
	// 1. settings override
	if settingsPath != "" {
		if fileExists(settingsPath) {
			return settingsPath, nil
		}
		return "", fmt.Errorf("%s not found at settings path %q", name, settingsPath)
	}

	// 2. PATH
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	// 3. provisioned bin dir
	if p := provisionedPath(name, dataDir); fileExists(p) {
		return p, nil
	}

	return "", fmt.Errorf("%s not found on PATH or in %s", name, filepath.Join(dataDir, "bin"))
}

// provisionedPath returns the canonical path where this package installs name.
func provisionedPath(name, dataDir string) string {
	exe := name
	if runtime.GOOS == "windows" {
		// yt-dlp binary is named yt-dlp on disk even on Windows (we rename it).
		if !strings.HasSuffix(exe, ".exe") {
			exe = name + ".exe"
		}
	}
	return filepath.Join(dataDir, "bin", exe)
}

// sibling returns the path of siblingName placed next to knownPath, preserving
// the .exe suffix on Windows.
func sibling(knownPath, siblingName string) string {
	dir := filepath.Dir(knownPath)
	exe := siblingName
	if runtime.GOOS == "windows" && !strings.HasSuffix(exe, ".exe") {
		exe = siblingName + ".exe"
	}
	return filepath.Join(dir, exe)
}

// fileExists reports whether p refers to a regular file.
func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
