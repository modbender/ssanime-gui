package source

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// TrustedReleaseGroups are the original-source subbing groups we prefer. The app
// re-encodes itself, so we want clean, untouched original releases — not remuxes
// or re-encodes from secondary groups. Ordered by preference (earlier = better);
// this drives a tie-break bonus, not a hard filter, so an unknown group can still
// win on seeders when no trusted group is present.
var TrustedReleaseGroups = []string{
	"SubsPlease",
	"Erai-raws",
	"ASW",
	"EMBER",
	"Judas",
	"Anime Time",
	"Tsundere-Raws",
}

// trustedRank returns the preference index of a group (lower = better), or -1 if
// the group is not in the trusted list.
func trustedRank(group string) int {
	for i, g := range TrustedReleaseGroups {
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
	// RequireTrustedGroup drops any release not from a trusted group.
	RequireTrustedGroup bool
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
	candidates := make([]*AnimeTorrent, 0, len(torrents))
	for _, t := range torrents {
		if t == nil {
			continue
		}
		// Resolution hard filter.
		if opts.Resolution != "" && !resolutionEqual(t.Resolution, opts.Resolution) {
			continue
		}
		// Trusted-group hard filter (optional).
		if opts.RequireTrustedGroup && trustedRank(t.ReleaseGroup) < 0 {
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
		sa, sb := score(a, opts), score(b, opts)
		if sa != sb {
			return cmp.Compare(sb, sa) // higher score first
		}
		return cmp.Compare(b.Seeders, a.Seeders) // then more seeders
	})
	return candidates[0], nil
}

// score ranks one candidate by trusted group, best-release/confirmed flags, and
// exact episode match. Seeders are the final tie-break in SelectBest, not here.
func score(t *AnimeTorrent, opts SelectOptions) int {
	s := 0
	if rank := trustedRank(t.ReleaseGroup); rank >= 0 {
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
