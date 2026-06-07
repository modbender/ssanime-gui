package poller

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// digits extracts the leading run of digits from a resolution string.
var digits = regexp.MustCompile(`\d+`)

// newUUID returns a random uuid for an episode row's uuid column.
func newUUID() string { return uuid.NewString() }

// dedupeKey is the stable identity of a release within a feed's seen_cache. The
// info hash is the strongest key; fall back to the release name so a result with
// no hash yet still dedupes.
func dedupeKey(t *source.AnimeTorrent) string {
	if t.InfoHash != "" {
		return "ih:" + strings.ToLower(t.InfoHash)
	}
	return "name:" + strings.ToLower(strings.TrimSpace(t.Name))
}

// loadSeenCache parses a feed's seen_cache JSON (a JSON array of keys) into a
// set. A nil/blank/invalid cache yields an empty set rather than an error — the
// cache is an optimization, not durable truth.
func loadSeenCache(raw *string) map[string]struct{} {
	out := map[string]struct{}{}
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return out
	}
	var keys []string
	if err := json.Unmarshal([]byte(*raw), &keys); err != nil {
		return out
	}
	for _, k := range keys {
		out[k] = struct{}{}
	}
	return out
}

// dumpSeenCache serializes a seen set back to a JSON array, capped so a long-
// running feed's cache can't grow without bound. Newer keys are kept.
func dumpSeenCache(seen map[string]struct{}) string {
	const cap = 500
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	if len(keys) > cap {
		keys = keys[len(keys)-cap:]
	}
	b, err := json.Marshal(keys)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// mediaFromSeries builds the source.Media used to drive SmartSearch + autoselect
// from a series row's cached AniList metadata.
func mediaFromSeries(s store.Series) source.Media {
	m := source.Media{
		ID:           derefInt(s.AnilistID),
		RomajiTitle:  deref(s.RomajiTitle),
		EpisodeCount: -1,
	}
	if s.MalID != nil {
		v := int(*s.MalID)
		m.IDMal = &v
	}
	if s.EnglishTitle != nil {
		m.EnglishTitle = s.EnglishTitle
	}
	if s.Status != nil {
		m.Status = *s.Status
	}
	if s.Format != nil {
		m.Format = *s.Format
	}
	if s.EpisodeCount != nil {
		m.EpisodeCount = int(*s.EpisodeCount)
	}
	m.Synonyms = parseSynonyms(s.Synonyms)
	// Always include the canonical title as a fallback synonym for matching.
	if s.Title != "" {
		m.Synonyms = append(m.Synonyms, s.Title)
	}
	return m
}

// parseSynonyms reads the series.synonyms JSON array column.
func parseSynonyms(raw *string) []string {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	var syn []string
	if err := json.Unmarshal([]byte(*raw), &syn); err != nil {
		return nil
	}
	return syn
}

// feedQuery returns the explicit search query for a feed: its title_regex when
// it looks like a plain query, else empty (let SmartSearch use the media title).
func feedQuery(feed store.Feed, _ store.Series) string {
	if feed.TitleRegex != nil {
		q := strings.TrimSpace(*feed.TitleRegex)
		// title_regex is a filter, not a query; only treat it as a query if it
		// has no regex metacharacters (a bare phrase the user wants searched).
		if q != "" && !strings.ContainsAny(q, `\^$.|?*+()[]{}`) {
			return q
		}
	}
	return ""
}

// resolutionFilter returns the feed's quality as a habari-style resolution
// string ("1080"), or "" when the feed sets no quality constraint.
func resolutionFilter(feed store.Feed) string {
	if feed.Quality == nil || *feed.Quality == 0 {
		return ""
	}
	return strconv.FormatInt(*feed.Quality, 10)
}

// isCompleted reports whether a FINISHED series has every aired episode archived,
// in which case polling should stop (the "completed -> no auto-poll" rule that
// the cheap SQL filter can't compute). Best-effort: any error returns false so
// we keep polling rather than silently stop.
func (p *Poller) isCompleted(ctx context.Context, s store.Series) bool {
	if s.AiringStatus == nil || *s.AiringStatus != "FINISHED" {
		return false
	}
	if s.EpisodeCount == nil || *s.EpisodeCount <= 0 {
		return false
	}
	episodes, err := p.store.Read().ListEpisodesBySeries(ctx, s.ID)
	if err != nil {
		return false
	}
	archived := 0
	for _, e := range episodes {
		if e.Status == "archived" {
			archived++
		}
	}
	return int64(archived) >= *s.EpisodeCount
}

func resolutionInt(res string) int {
	d := digits.FindString(res)
	if d == "" {
		return 0
	}
	n, _ := strconv.Atoi(d)
	return n
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(i *int64) int {
	if i == nil {
		return 0
	}
	return int(*i)
}
