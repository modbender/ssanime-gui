package source

import (
	"regexp"
	"strings"
	"time"
)

// preferredTitle picks the best title to search a provider with: romaji first
// (release groups name files in romaji), then english, then the first synonym.
func preferredTitle(m Media) string {
	if m.RomajiTitle != "" {
		return m.RomajiTitle
	}
	if m.EnglishTitle != nil && *m.EnglishTitle != "" {
		return *m.EnglishTitle
	}
	if len(m.Synonyms) > 0 {
		return m.Synonyms[0]
	}
	return ""
}

// nonAlnum collapses everything that isn't a letter/digit to a single space,
// for loose title comparison ("Sousou no Frieren" vs "Frieren - Sousou no").
var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// normalizeTitle lowercases and strips punctuation for fuzzy title comparison.
func normalizeTitle(s string) string {
	return strings.TrimSpace(nonAlnum.ReplaceAllString(strings.ToLower(s), " "))
}

// titleMatches reports whether the release title plausibly belongs to one of the
// media's titles: a normalized substring match in either direction. This is the
// lightweight matcher that turns raw nyaa titles into "is this the right show?"
// — habari already gave us the parsed series title in name.
func titleMatches(releaseName string, titles []string) bool {
	name := normalizeTitle(releaseName)
	if name == "" {
		return false
	}
	for _, t := range titles {
		nt := normalizeTitle(t)
		if nt == "" {
			continue
		}
		if strings.Contains(name, nt) || strings.Contains(nt, name) {
			return true
		}
	}
	return false
}

// Filter applies the SmartSearch filters (media title, episode, resolution,
// batch) to an already-fetched result set, without re-querying a provider. It is
// the same narrowing the native providers run inside SmartSearch, exposed for
// callers that post-process results.
func Filter(results []*AnimeTorrent, opts SmartSearchOptions) []*AnimeTorrent {
	return filterSmart(results, opts)
}

// filterSmart applies the SmartSearch filters (media title, episode, resolution,
// batch) to a set of habari-enriched results. Provider-agnostic so both Nyaa and
// SubsPlease reuse it.
func filterSmart(results []*AnimeTorrent, opts SmartSearchOptions) []*AnimeTorrent {
	titles := opts.Media.Titles()
	// When the user typed an explicit query we trust it and skip title matching;
	// a custom query is the user overriding the metadata.
	matchTitle := opts.Query == "" && len(titles) > 0

	out := results[:0:0]
	for _, t := range results {
		if matchTitle && !titleMatches(t.Name, titles) {
			continue
		}
		if opts.Batch && !t.IsBatch {
			continue
		}
		if !opts.Batch && opts.EpisodeNumber > 0 {
			// A batch may still contain the episode; keep batches, and keep the
			// exact single-episode match.
			if !t.IsBatch && t.EpisodeNumber != opts.EpisodeNumber {
				continue
			}
		}
		if opts.Resolution != "" && !resolutionEqual(t.Resolution, opts.Resolution) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// resolutionEqual compares resolutions ignoring a trailing "p" ("1080p" == "1080").
func resolutionEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.ToLower(a), "p"),
		strings.TrimSuffix(strings.ToLower(b), "p"))
}

// parseTime is a thin wrapper kept here so providers don't import time directly
// just for one format parse.
func parseTime(layout, value string) (time.Time, error) {
	return time.Parse(layout, value)
}
