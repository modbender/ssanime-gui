package binaries

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// tmpDir creates a temp directory and registers cleanup.
func tmpDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	return d
}

// ─── zip-slip tests ───────────────────────────────────────────────────────────

func TestResolveEntryPath_Safe(t *testing.T) {
	dest := "/tmp/extract"
	cases := []struct {
		entry string
		want  string
	}{
		{"file.txt", filepath.Join(dest, "file.txt")},
		{"dir/file.txt", filepath.Join(dest, "dir", "file.txt")},
		{"a/b/c.txt", filepath.Join(dest, "a", "b", "c.txt")},
	}
	for _, c := range cases {
		got, err := resolveEntryPath(dest, c.entry)
		if err != nil {
			t.Errorf("resolveEntryPath(%q) unexpected error: %v", c.entry, err)
		}
		if got != c.want {
			t.Errorf("resolveEntryPath(%q) = %q, want %q", c.entry, got, c.want)
		}
	}
}

func TestResolveEntryPath_ZipSlipRejected(t *testing.T) {
	dest := "/tmp/extract"
	malicious := []string{
		"../escape.txt",
		"../../etc/passwd",
		"dir/../../escape.txt",
		"/absolute/path.txt",
	}
	for _, entry := range malicious {
		_, err := resolveEntryPath(dest, entry)
		if err == nil {
			t.Errorf("resolveEntryPath(%q) should have been rejected but was not", entry)
		}
	}
}

// makeEvilZip creates a zip archive containing a traversal entry.
func makeEvilZip(t *testing.T, entryName, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "evil-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	fw, err := w.Create(entryName)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(fw, content)
	w.Close()
	f.Close()
	return f.Name()
}

func TestExtractZip_ZipSlipRejected(t *testing.T) {
	zip := makeEvilZip(t, "../escape.txt", "evil")
	dest := t.TempDir()
	_, err := extractZip(zip, dest)
	if err == nil {
		t.Fatal("expected zip-slip error, got nil")
	}
	if !strings.Contains(err.Error(), "zip-slip") {
		t.Errorf("expected zip-slip in error, got: %v", err)
	}
}

func TestExtractZip_SymlinkRejected(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "symlink-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	// Create a symlink entry by setting the Mode with ModeSymlink.
	header := &zip.FileHeader{Name: "link.txt"}
	header.SetMode(os.ModeSymlink | 0o777)
	_, err = w.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	f.Close()

	_, err = extractZip(f.Name(), t.TempDir())
	if err == nil {
		t.Fatal("expected symlink rejection, got nil")
	}
}

func TestExtractZip_NormalExtraction(t *testing.T) {
	// Build a valid zip with nested dirs.
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	files := map[string]string{
		"dir/":         "",
		"dir/hello.txt": "hello world",
		"readme.txt":   "readme",
	}
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(fw, content)
	}
	w.Close()

	archivePath := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	extracted, err := extractZip(archivePath, dest)
	if err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	// At least the two files should be extracted.
	if len(extracted) < 2 {
		t.Errorf("expected ≥2 files extracted, got %d", len(extracted))
	}
	// Verify content.
	got, err := os.ReadFile(filepath.Join(dest, "dir", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want 'hello world'", got)
	}
}

// ─── tar zip-slip tests ────────────────────────────────────────────────────────

func makeEvilTarGz(t *testing.T, entryName, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "evil-*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	body := []byte(content)
	tw.WriteHeader(&tar.Header{
		Name:     entryName,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
		Mode:     0o644,
	})
	tw.Write(body)
	tw.Close()
	gw.Close()
	f.Close()
	return f.Name()
}

func TestExtractTarGz_ZipSlipRejected(t *testing.T) {
	archive := makeEvilTarGz(t, "../../escape.txt", "evil")
	_, err := extractTarGz(archive, t.TempDir())
	if err == nil {
		t.Fatal("expected zip-slip error from tar.gz, got nil")
	}
}

func TestExtractTarGz_NormalExtraction(t *testing.T) {
	// Build a valid tar.gz.
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	entries := []struct{ name, content string }{
		{"dir/", ""},
		{"dir/ffmpeg", "fake ffmpeg binary"},
		{"dir/ffprobe", "fake ffprobe binary"},
	}
	for _, e := range entries {
		if strings.HasSuffix(e.name, "/") {
			tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: tar.TypeDir, Mode: 0o755})
			continue
		}
		body := []byte(e.content)
		tw.WriteHeader(&tar.Header{Name: e.name, Size: int64(len(body)), Typeflag: tar.TypeReg, Mode: 0o755})
		tw.Write(body)
	}
	tw.Close()
	gw.Close()

	archivePath := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	extracted, err := extractTarGz(archivePath, dest)
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	if len(extracted) < 2 {
		t.Errorf("expected ≥2 files, got %d", len(extracted))
	}
}

// ─── sha256 verification test ─────────────────────────────────────────────────

func TestVerifyFile(t *testing.T) {
	content := []byte("hello, provision")
	f, err := os.CreateTemp(t.TempDir(), "verify-*")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(content)
	f.Close()

	h := sha256.Sum256(content)
	correctHex := hex.EncodeToString(h[:])

	if err := verifyFile(f.Name(), correctHex); err != nil {
		t.Errorf("verifyFile with correct hash: %v", err)
	}
	if err := verifyFile(f.Name(), "deadbeef"); err == nil {
		t.Error("verifyFile with wrong hash should have failed")
	}
}

// ─── parseChecksum test ───────────────────────────────────────────────────────

func TestParseChecksum(t *testing.T) {
	sums := `
abc123  yt-dlp
def456  yt-dlp.exe
789xyz *yt-dlp_linux
`
	cases := []struct{ name, want string }{
		{"yt-dlp", "abc123"},
		{"yt-dlp.exe", "def456"},
		{"yt-dlp_linux", "789xyz"},
		{"notfound", ""},
	}
	for _, c := range cases {
		got := parseChecksum(sums, c.name)
		if got != c.want {
			t.Errorf("parseChecksum(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// ─── GOOS/GOARCH asset-name resolution ────────────────────────────────────────

func TestFFmpegAssetName(t *testing.T) {
	cases := []struct{ goos, goarch, want string }{
		{"windows", "amd64", "ffmpeg-master-latest-win64-gpl.zip"},
		{"windows", "arm64", "ffmpeg-master-latest-winarm64-gpl.zip"},
		{"linux", "amd64", "ffmpeg-master-latest-linux64-gpl.tar.xz"},
		{"linux", "arm64", "ffmpeg-master-latest-linuxarm64-gpl.tar.xz"},
		{"darwin", "amd64", ""},   // not provisioned
		{"darwin", "arm64", ""},   // not provisioned
		{"freebsd", "amd64", ""}, // not provisioned
	}
	for _, c := range cases {
		got := ffmpegSpec.assetName(c.goos, c.goarch)
		if got != c.want {
			t.Errorf("ffmpegSpec.assetName(%q,%q) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestYtDlpAssetName(t *testing.T) {
	cases := []struct{ goos, goarch, want string }{
		{"windows", "amd64", "yt-dlp.exe"},
		{"windows", "arm64", "yt-dlp_win.zip"},
		{"linux", "amd64", "yt-dlp_linux"},
		{"linux", "arm64", "yt-dlp_linux_aarch64"},
		{"darwin", "amd64", "yt-dlp_macos"},
		{"darwin", "arm64", "yt-dlp_macos"},
	}
	for _, c := range cases {
		got := ytdlpSpec.assetName(c.goos, c.goarch)
		if got != c.want {
			t.Errorf("ytdlpSpec.assetName(%q,%q) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

// ─── locator priority test ────────────────────────────────────────────────────

func TestLocate_SettingsPathWins(t *testing.T) {
	dir := tmpDir(t)
	fakeExe := filepath.Join(dir, "ffmpeg-custom.exe")
	if err := os.WriteFile(fakeExe, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := locate("ffmpeg", fakeExe, dir)
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != fakeExe {
		t.Errorf("got %q, want %q", got, fakeExe)
	}
}

func TestLocate_ProvisionedPathFallback(t *testing.T) {
	dir := tmpDir(t)
	// Use a name guaranteed not to be on PATH so locate() reaches the provisioned path.
	const fakeName = "ssanime-fake-binary-xyz-9999"
	prov := provisionedPath(fakeName, dir)
	if err := os.MkdirAll(filepath.Dir(prov), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prov, []byte("fake binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := locate(fakeName, "", dir)
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != prov {
		t.Errorf("got %q, want provisioned %q", got, prov)
	}
}

func TestLocate_MissingReturnsError(t *testing.T) {
	dir := tmpDir(t)
	_, err := locate("no-such-binary-xyz-99", "", dir)
	if err == nil {
		t.Error("expected error for missing binary, got nil")
	}
}

// ─── download + provision via httptest ───────────────────────────────────────

// buildFakeYtDlpServer returns a httptest.Server that serves a minimal GitHub
// API-compatible releases/latest response and the binary asset itself.
// The binary content is "fake-ytdlp-binary".
func buildFakeYtDlpServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	binaryContent := []byte("fake-ytdlp-binary")
	h := sha256.Sum256(binaryContent)
	sumHex := hex.EncodeToString(h[:])
	assetName := "yt-dlp_linux" // neutral — we override GOOS in the test

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			rel := githubRelease{
				TagName: "2099.01.01",
				Assets: []githubAsset{
					{Name: assetName, BrowserDownloadURL: srv.URL + "/asset/" + assetName, Size: int64(len(binaryContent))},
					{Name: "SHA2-256SUMS", BrowserDownloadURL: srv.URL + "/sums", Size: 100},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rel)
		case r.URL.Path == "/sums":
			fmt.Fprintf(w, "%s  %s\n", sumHex, assetName)
		case strings.HasPrefix(r.URL.Path, "/asset/"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(binaryContent)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, assetName
}

func TestDownloadAndVerify_ViaHTTPTest(t *testing.T) {
	srv, assetName := buildFakeYtDlpServer(t)
	defer srv.Close()

	// Point the package's HTTP client at the test server.
	orig := githubClient
	githubClient = &http.Client{Transport: &prefixTransport{base: http.DefaultTransport, prefix: srv.URL}}
	defer func() { githubClient = orig }()

	ctx := context.Background()
	rel, err := fetchLatestRelease(ctx, "fake/ytdlp")
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if rel.TagName != "2099.01.01" {
		t.Errorf("tag = %q", rel.TagName)
	}

	dir := tmpDir(t)
	archivePath := filepath.Join(dir, assetName)
	assetURL, assetSize, err := findAsset(rel, assetName)
	if err != nil {
		t.Fatal(err)
	}

	var received, total int64
	err = downloadToFile(ctx, assetURL, archivePath, assetSize, func(r, tot int64) {
		received = r
		total = tot
	})
	if err != nil {
		t.Fatalf("downloadToFile: %v", err)
	}
	if received == 0 {
		t.Error("progress callback never called")
	}
	_ = total

	// Verify checksum.
	sums, err := checksumLines(ctx, rel, "SHA2-256SUMS")
	if err != nil {
		t.Fatalf("checksumLines: %v", err)
	}
	expected := parseChecksum(sums, assetName)
	if expected == "" {
		t.Fatal("checksum not found in sums file")
	}
	if err := verifyFile(archivePath, expected); err != nil {
		t.Errorf("verifyFile: %v", err)
	}
}

// prefixTransport routes all requests to a fixed base URL (used to redirect
// the GitHub API client to the httptest server).
type prefixTransport struct {
	base   http.RoundTripper
	prefix string
}

func (p *prefixTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the host to the test server.
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = strings.TrimPrefix(p.prefix, "http://")
	// Keep the path so API endpoints match.
	return p.base.RoundTrip(cloned)
}

// ─── extractArchive dispatch ──────────────────────────────────────────────────

func TestExtractArchive_ZipDispatch(t *testing.T) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	fw, _ := w.Create("hello.txt")
	fmt.Fprint(fw, "content")
	w.Close()

	archivePath := filepath.Join(t.TempDir(), "test.zip")
	os.WriteFile(archivePath, buf.Bytes(), 0o644)

	dest := t.TempDir()
	if err := extractArchive("test.zip", archivePath, dest, noopLogger()); err != nil {
		t.Errorf("extractArchive zip: %v", err)
	}
}

// ─── archiveBaseName ──────────────────────────────────────────────────────────

func TestArchiveBaseName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ffmpeg-master-latest-win64-gpl.zip", "ffmpeg-master-latest-win64-gpl"},
		{"ffmpeg-master-latest-linux64-gpl.tar.xz", "ffmpeg-master-latest-linux64-gpl"},
		{"ffmpeg.tar.gz", "ffmpeg"},
		{"yt-dlp.exe", "yt-dlp"},
	}
	for _, c := range cases {
		got := archiveBaseName(c.in)
		if got != c.want {
			t.Errorf("archiveBaseName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ─── provisionedPath ─────────────────────────────────────────────────────────

func TestProvisionedPath(t *testing.T) {
	dir := "/tmp/data"
	p := provisionedPath("ffmpeg", dir)
	expected := filepath.Join(dir, "bin", "ffmpeg")
	if runtime.GOOS == "windows" {
		expected = filepath.Join(dir, "bin", "ffmpeg.exe")
	}
	if p != expected {
		t.Errorf("provisionedPath = %q, want %q", p, expected)
	}
}

// ─── atomicCopyFile ───────────────────────────────────────────────────────────

func TestAtomicCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	dst := filepath.Join(dir, "subdir", "dst.bin")
	content := []byte("binary content")
	os.WriteFile(src, content, 0o644)

	if err := atomicCopyFile(src, dst); err != nil {
		t.Fatalf("atomicCopyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch")
	}
}

// noopLogger returns a slog.Logger that discards output.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
