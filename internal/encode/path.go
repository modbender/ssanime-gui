package encode

import (
	"path/filepath"
	"strconv"
	"strings"
)

// illegalPathChars are stripped from a series title before it is used as a
// filesystem path segment (Jellyfin/Plex sanitization).
const illegalPathChars = `\/:*?"<>|`

// reservedNames are Windows device names that can't be used as a path segment
// (case-insensitive, with or without an extension). A matching segment is
// prefixed with "_" so e.g. a series literally titled "CON" yields a valid path.
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// PathParams are the inputs to the Jellyfin/Plex library path builder.
type PathParams struct {
	EncodedRoot string // settings.encoded_root
	Series      string // display title (English/romaji), pre-sanitization
	Season      int    // series.season_number
	Episode     int    // episodes.episode_no (0 for specials/movies/OVAs)
	IsSpecial   bool   // episode_no IS NULL -> Season 00 / S00E..
	Resolution  int    // 1080/720/480 -> "<res>p" subfolder
	Ext         string // container extension without dot (e.g. "mkv")
	// Template is settings.naming_template; when empty the Jellyfin default is
	// used. Tokens: {series} {season} {episode} {res} {ext}.
	Template string
}

// defaultNamingTemplate mirrors the seeded settings.naming_template; used when
// PathParams.Template is empty.
const defaultNamingTemplate = "{series}/Season {season}/{res}/{series} - S{season}E{episode}.{ext}"

// LibraryPath builds the absolute encoded-output path under the encoded root,
// following the Jellyfin/Plex convention:
//
//	<encoded_root>/<Series>/Season NN/<res>p/<Series> - SNNENN.<ext>
//
// Specials (episode_no NULL) map to Season 00 / S00E.. per Jellyfin's specials
// convention. The series title is filesystem-sanitized. naming_template tokens
// are honored when a template is supplied.
func LibraryPath(p PathParams) string {
	series := sanitizeSegment(p.Series)
	season := p.Season
	if p.IsSpecial {
		season = 0
	}
	res := strconv.Itoa(p.Resolution) + "p"
	ext := strings.TrimPrefix(p.Ext, ".")
	if ext == "" {
		ext = defaultContainer
	}

	tmpl := p.Template
	if strings.TrimSpace(tmpl) == "" {
		tmpl = defaultNamingTemplate
	}

	rel := expandTemplate(tmpl, map[string]string{
		"{series}":  series,
		"{season}":  pad2(season),
		"{episode}": padEpisode(p.Episode),
		"{res}":     res,
		"{ext}":     ext,
	})
	// Sanitize each path segment (the template may interpolate untrusted titles
	// into multiple segments) while preserving the separators.
	rel = sanitizeRelPath(rel)
	return filepath.Join(p.EncodedRoot, filepath.FromSlash(rel))
}

// expandTemplate replaces each token in the template with its value.
func expandTemplate(tmpl string, vals map[string]string) string {
	out := tmpl
	for token, val := range vals {
		out = strings.ReplaceAll(out, token, val)
	}
	return out
}

// sanitizeRelPath sanitizes every segment of a slash-separated relative path,
// leaving the separators intact so the directory structure is preserved.
func sanitizeRelPath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		parts[i] = sanitizeSegment(part)
	}
	return strings.Join(parts, "/")
}

// sanitizeSegment strips filesystem-illegal characters and trims trailing dots
// and spaces (which Windows rejects) from one path segment.
func sanitizeSegment(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(illegalPathChars, r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimRight(s, " .")
	s = strings.TrimSpace(s)
	// Windows reserved device names are checked on the base name (sans extension).
	base := s
	if dot := strings.IndexByte(s, '.'); dot > 0 {
		base = s[:dot]
	}
	if reservedNames[strings.ToUpper(base)] {
		s = "_" + s
	}
	return s
}

// pad2 zero-pads a non-negative int to at least two digits.
func pad2(n int) string {
	if n < 0 {
		n = 0
	}
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

// padEpisode zero-pads an episode number to at least two digits, allowing 3+
// digits for long-runners (e.g. E1090) without truncation.
func padEpisode(n int) string {
	return pad2(n)
}
