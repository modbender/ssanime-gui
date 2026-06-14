package extension

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/source"
)

// stubResolver returns a canned IDs block (and optionally records the call).
type stubResolver struct {
	ids    anizip.IDs
	err    error
	calls  int
}

func (s *stubResolver) GetIDs(_ context.Context, anilistID int) (anizip.IDs, error) {
	s.calls++
	if s.err != nil {
		return anizip.IDs{}, s.err
	}
	return s.ids, nil
}

// echoSingleJS is an extension whose single(options) echoes the received options
// object (JSON) into the torrent name so the test can assert exact keys/values.
const echoSingleJS = `export default new class {
  async single(o, settings) {
    return [{name: JSON.stringify({opts: o, settings: settings}), magnetLink: "` + magnet40 + `"}];
  }
  // Alias the Hayase primaries to single so option-building tests work
  // regardless of which primary the adapter selects (batch/movie/single).
  batch = this.single;
  movie = this.single;
}`

// echoOpts runs a SmartSearch through the echo extension and returns the decoded
// options object and settings the JS received.
func echoOpts(t *testing.T, resolver IDResolver, settings map[string]interface{}, opts source.SmartSearchOptions) (map[string]interface{}, map[string]interface{}) {
	t.Helper()
	p, err := NewJSProviderWithDeps("echo", "Echo", echoSingleJS, http.DefaultClient, resolver, settings, testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProviderWithDeps: %v", err)
	}
	res, err := p.SmartSearch(context.Background(), opts)
	if err != nil {
		t.Fatalf("SmartSearch: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("no results")
	}
	var decoded struct {
		Opts     map[string]interface{} `json:"opts"`
		Settings map[string]interface{} `json:"settings"`
	}
	if err := json.Unmarshal([]byte(res[0].Name), &decoded); err != nil {
		t.Fatalf("decode echoed options from %q: %v", res[0].Name, err)
	}
	return decoded.Opts, decoded.Settings
}

func intVal(t *testing.T, m map[string]interface{}, key string) int {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("option %q missing from %v", key, m)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("option %q = %T(%v), want number", key, v, v)
	}
	return int(f)
}

func TestBuildOptionsAllIDsPresent(t *testing.T) {
	r := &stubResolver{ids: anizip.IDs{
		AnilistID: 21, AnidbID: 69, MalID: 21, TvdbID: 81797, TmdbID: 37854,
		Episodes: map[int]anizip.EpisodeIDs{1: {AnidbEid: 440, TvdbEid: 5505123}},
	}}
	eng := "One Piece"
	opts, _ := echoOpts(t, r, nil, source.SmartSearchOptions{
		Media:         source.Media{ID: 21, RomajiTitle: "One Piece", EnglishTitle: &eng, EpisodeCount: 1100, Format: "TV"},
		EpisodeNumber: 1,
		Resolution:    "1080",
	})

	if intVal(t, opts, "anilistId") != 21 {
		t.Errorf("anilistId = %v", opts["anilistId"])
	}
	if intVal(t, opts, "anidbAid") != 69 {
		t.Errorf("anidbAid = %v", opts["anidbAid"])
	}
	if intVal(t, opts, "anidbEid") != 440 {
		t.Errorf("anidbEid = %v", opts["anidbEid"])
	}
	if intVal(t, opts, "malId") != 21 {
		t.Errorf("malId = %v", opts["malId"])
	}
	if intVal(t, opts, "tvdbId") != 81797 {
		t.Errorf("tvdbId = %v", opts["tvdbId"])
	}
	// Capitalization is load-bearing: Hayase reads tvdbEId (capital E then Id).
	if _, ok := opts["tvdbEId"]; !ok {
		t.Fatalf("tvdbEId key missing (capitalization!); keys = %v", keysOf(opts))
	}
	if intVal(t, opts, "tvdbEId") != 5505123 {
		t.Errorf("tvdbEId = %v", opts["tvdbEId"])
	}
	if intVal(t, opts, "tmdbId") != 37854 {
		t.Errorf("tmdbId = %v", opts["tmdbId"])
	}
	if opts["resolution"] != "1080" {
		t.Errorf("resolution = %v, want 1080", opts["resolution"])
	}
	if intVal(t, opts, "episode") != 1 {
		t.Errorf("episode = %v", opts["episode"])
	}
	if intVal(t, opts, "episodeCount") != 1100 {
		t.Errorf("episodeCount = %v", opts["episodeCount"])
	}
	// titles present.
	if titles, ok := opts["titles"].([]interface{}); !ok || len(titles) == 0 {
		t.Errorf("titles = %v, want non-empty", opts["titles"])
	}
	if r.calls != 1 {
		t.Errorf("resolver called %d times, want exactly 1", r.calls)
	}
}

func TestBuildOptionsSeadexOnlyAnilist(t *testing.T) {
	// seadex's resolver returns only AnilistID; the rest stay 0.
	r := &stubResolver{ids: anizip.IDs{AnilistID: 21}}
	opts, _ := echoOpts(t, r, nil, source.SmartSearchOptions{
		Media:         source.Media{ID: 21, RomajiTitle: "One Piece", EpisodeCount: 1},
		EpisodeNumber: 1,
	})
	if intVal(t, opts, "anilistId") != 21 {
		t.Errorf("anilistId = %v", opts["anilistId"])
	}
	if intVal(t, opts, "anidbAid") != 0 || intVal(t, opts, "tvdbId") != 0 || intVal(t, opts, "tmdbId") != 0 {
		t.Errorf("non-anilist ids should be 0: %v", opts)
	}
}

func TestBuildOptionsNekobtTvdbTmdbOnly(t *testing.T) {
	r := &stubResolver{ids: anizip.IDs{
		AnilistID: 21, TvdbID: 81797, TmdbID: 37854,
		Episodes: map[int]anizip.EpisodeIDs{5: {TvdbEid: 999}},
	}}
	opts, _ := echoOpts(t, r, nil, source.SmartSearchOptions{
		Media:         source.Media{ID: 21, RomajiTitle: "x", EpisodeCount: 10, Format: "TV"},
		EpisodeNumber: 5,
	})
	if intVal(t, opts, "tvdbId") != 81797 {
		t.Errorf("tvdbId = %v", opts["tvdbId"])
	}
	if intVal(t, opts, "tvdbEId") != 999 {
		t.Errorf("tvdbEId = %v", opts["tvdbEId"])
	}
	if intVal(t, opts, "anidbAid") != 0 {
		t.Errorf("anidbAid should be 0, got %v", opts["anidbAid"])
	}
}

func TestBuildOptionsResolverNilFallsBackToMedia(t *testing.T) {
	mal := 21
	opts, _ := echoOpts(t, nil, nil, source.SmartSearchOptions{
		Media: source.Media{
			ID: 21, RomajiTitle: "x", IDMal: &mal, AnidbAID: 7, AnidbEID: 8,
			TvdbID: 100, TmdbID: 200, EpisodeCount: 12,
		},
		EpisodeNumber: 1,
	})
	if intVal(t, opts, "anidbAid") != 7 {
		t.Errorf("anidbAid fallback = %v, want 7", opts["anidbAid"])
	}
	if intVal(t, opts, "anidbEid") != 8 {
		t.Errorf("anidbEid fallback = %v, want 8", opts["anidbEid"])
	}
	if intVal(t, opts, "tvdbId") != 100 {
		t.Errorf("tvdbId fallback = %v, want 100", opts["tvdbId"])
	}
	if intVal(t, opts, "tmdbId") != 200 {
		t.Errorf("tmdbId fallback = %v, want 200", opts["tmdbId"])
	}
	if intVal(t, opts, "malId") != 21 {
		t.Errorf("malId fallback = %v, want 21", opts["malId"])
	}
}

func TestBuildOptionsResolverErrorDegradesNotFails(t *testing.T) {
	r := &stubResolver{err: context.DeadlineExceeded}
	opts, _ := echoOpts(t, r, nil, source.SmartSearchOptions{
		Media:         source.Media{ID: 21, RomajiTitle: "x", AnidbAID: 7, EpisodeCount: 1},
		EpisodeNumber: 1,
	})
	// Resolver failed → fall back to Media ids, no error.
	if intVal(t, opts, "anidbAid") != 7 {
		t.Errorf("anidbAid = %v, want 7 (media fallback on resolver error)", opts["anidbAid"])
	}
}

func TestBuildOptionsExclusionsEmptyNotNull(t *testing.T) {
	opts, _ := echoOpts(t, nil, nil, source.SmartSearchOptions{
		Media: source.Media{ID: 21, RomajiTitle: "x"},
	})
	excl, ok := opts["exclusions"]
	if !ok || excl == nil {
		t.Fatalf("exclusions = %v, want [] not null", excl)
	}
	if _, ok := excl.([]interface{}); !ok {
		t.Errorf("exclusions = %T, want array", excl)
	}
}

func TestBuildOptionsExclusionsPassedThrough(t *testing.T) {
	opts, _ := echoOpts(t, nil, nil, source.SmartSearchOptions{
		Media:      source.Media{ID: 21, RomajiTitle: "x"},
		Exclusions: []string{"HEVC", "x265"},
	})
	arr, ok := opts["exclusions"].([]interface{})
	if !ok || len(arr) != 2 {
		t.Fatalf("exclusions = %v, want 2 elements", opts["exclusions"])
	}
}

func TestBuildOptionsBatchReflected(t *testing.T) {
	for _, batch := range []bool{true, false} {
		opts, _ := echoOpts(t, nil, nil, source.SmartSearchOptions{
			Media: source.Media{ID: 21, RomajiTitle: "x", Format: "TV"},
			Batch: batch,
		})
		if opts["batch"] != batch {
			t.Errorf("batch = %v, want %v", opts["batch"], batch)
		}
	}
}

func TestBuildOptionsResolutionEmpty(t *testing.T) {
	opts, _ := echoOpts(t, nil, nil, source.SmartSearchOptions{
		Media: source.Media{ID: 21, RomajiTitle: "x"},
	})
	if opts["resolution"] != "" {
		t.Errorf("resolution = %v, want empty string", opts["resolution"])
	}
}

func TestSettingsPassedToJS(t *testing.T) {
	settings := map[string]interface{}{"useTorrent": true, "region": "us"}
	_, gotSettings := echoOpts(t, nil, settings, source.SmartSearchOptions{
		Media: source.Media{ID: 21, RomajiTitle: "x"},
	})
	if gotSettings["useTorrent"] != true {
		t.Errorf("settings.useTorrent = %v, want true", gotSettings["useTorrent"])
	}
	if gotSettings["region"] != "us" {
		t.Errorf("settings.region = %v, want us", gotSettings["region"])
	}
}

// --- Method selection + fallback ---

// methodEchoJS exposes the named methods; each echoes its own name so the test
// can assert which one was chosen. Only the methods listed in `present` exist.
func methodEchoExtension(present ...string) string {
	var b strings.Builder
	b.WriteString("export default new class {\n")
	for _, m := range present {
		b.WriteString("  async " + m + "(o, s) { return [{name: \"" + m + "\", magnetLink: \"" + magnet40 + "\"}]; }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func runMethod(t *testing.T, js string, opts source.SmartSearchOptions) (string, error) {
	t.Helper()
	p, err := NewJSProviderWithDeps("m", "M", js, http.DefaultClient, nil, nil, testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProviderWithDeps: %v", err)
	}
	res, err := p.SmartSearch(context.Background(), opts)
	if err != nil {
		return "", err
	}
	if len(res) == 0 {
		t.Fatal("no results")
	}
	return res[0].Name, nil
}

func TestMethodSelectionSingle(t *testing.T) {
	got, err := runMethod(t, methodEchoExtension("single", "batch"), source.SmartSearchOptions{
		Media: source.Media{ID: 1, RomajiTitle: "x", Format: "TV"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "single" {
		t.Errorf("chose %q, want single", got)
	}
}

func TestMethodSelectionBatch(t *testing.T) {
	got, err := runMethod(t, methodEchoExtension("single", "batch"), source.SmartSearchOptions{
		Media: source.Media{ID: 1, RomajiTitle: "x", Format: "TV"},
		Batch: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "batch" {
		t.Errorf("chose %q, want batch", got)
	}
}

func TestMethodSelectionMovie(t *testing.T) {
	got, err := runMethod(t, methodEchoExtension("single", "movie"), source.SmartSearchOptions{
		Media: source.Media{ID: 1, RomajiTitle: "x", Format: "MOVIE"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "movie" {
		t.Errorf("chose %q, want movie", got)
	}
}

func TestMethodSelectionPrimaryAbsentFallsToSearch(t *testing.T) {
	// No single; legacy "search" present → used.
	got, err := runMethod(t, methodEchoExtension("search"), source.SmartSearchOptions{
		Media: source.Media{ID: 1, RomajiTitle: "x", Format: "TV"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "search" {
		t.Errorf("chose %q, want search fallback", got)
	}
}

func TestMethodRejectionSurfacesNotSwallowed(t *testing.T) {
	// single exists and throws; the error must surface, NOT fall through to search.
	js := `export default new class {
		async single(o, s) { throw new Error("upstream-boom"); }
		async search(o, s) { return [{name: "search", magnetLink: "` + magnet40 + `"}]; }
	}`
	_, err := runMethod(t, js, source.SmartSearchOptions{
		Media: source.Media{ID: 1, RomajiTitle: "x", Format: "TV"},
	})
	if err == nil {
		t.Fatal("expected the single() rejection to surface")
	}
	if !strings.Contains(err.Error(), "upstream-boom") {
		t.Errorf("error = %q, want it to contain upstream-boom (not masked by search)", err)
	}
}

func TestMethodNoneFound(t *testing.T) {
	js := `export default new class { async other() { return []; } }`
	_, err := runMethod(t, js, source.SmartSearchOptions{
		Media: source.Media{ID: 1, RomajiTitle: "x", Format: "TV"},
	})
	if err == nil {
		t.Fatal("expected a 'none of methods' error")
	}
	if !strings.Contains(err.Error(), "none of methods") {
		t.Errorf("error = %q, want 'none of methods'", err)
	}
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
