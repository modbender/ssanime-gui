package source

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// TrustedReleaseGroups is the DEFAULT original-source subbing-group allowlist. The
// app re-encodes itself, so we want clean, untouched original releases — not remuxes
// or re-encodes from secondary groups. Ordered by preference (earlier = better).
// Callers may override this per-call via SelectOptions.TrustedGroups (the
// user-configured list); this var is the fallback when that is nil/empty.
var TrustedReleaseGroups = []string{
	"SubsPlease",
	"Erai-raws",
}

// effectiveTrusted returns the trusted-group list to rank/filter against. A nil
// opts.TrustedGroups means "not configured" → the package default. A non-nil slice
// (including an explicitly-empty one) is taken verbatim: an empty user list is the
// "no trust filter" signal and must NOT silently fall back to the default.
func effectiveTrusted(opts SelectOptions) []string {
	if opts.TrustedGroups == nil {
		return TrustedReleaseGroups
	}
	return opts.TrustedGroups
}

// trustedRankIn returns the preference index of a group within trusted (lower =
// better), or -1 if absent (case-insensitive).
func trustedRankIn(group string, trusted []string) int {
	for i, g := range trusted {
		if strings.EqualFold(g, group) {
			return i
		}
	}
	return -1
}

const (
	scoreTrustedBase  = 100 // best trusted group
	scoreTrustedDecay = 10  // each step down the trusted list
	scoreBestRelease  = 40
	scoreConfirmed    = 30
	scoreEpisodeMatch = 20
)

// SelectOptions tunes SelectBest. Zero values mean "no constraint".
type SelectOptions struct {
	// Resolution, e.g. "1080" or "1080p": a hard filter when set.
	Resolution string
	// Episode is the wanted episode number; a single-episode release matching it
	// scores higher, and non-matching single episodes are dropped when > 0.
	Episode int
	// PreferBatch keeps batch releases instead of single episodes.
	PreferBatch bool
	// RequireTrustedGroup drops any release not from a trusted group — UNLESS the
	// effective trusted list (TrustedGroups, or the package default when that is
	// empty) is itself empty, in which case there is nothing to allow and this
	// filter is skipped so selection falls back to best-available instead of
	// dropping everything.
	RequireTrustedGroup bool
	// TrustedGroups overrides the package-level TrustedReleaseGroups default for
	// this call (the user-configured allowlist). Nil/empty falls back to the
	// default. An explicitly-empty user list reaches selection as an empty slice
	// only after the caller resolves the default away; within selection, an empty
	// effective list disables the trusted-only hard filter (see RequireTrustedGroup).
	TrustedGroups []string
	// Group, when set, is a hard filter: only releases whose ReleaseGroup equals it
	// (case-insensitive) pass. Used for the per-series locked-group stage.
	Group string
	// MinSeeders drops releases below this seeder count (0 disables; -1 seeders
	// means "unknown" and always passes).
	MinSeeders int
}

// SelectBest picks the best original release for the media/episode from torrents:
// it filters to the target resolution (and, optionally, trusted groups + the
// wanted episode), then ranks by trusted-group preference, best-release /
// confirmed flags, and finally seeders. Returns an error when nothing qualifies.
func SelectBest(media Media, torrents []*AnimeTorrent, opts SelectOptions) (*AnimeTorrent, error) {
	if len(torrents) == 0 {
		return nil, fmt.Errorf("autoselect: no torrents to choose from")
	}

	titles := media.Titles()
	trusted := effectiveTrusted(opts)
	candidates := make([]*AnimeTorrent, 0, len(torrents))
	for _, t := range torrents {
		if t == nil {
			continue
		}
		// Resolution hard filter.
		if opts.Resolution != "" && !resolutionEqual(t.Resolution, opts.Resolution) {
			continue
		}
		// Trusted-group hard filter (optional). Skipped when the effective trusted
		// list is empty: there is nothing to allow, so fall back to best-available
		// rather than dropping every release.
		if opts.RequireTrustedGroup && len(trusted) > 0 && trustedRankIn(t.ReleaseGroup, trusted) < 0 {
			continue
		}
		// Locked-group hard filter (optional).
		if opts.Group != "" && !strings.EqualFold(t.ReleaseGroup, opts.Group) {
			continue
		}
		// Seeder floor (unknown seeders == -1 always passes).
		if opts.MinSeeders > 0 && t.Seeders >= 0 && t.Seeders < opts.MinSeeders {
			continue
		}
		// Episode constraint.
		if opts.Episode > 0 {
			if opts.PreferBatch {
				if !t.IsBatch {
					continue
				}
			} else if !t.IsBatch && t.EpisodeNumber != opts.Episode {
				continue
			}
		} else if opts.PreferBatch && !t.IsBatch {
			continue
		}
		// Title sanity check: when we have titles and the result isn't already
		// confirmed, require a loose title match so a seeders-sorted feed of
		// unrelated shows can't slip through.
		if len(titles) > 0 && !t.Confirmed && !titleMatches(t.Name, titles) {
			continue
		}
		candidates = append(candidates, t)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("autoselect: no release matched (res=%q episode=%d batch=%v)",
			opts.Resolution, opts.Episode, opts.PreferBatch)
	}

	slices.SortStableFunc(candidates, func(a, b *AnimeTorrent) int {
		sa, sb := score(a, opts, trusted), score(b, opts, trusted)
		if sa != sb {
			return cmp.Compare(sb, sa) // higher score first
		}
		return cmp.Compare(b.Seeders, a.Seeders) // then more seeders
	})
	return candidates[0], nil
}

// score ranks one candidate by trusted group, best-release/confirmed flags, and
// exact episode match. trusted is the effective allowlist (empty = no trust bonus).
// Seeders are the final tie-break in SelectBest, not here.
func score(t *AnimeTorrent, opts SelectOptions, trusted []string) int {
	s := 0
	if rank := trustedRankIn(t.ReleaseGroup, trusted); rank >= 0 {
		s += scoreTrustedBase - rank*scoreTrustedDecay
	}
	if t.IsBestRelease {
		s += scoreBestRelease
	}
	if t.Confirmed {
		s += scoreConfirmed
	}
	if opts.Episode > 0 && !t.IsBatch && t.EpisodeNumber == opts.Episode {
		s += scoreEpisodeMatch
	}
	return s
}
