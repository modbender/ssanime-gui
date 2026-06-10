package animedb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"

	"github.com/klauspost/compress/zstd"
)

// rawEntry mirrors the manami anime-offline-database entry shape. The minified
// dataset omits null-valued keys, so every field must tolerate being absent
// (animeSeason, episodes, picture and synonyms are all optional in practice).
// Verified against the live schema 2026-06-10:
//
//	type   ∈ {TV, MOVIE, OVA, ONA, SPECIAL, UNKNOWN}
//	status ∈ {FINISHED, ONGOING, UPCOMING, UNKNOWN}
//	animeSeason.season ∈ {SPRING, SUMMER, FALL, WINTER, UNDEFINED}
//	sources is a list of provider URLs; the AniList one is anilist.co/anime/<id>
type rawEntry struct {
	Sources     []string `json:"sources"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Episodes    int      `json:"episodes"`
	Status      string   `json:"status"`
	AnimeSeason *struct {
		Season string `json:"season"`
		Year   int    `json:"year"`
	} `json:"animeSeason"`
	Picture  string   `json:"picture"`
	Synonyms []string `json:"synonyms"`
}

// anilistSourceRe extracts the numeric AniList id from a sources URL such as
// "https://anilist.co/anime/1535".
var anilistSourceRe = regexp.MustCompile(`anilist\.co/anime/(\d+)`)

// anilistID returns the AniList id from an entry's sources, or 0 if none.
func anilistID(sources []string) int {
	for _, s := range sources {
		if m := anilistSourceRe.FindStringSubmatch(s); m != nil {
			if id, err := strconv.Atoi(m[1]); err == nil {
				return id
			}
		}
	}
	return 0
}

// loadFromFile opens the cached compressed dataset, decompresses it, and
// rebuilds the index. It is the file-backed entry point used by Start/refresh.
func (d *DB) loadFromFile(ctx context.Context) error {
	f, err := d.openCached()
	if err != nil {
		return err
	}
	defer f.Close()

	dec, err := zstd.NewReader(f)
	if err != nil {
		return fmt.Errorf("zstd reader: %w", err)
	}
	defer dec.Close()

	return d.loadFromReader(ctx, dec.IOReadCloser())
}

// loadFromReader streams JSON from r, builds the index, and swaps it in under
// the write lock. r is the already-decompressed JSON stream (plain JSON in
// tests, zstd-decoded at runtime). It walks the top-level object token by token
// to reach the "data" array, then decodes that array element-by-element so the
// whole parsed tree is never materialized at once. The decoded byte count is
// capped by maxDecodedBytes.
func (d *DB) loadFromReader(ctx context.Context, r io.Reader) error {
	dec := json.NewDecoder(io.LimitReader(r, maxDecodedBytes))

	if err := seekToDataArray(dec); err != nil {
		return err
	}

	// Pre-size for the current dataset (~40k indexed); grows if needed.
	out := make([]record, 0, 40000)
	for dec.More() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var e rawEntry
		if err := dec.Decode(&e); err != nil {
			return fmt.Errorf("decode entry: %w", err)
		}
		// Skip entries with no AniList source — they can't be added by AniList id.
		id := anilistID(e.Sources)
		if id == 0 {
			continue
		}
		out = append(out, newRecord(id, e))
	}

	d.mu.Lock()
	d.index = out
	d.ready = true
	d.mu.Unlock()

	d.logger.Info("animedb: index loaded", "entries", len(out))
	return nil
}

// seekToDataArray advances the decoder past the opening object brace and the
// keys preceding "data", stopping just after the "data" array's opening
// bracket so the caller can stream its elements with dec.More()/dec.Decode().
func seekToDataArray(dec *json.Decoder) error {
	// Opening "{" of the wrapper object.
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("read opening token: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected object, got %v", tok)
	}

	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return fmt.Errorf("read key: %w", err)
		}
		name, _ := key.(string)
		if name == "data" {
			// Consume the array's opening "[".
			open, err := dec.Token()
			if err != nil {
				return fmt.Errorf("read data array open: %w", err)
			}
			if delim, ok := open.(json.Delim); !ok || delim != '[' {
				return fmt.Errorf("expected data array, got %v", open)
			}
			return nil
		}
		// Not "data": skip its value (scalar, object, or array) wholesale.
		if err := skipValue(dec); err != nil {
			return fmt.Errorf("skip %q: %w", name, err)
		}
	}
	return fmt.Errorf("no %q array in dataset", "data")
}

// skipValue consumes exactly one JSON value from dec, descending through nested
// objects/arrays so the decoder lands on the next key.
func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil // scalar value already consumed
	}
	if delim != '{' && delim != '[' {
		return nil
	}
	depth := 1
	for depth > 0 {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		if dl, ok := t.(json.Delim); ok {
			switch dl {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	return nil
}

// newRecord builds the compact indexed record from a raw entry, precomputing
// normalized title/synonyms for allocation-light search.
func newRecord(id int, e rawEntry) record {
	res := Result{
		AniListID: id,
		Title:     e.Title,
		Synonyms:  e.Synonyms,
		Type:      mapType(e.Type),
		Status:    mapStatus(e.Status),
		Episodes:  e.Episodes,
		Picture:   e.Picture,
	}
	if e.AnimeSeason != nil {
		res.Season = mapSeason(e.AnimeSeason.Season)
		res.Year = e.AnimeSeason.Year
	}

	normSyn := make([]string, 0, len(e.Synonyms))
	for _, s := range e.Synonyms {
		if n := normalize(s); n != "" {
			normSyn = append(normSyn, n)
		}
	}
	return record{
		Result:       res,
		normTitle:    normalize(e.Title),
		normSynonyms: normSyn,
	}
}
