package animedb

import (
	"regexp"
	"sort"
	"strings"
)

// nonAlnum collapses every run of non-alphanumeric characters to a single
// space. Mirrors source.normalizeTitle's approach (kept local to avoid a
// cross-package dependency for one small helper).
var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// normalize lowercases and strips punctuation for fuzzy title comparison, so
// "Sousou no Frieren" and "Frieren: Sousou no" compare on equal footing.
func normalize(s string) string {
	return strings.TrimSpace(nonAlnum.ReplaceAllString(strings.ToLower(s), " "))
}

// rank scores how well a normalized candidate matches the normalized query.
// Higher is better; 0 means no match. Exact > prefix > substring, and a title
// hit outranks the same tier on a synonym.
const (
	rankNone       = 0
	rankSubstring  = 1
	rankPrefix     = 2
	rankExact      = 3
	titleTierBonus = 10 // title matches sort ahead of synonym matches
)

// scoreField returns the match tier of one normalized field against nq.
func scoreField(field, nq string) int {
	switch {
	case field == "":
		return rankNone
	case field == nq:
		return rankExact
	case strings.HasPrefix(field, nq):
		return rankPrefix
	case strings.Contains(field, nq):
		return rankSubstring
	default:
		return rankNone
	}
}

// score returns the best match score for a record against the normalized query,
// weighting title matches above synonym matches of the same tier.
func (r record) score(nq string) int {
	best := scoreField(r.normTitle, nq)
	if best > 0 {
		best += titleTierBonus
	}
	for _, syn := range r.normSynonyms {
		if s := scoreField(syn, nq); s > best {
			best = s
		}
	}
	return best
}

// Search normalizes query, scans the index, and returns up to limit results
// ranked by match quality (exact title > prefix title > substring title >
// synonym tiers), ties broken by title for stable output. A linear scan over
// ~40k entries is fast enough because search runs on submit, not per-keystroke.
func (d *DB) Search(query string, limit int) []Result {
	nq := normalize(query)
	if nq == "" || limit <= 0 {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	type scored struct {
		rec   *record
		score int
	}
	hits := make([]scored, 0, limit*4)
	for i := range d.index {
		if s := d.index[i].score(nq); s > 0 {
			hits = append(hits, scored{rec: &d.index[i], score: s})
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].rec.Title < hits[j].rec.Title
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Result, len(hits))
	for i, h := range hits {
		out[i] = h.rec.Result
	}
	return out
}

// statusMap translates manami status vocabulary to AniList airing-status
// vocabulary. UNKNOWN (and anything unrecognized) maps to "".
var statusMap = map[string]string{
	"FINISHED": "FINISHED",
	"ONGOING":  "RELEASING",
	"UPCOMING": "NOT_YET_RELEASED",
}

// mapStatus returns the AniList-style status for a manami status, or "".
func mapStatus(s string) string {
	return statusMap[s]
}

// typePassthrough is the set of manami types that already match AniList format
// vocabulary and pass through unchanged. UNKNOWN / unrecognized → "".
var typePassthrough = map[string]bool{
	"TV":      true,
	"MOVIE":   true,
	"OVA":     true,
	"ONA":     true,
	"SPECIAL": true,
}

// mapType returns the AniList-style format for a manami type, or "".
func mapType(t string) string {
	if typePassthrough[t] {
		return t
	}
	return ""
}

// mapSeason normalizes the manami season: the four real seasons pass through,
// UNDEFINED / unrecognized → "".
var seasonPassthrough = map[string]bool{
	"SPRING": true,
	"SUMMER": true,
	"FALL":   true,
	"WINTER": true,
}

func mapSeason(s string) string {
	if seasonPassthrough[s] {
		return s
	}
	return ""
}
