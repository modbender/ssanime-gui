package encode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// unknownLang is the normalized code for a track with no usable language tag
// (missing, "und", or unrecognized).
const unknownLang = "unknown"

// langAliases maps the common ffprobe language tokens to a normalized code.
// ffprobe tags are inconsistent across releases (ISO 639-1, 639-2/B, 639-2/T,
// or the English name), so the alias set is data-driven — adding a language is
// one entry, not a branch. Tokens not in the map normalize to themselves
// (lowercased) so an unlisted ISO code still round-trips rather than vanishing.
var langAliases = map[string]string{
	"en": "en", "eng": "en", "english": "en",
	"ja": "ja", "jpn": "ja", "jp": "ja", "japanese": "ja",
	"es": "es", "spa": "es", "esp": "es", "spanish": "es",
	"pt": "pt", "por": "pt", "portuguese": "pt",
	"fr": "fr", "fra": "fr", "fre": "fr", "french": "fr",
	"de": "de", "ger": "de", "deu": "de", "german": "de",
	"it": "it", "ita": "it", "italian": "it",
	"ru": "ru", "rus": "ru", "russian": "ru",
	"zh": "zh", "chi": "zh", "zho": "zh", "chinese": "zh",
	"ko": "ko", "kor": "ko", "korean": "ko",
	"ar": "ar", "ara": "ar", "arabic": "ar",
}

// normalizeLang maps a raw ffprobe language token to a normalized code. Empty
// or "und" → unknown; a known alias → its canonical code; anything else → the
// lowercased token (so an uncommon-but-valid ISO code is preserved).
func normalizeLang(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || s == "und" {
		return unknownLang
	}
	if c, ok := langAliases[s]; ok {
		return c
	}
	return s
}

// mp4CopyableSubCodecs are the only subtitle codecs an MP4 container can carry
// as soft subs (the timed-text codec). Anything else — ASS/SSA, SubRip, image
// subs (PGS/dvdsub) — cannot be copied into MP4, which forces burn-in.
var mp4CopyableSubCodecs = map[string]bool{
	"mov_text": true,
}

// Stream is the minimal per-track shape the selector needs, decoded from
// ffprobe. Index is the absolute stream index; Lang is already normalized.
type Stream struct {
	Index     int
	Type      string // "video" | "audio" | "subtitle"
	Codec     string
	Lang      string
	IsDefault bool
}

// TrackSelection is the resolved per-track mapping the arg builder consumes. It
// is fully concrete — the arg builder never reasons about language sets again.
type TrackSelection struct {
	// AudioStreamIndices are the absolute stream indices of kept audio tracks,
	// in source order (MKV soft mode keeps several; MP4 keeps at most one).
	AudioStreamIndices []int
	// SubtitleStreamIndices are the absolute indices of kept soft-sub tracks
	// (MKV soft mode only; empty when burning or MP4).
	SubtitleStreamIndices []int
	// SoftSubs is true when subtitle tracks are copied through (MKV, no burn).
	SoftSubs bool

	// BurnSub is true when one subtitle is rendered into the video.
	BurnSub bool
	// BurnSubFilterIndex is the index *among subtitle streams* (the `si` value
	// for the subtitles filter), valid only when BurnSub is true.
	BurnSubFilterIndex int
	// BurnSubAbsoluteIndex is the chosen sub's absolute stream index (snapshot).
	BurnSubAbsoluteIndex int
	// BurnSubLang is the chosen sub's normalized language (snapshot).
	BurnSubLang string

	// AudioLang is the chosen primary audio language (snapshot; MP4 single pick).
	AudioLang string
	// Explicit is true when explicit -map flags should be emitted (anything but
	// the MKV all-passthrough case, which keeps the legacy blind -map 0).
	Explicit bool
}

// ProbeStreams reads the source's audio/subtitle stream table via ffprobe and
// returns normalized Stream rows (video excluded — it is always mapped). It
// reuses the ffprobe binary discovered for color/duration probing.
func (t Tools) ProbeStreams(ctx context.Context, input string) ([]Stream, error) {
	cmd := execCommand(ctx, t.FFprobe,
		"-v", "error",
		"-show_entries", "stream=index,codec_type,codec_name,disposition:stream_tags=language",
		"-of", "json",
		input,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe streams: %w", err)
	}
	var parsed struct {
		Streams []struct {
			Index       int    `json:"index"`
			CodecType   string `json:"codec_type"`
			CodecName   string `json:"codec_name"`
			Disposition struct {
				Default int `json:"default"`
			} `json:"disposition"`
			Tags struct {
				Language string `json:"language"`
			} `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parse ffprobe streams: %w", err)
	}
	var streams []Stream
	for _, s := range parsed.Streams {
		if s.CodecType != "audio" && s.CodecType != "subtitle" {
			continue
		}
		streams = append(streams, Stream{
			Index:     s.Index,
			Type:      s.CodecType,
			Codec:     strings.ToLower(s.CodecName),
			Lang:      normalizeLang(s.Tags.Language),
			IsDefault: s.Disposition.Default == 1,
		})
	}
	return streams, nil
}

// SelectTracks resolves the source streams against a profile into a concrete
// TrackSelection. mp4 drives single-pick + burn semantics; mkv drives soft
// copy-all/keep-set. burnSubs forces burn-in regardless of container, and MP4
// with a non-text source sub forces it too (ASS can't be copied into MP4).
func SelectTracks(res Resolved, streams []Stream) TrackSelection {
	isMP4 := strings.EqualFold(strings.TrimSpace(res.Container), "mp4")

	var audio, subs []Stream
	for _, s := range streams {
		switch s.Type {
		case "audio":
			audio = append(audio, s)
		case "subtitle":
			subs = append(subs, s)
		}
	}

	if isMP4 {
		return selectMP4(res, audio, subs)
	}
	return selectMKV(res, audio, subs, streams)
}

// selectMKV keeps every audio/sub track matching the language set (or all when
// wildcard). The wildcard-all case keeps the blind -map 0 (Explicit=false) so
// chapters/attachments/data also carry through unchanged.
func selectMKV(res Resolved, audio, subs []Stream, all []Stream) TrackSelection {
	sel := TrackSelection{SoftSubs: true}

	// Burn requested even on MKV: render one sub, drop soft subs, map explicitly.
	if res.BurnSubs {
		return selectBurn(res, audio, subs)
	}

	audioWild := res.AudioLanguages == nil
	subWild := res.SubtitleLanguages == nil
	if audioWild && subWild {
		// All-passthrough: keep legacy -map 0 (no explicit maps).
		return sel
	}

	sel.Explicit = true
	for _, s := range audio {
		if audioWild || langInSet(s.Lang, res.AudioLanguages) {
			sel.AudioStreamIndices = append(sel.AudioStreamIndices, s.Index)
		}
	}
	for _, s := range subs {
		if subWild || langInSet(s.Lang, res.SubtitleLanguages) {
			sel.SubtitleStreamIndices = append(sel.SubtitleStreamIndices, s.Index)
		}
	}
	_ = all
	return sel
}

// selectMP4 picks one audio and (when burning) one subtitle. MP4 always burns
// when a sub exists: a text sub if burn requested, and forced for ASS/image
// subs that can't be copied into MP4.
func selectMP4(res Resolved, audio, subs []Stream) TrackSelection {
	// MP4 burns whenever a subtitle exists — either the profile asked for it, or
	// the source sub is non-text (copying ASS/PGS into MP4 fails).
	wantBurn := res.BurnSubs || hasNonMP4CopyableSub(subs)
	if wantBurn {
		return selectBurn(res, audio, subs)
	}
	// No burn, no subs to copy (MP4 soft-sub copy is not supported here): map
	// video + one audio explicitly.
	sel := TrackSelection{Explicit: true}
	if a, ok := pickAudio(res, audio); ok {
		sel.AudioStreamIndices = []int{a.Index}
		sel.AudioLang = a.Lang
	}
	return sel
}

// selectBurn picks one subtitle (with the default-flag tiebreak) and one audio,
// mapping video + that audio only. The burn filter index is the position of the
// chosen sub among subtitle streams (the `si` value).
func selectBurn(res Resolved, audio, subs []Stream) TrackSelection {
	sel := TrackSelection{Explicit: true}
	if a, ok := pickAudio(res, audio); ok {
		sel.AudioStreamIndices = []int{a.Index}
		sel.AudioLang = a.Lang
	}
	if si, ok := pickSubtitle(res, subs); ok {
		sel.BurnSub = true
		sel.BurnSubFilterIndex = si
		sel.BurnSubAbsoluteIndex = subs[si].Index
		sel.BurnSubLang = subs[si].Lang
	}
	return sel
}

// pickAudio chooses the single MP4/burn audio track: first language match (list
// order) → default-flagged → first track.
func pickAudio(res Resolved, audio []Stream) (Stream, bool) {
	if len(audio) == 0 {
		return Stream{}, false
	}
	if res.AudioLanguages != nil {
		if s, ok := firstByLang(audio, res.AudioLanguages); ok {
			return s, true
		}
	}
	if s, ok := firstDefault(audio); ok {
		return s, true
	}
	return audio[0], true
}

// pickSubtitle chooses the single sub to burn, returning its index among the
// subtitle streams (the filter `si`). First language match in list order, with
// a default-flag tiebreak when a language has several tracks (picks the full
// Dialogue track over Signs&Songs). Fallbacks: default-flagged → first → none.
func pickSubtitle(res Resolved, subs []Stream) (int, bool) {
	if len(subs) == 0 {
		return 0, false
	}
	if res.SubtitleLanguages != nil {
		// Walk the language priority list; within a matched language prefer the
		// default-flagged track (Dialogue), else the first of that language.
		for _, want := range res.SubtitleLanguages {
			matchIdx, defaultIdx := -1, -1
			for i, s := range subs {
				if s.Lang != normalizeLang(want) {
					continue
				}
				if matchIdx == -1 {
					matchIdx = i
				}
				if s.IsDefault && defaultIdx == -1 {
					defaultIdx = i
				}
			}
			if defaultIdx >= 0 {
				return defaultIdx, true
			}
			if matchIdx >= 0 {
				return matchIdx, true
			}
		}
	}
	for i, s := range subs {
		if s.IsDefault {
			return i, true
		}
	}
	return 0, true
}

// firstByLang returns the first stream whose language is in the priority list,
// scanning languages in list order (the list is a preference ranking).
func firstByLang(streams []Stream, langs []string) (Stream, bool) {
	for _, want := range langs {
		w := normalizeLang(want)
		for _, s := range streams {
			if s.Lang == w {
				return s, true
			}
		}
	}
	return Stream{}, false
}

// firstDefault returns the first default-flagged stream.
func firstDefault(streams []Stream) (Stream, bool) {
	for _, s := range streams {
		if s.IsDefault {
			return s, true
		}
	}
	return Stream{}, false
}

// langInSet reports whether a normalized language is in the selection set.
func langInSet(lang string, set []string) bool {
	for _, s := range set {
		if normalizeLang(s) == lang {
			return true
		}
	}
	return false
}

// hasNonMP4CopyableSub reports whether any subtitle stream is a codec MP4 can't
// soft-copy (ASS/SubRip/image), which forces burn-in for an MP4 profile.
func hasNonMP4CopyableSub(subs []Stream) bool {
	for _, s := range subs {
		if !mp4CopyableSubCodecs[s.Codec] {
			return true
		}
	}
	return false
}
