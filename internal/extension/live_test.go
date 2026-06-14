//go:build live

// Live integration test for Hayase-compatible extensions (exten.pages.dev).
// Run with:
//
//	go test -tags live ./internal/extension/... -run TestLive -v
//
// Network access is required. It fetches the repo index, resolves ids via the
// real ani.zip, loads every torrent extension through the goja runtime, and runs
// a real SmartSearch for One Piece (AniList 21).
//
// PASS criterion: every extension LOADS without a mechanics bug (a panic, an
// undefined global like atob/navigator, or a type error). A dead upstream
// (network/HTTP failure) or an empty result set is reported, not failed — those
// are not bugs in ssanime's runtime.
package extension

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/source"
)

const defaultRepoIndex = "https://exten.pages.dev/index.json"

// loadDotEnv parses a repo-root .env (KEY=VALUE, # comments, blanks ignored).
// The test runs with cwd = internal/extension, so the repo root is ../..  .
func loadDotEnv() map[string]string {
	out := map[string]string{}
	path := filepath.Join("..", "..", ".env")
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		out[k] = v
	}
	return out
}

// envOr returns os.Getenv(key), then the .env value, then fallback.
func envOr(dotenv map[string]string, key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if v, ok := dotenv[key]; ok && v != "" {
		return v
	}
	return fallback
}

// extResult records one extension's outcome for the summary table.
type extResult struct {
	id      string
	name    string
	loaded  bool
	results int
	err     string
}

// mechanicsBugSubstrings flag a genuine runtime bug (vs. a tolerated upstream
// failure). If an error contains one of these, the extension is broken on our
// side and the test fails.
var mechanicsBugSubstrings = []string{
	"is not defined",
	"ReferenceError",
	"TypeError",
	"not a function",
	"__ssExt undefined",
	"compile:",
}

// classifyError reports whether an error string indicates a genuine runtime
// (mechanics) bug rather than a tolerated upstream/network failure.
func classifyError(errStr string) (isMechanicsBug bool) {
	low := strings.ToLower(errStr)
	for _, s := range mechanicsBugSubstrings {
		if strings.Contains(low, strings.ToLower(s)) {
			// A TypeError/ReferenceError on atob/navigator/etc is always a bug,
			// even if other upstream words are present.
			return true
		}
	}
	return false
}

func TestLiveHayaseExtensions(t *testing.T) {
	dotenv := loadDotEnv()
	indexURL := envOr(dotenv, "SSANIME_TEST_REPO_INDEX", defaultRepoIndex)
	t.Logf("repo index: %s", indexURL)

	client := &http.Client{Timeout: 30 * time.Second}
	ctx := context.Background()

	// 1. Fetch + decode the index. A malformed/empty index is a hard failure.
	entries, err := fetchIndex(ctx, client, indexURL)
	if err != nil {
		t.Fatalf("fetch index %s: %v", indexURL, err)
	}
	if len(entries) == 0 {
		t.Fatalf("index %s is empty", indexURL)
	}
	t.Logf("index: %d entries", len(entries))

	// 2. Resolve ids for AniList 21 (One Piece) via the real ani.zip.
	resolver := anizip.New()
	ids, idErr := resolver.GetIDs(ctx, 21)
	if idErr != nil {
		t.Logf("WARN ani.zip GetIDs(21) failed (id-dependent extensions may return 0 results): %v", idErr)
	} else {
		ep1 := ids.Episodes[1]
		t.Logf("ani.zip ids for 21: anidb=%d tvdb=%d tmdb=%d mal=%d | ep1: anidbEid=%d tvdbEid=%d",
			ids.AnidbID, ids.TvdbID, ids.TmdbID, ids.MalID, ep1.AnidbEid, ep1.TvdbEid)
	}

	eng := "One Piece"
	media := source.Media{
		ID:           21,
		RomajiTitle:  "One Piece",
		EnglishTitle: &eng,
		EpisodeCount: 1100,
		Format:       "TV",
		Status:       "RELEASING",
	}
	opts := source.SmartSearchOptions{Media: media, EpisodeNumber: 1, Resolution: "1080"}

	var results []extResult
	for _, e := range entries {
		if !strings.EqualFold(e.Type, "torrent") {
			continue
		}
		results = append(results, runOneExtension(t, client, resolver, e, opts))
	}

	// 4. Summary table + assertions.
	fmt.Printf("\n=== LIVE HAYASE EXTENSION SUMMARY ===\n")
	fmt.Printf("%-32s %-8s %-8s %s\n", "EXTENSION", "LOADED", "RESULTS", "ERROR")
	var seadexResults, idPathResults int
	for _, r := range results {
		fmt.Printf("%-32s %-8v %-8d %s\n", r.id, r.loaded, r.results, r.err)
		if strings.Contains(strings.ToLower(r.id), "seadex") {
			seadexResults = r.results
		}
		if strings.Contains(strings.ToLower(r.id), "animetosho") || strings.Contains(strings.ToLower(r.id), "nekobt") {
			if r.results > idPathResults {
				idPathResults = r.results
			}
		}
		if !r.loaded {
			t.Errorf("extension %s did NOT load (mechanics bug): %s", r.id, r.err)
		}
	}
	fmt.Printf("\nseadex (anilistId path) results: %d\n", seadexResults)
	fmt.Printf("animetosho/nekobt (ani.zip id path) best results: %d\n", idPathResults)
	if seadexResults == 0 {
		t.Logf("NOTE: seadex returned 0 results — upstream may be down or no best-release for this episode")
	}
	if idPathResults == 0 {
		t.Logf("NOTE: animetosho/nekobt returned 0 results — upstream may be down")
	}
}

func runOneExtension(t *testing.T, client *http.Client, resolver IDResolver, e IndexEntry, opts source.SmartSearchOptions) (res extResult) {
	res = extResult{id: e.ID, name: e.Name}

	// Recover a panic into a mechanics-bug failure for THIS extension only.
	defer func() {
		if rec := recover(); rec != nil {
			res.loaded = false
			res.err = fmt.Sprintf("panic: %v", rec)
			t.Errorf("extension %s panicked: %v", e.ID, rec)
		}
	}()

	if e.Code == "" {
		res.err = "no code URL"
		res.loaded = true // nothing to load; not a mechanics bug
		t.Logf("%s: no code URL, skipping", e.ID)
		return res
	}

	payload, err := fetchText(context.Background(), client, e.Code)
	if err != nil {
		// Dead upstream code URL — not a mechanics bug.
		res.loaded = true
		res.err = "code fetch failed: " + err.Error()
		t.Logf("%s: code fetch failed (dead upstream): %v", e.ID, err)
		return res
	}

	settings := resolveSettings(e.Options, nil)
	p, err := NewJSProviderWithDeps(e.ID, e.Name, payload, client, resolver, settings, testLogger(t))
	if err != nil {
		res.loaded = false
		res.err = "compile/load: " + err.Error()
		t.Errorf("%s: failed to load (mechanics bug): %v", e.ID, err)
		return res
	}
	res.loaded = true

	out, err := p.SmartSearch(context.Background(), opts)
	if err != nil {
		res.err = err.Error()
		if classifyError(err.Error()) {
			res.loaded = false
			t.Errorf("%s: mechanics bug during search: %v", e.ID, err)
		} else {
			t.Logf("%s: tolerated upstream/empty error: %v", e.ID, err)
		}
		return res
	}
	res.results = len(out)
	t.Logf("%s: %d results", e.ID, len(out))
	for i, r := range out {
		if i >= 3 {
			break
		}
		t.Logf("  [%d] %q seeders=%d res=%q magnet=%v", i, r.Name, r.Seeders, r.Resolution, r.Magnet != "")
	}
	return res
}

func fetchIndex(ctx context.Context, client *http.Client, url string) ([]IndexEntry, error) {
	body, err := fetchText(ctx, client, url)
	if err != nil {
		return nil, err
	}
	var entries []IndexEntry
	if err := json.Unmarshal([]byte(body), &entries); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	return entries, nil
}

func fetchText(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}
