package source

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/5rahim/habari"
)

// trackers are appended when building a magnet from a bare info hash (providers
// that only expose an info hash, e.g. nyaa RSS items without a magnet element).
var trackers = []string{
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.stealth.si:80/announce",
	"udp://exodus.desync.com:6969/announce",
}

var (
	// infoHashRe captures the btih value, which may be hex (40 chars) or, as
	// SubsPlease emits, base32 (32 chars, A-Z2-7).
	infoHashRe = regexp.MustCompile(`(?i)btih:([A-Za-z0-9]+)`)
	episodeRe  = regexp.MustCompile(`\d+`)
)

// enrich runs habari over t.Name and fills any structured field the provider
// left unset. Provider-supplied values win over the parse (the provider knows
// its own data best), so this only backfills.
func enrich(t *AnimeTorrent) {
	m := habari.Parse(t.Name)
	if t.ReleaseGroup == "" {
		t.ReleaseGroup = m.ReleaseGroup
	}
	if t.Resolution == "" {
		t.Resolution = m.VideoResolution
	}
	if t.EpisodeNumber == 0 {
		t.EpisodeNumber = parseEpisode(m)
	}
	if !t.IsBatch {
		t.IsBatch = isBatch(m)
	}
	if t.InfoHash == "" && t.Magnet != "" {
		if mm := infoHashRe.FindStringSubmatch(t.Magnet); mm != nil {
			t.InfoHash = strings.ToLower(mm[1])
		}
	}
}

// parseEpisode returns the single episode number from a parse, or -1 when the
// release has no single episode (a batch/range or an unparseable name).
func parseEpisode(m *habari.Metadata) int {
	if len(m.EpisodeNumber) != 1 {
		return -1
	}
	digits := episodeRe.FindString(m.EpisodeNumber[0])
	if digits == "" {
		return -1
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return -1
	}
	return n
}

// isBatch reports whether a parse looks like a multi-episode batch: an episode
// range (two numbers) or a season/volume pack with no single episode.
func isBatch(m *habari.Metadata) bool {
	if len(m.EpisodeNumber) > 1 {
		return true
	}
	if len(m.VolumeNumber) > 0 {
		return true
	}
	return false
}

// resolutionInt converts a habari resolution string ("1080p", "1080") to an int,
// or 0 when it can't be read. Used to populate episodes.resolution.
func resolutionInt(res string) int {
	digits := episodeRe.FindString(res)
	if digits == "" {
		return 0
	}
	n, _ := strconv.Atoi(digits)
	return n
}

// buildMagnet constructs a magnet URI from an info hash and display name when a
// provider exposes only the hash (nyaa RSS without a magnet element).
func buildMagnet(infoHash, displayName string) string {
	if infoHash == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "magnet:?xt=urn:btih:%s&dn=%s", infoHash, url.QueryEscape(displayName))
	for _, tr := range trackers {
		fmt.Fprintf(&b, "&tr=%s", url.QueryEscape(tr))
	}
	return b.String()
}

// ensureMagnet returns a usable magnet for t, building one from its info hash if
// no direct magnet is present.
func ensureMagnet(t *AnimeTorrent) string {
	if strings.HasPrefix(strings.ToLower(t.Magnet), "magnet:?") {
		return t.Magnet
	}
	if strings.HasPrefix(strings.ToLower(t.Link), "magnet:?") {
		return t.Link
	}
	return buildMagnet(t.InfoHash, t.Name)
}
