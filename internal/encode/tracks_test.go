package encode

import (
	"reflect"
	"testing"
)

func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"eng": "en", "en": "en", "English": "en", "  EN  ": "en",
		"jpn": "ja", "ja": "ja", "japanese": "ja",
		"und": unknownLang, "": unknownLang,
		"xx": "xx", // unlisted ISO code preserved (lowercased)
	}
	for in, want := range cases {
		if got := normalizeLang(in); got != want {
			t.Errorf("normalizeLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelectTracksMKVAllWildcard(t *testing.T) {
	res := Resolved{Container: "mkv"} // both language sets nil = wildcard
	streams := []Stream{
		{Index: 1, Type: "audio", Codec: "aac", Lang: "ja"},
		{Index: 2, Type: "subtitle", Codec: "ass", Lang: "en"},
	}
	sel := SelectTracks(res, streams)
	if sel.Explicit {
		t.Error("MKV all-wildcard must keep the blind -map 0 (Explicit=false)")
	}
	if !sel.SoftSubs || sel.BurnSub {
		t.Errorf("MKV wildcard: SoftSubs=%v BurnSub=%v, want true/false", sel.SoftSubs, sel.BurnSub)
	}
}

func TestSelectTracksMKVKeepSet(t *testing.T) {
	res := Resolved{Container: "mkv", AudioLanguages: []string{"ja"}, SubtitleLanguages: []string{"en"}}
	streams := []Stream{
		{Index: 1, Type: "audio", Codec: "aac", Lang: "ja"},
		{Index: 2, Type: "audio", Codec: "aac", Lang: "en"},
		{Index: 3, Type: "subtitle", Codec: "ass", Lang: "en"},
		{Index: 4, Type: "subtitle", Codec: "ass", Lang: "es"},
	}
	sel := SelectTracks(res, streams)
	if !sel.Explicit || !sel.SoftSubs {
		t.Fatalf("MKV keep-set must be explicit soft subs: %+v", sel)
	}
	if !reflect.DeepEqual(sel.AudioStreamIndices, []int{1}) {
		t.Errorf("audio indices = %v, want [1] (ja only)", sel.AudioStreamIndices)
	}
	if !reflect.DeepEqual(sel.SubtitleStreamIndices, []int{3}) {
		t.Errorf("sub indices = %v, want [3] (en only)", sel.SubtitleStreamIndices)
	}
}

func TestSelectTracksMP4DialogueOverSignsTiebreak(t *testing.T) {
	// Two English subs: Signs&Songs (first, not default) then Dialogue (default).
	// The default-flag tiebreak must pick Dialogue (si=1, index 4).
	res := Resolved{Container: "mp4", SubtitleLanguages: []string{"en"}}
	streams := []Stream{
		{Index: 1, Type: "video", Codec: "h264"},
		{Index: 2, Type: "audio", Codec: "aac", Lang: "ja", IsDefault: true},
		{Index: 3, Type: "subtitle", Codec: "ass", Lang: "en", IsDefault: false}, // Signs&Songs
		{Index: 4, Type: "subtitle", Codec: "ass", Lang: "en", IsDefault: true},  // Dialogue
	}
	sel := SelectTracks(res, streams)
	if !sel.BurnSub {
		t.Fatal("MP4 with subs must burn")
	}
	if sel.BurnSubFilterIndex != 1 {
		t.Errorf("si = %d, want 1 (Dialogue, default-flagged)", sel.BurnSubFilterIndex)
	}
	if sel.BurnSubAbsoluteIndex != 4 || sel.BurnSubLang != "en" {
		t.Errorf("chosen sub abs=%d lang=%q, want 4/en", sel.BurnSubAbsoluteIndex, sel.BurnSubLang)
	}
	if !reflect.DeepEqual(sel.AudioStreamIndices, []int{2}) {
		t.Errorf("audio = %v, want [2] (ja default)", sel.AudioStreamIndices)
	}
}

func TestSelectTracksMP4SubtitleFallbackRungs(t *testing.T) {
	video := Stream{Index: 0, Type: "video", Codec: "h264"}
	audio := Stream{Index: 1, Type: "audio", Codec: "aac", Lang: "ja"}

	// Rung 1: language match (es absent) → fall to default-flagged sub.
	res := Resolved{Container: "mp4", SubtitleLanguages: []string{"es"}}
	streams := []Stream{video, audio,
		{Index: 2, Type: "subtitle", Codec: "ass", Lang: "en", IsDefault: false},
		{Index: 3, Type: "subtitle", Codec: "ass", Lang: "fr", IsDefault: true},
	}
	sel := SelectTracks(res, streams)
	if !sel.BurnSub || sel.BurnSubAbsoluteIndex != 3 {
		t.Errorf("no-lang-match should fall to default sub (idx 3): %+v", sel)
	}

	// Rung 2: no language match, no default → first sub.
	streams2 := []Stream{video, audio,
		{Index: 2, Type: "subtitle", Codec: "ass", Lang: "en", IsDefault: false},
		{Index: 3, Type: "subtitle", Codec: "ass", Lang: "fr", IsDefault: false},
	}
	sel2 := SelectTracks(res, streams2)
	if !sel2.BurnSub || sel2.BurnSubAbsoluteIndex != 2 {
		t.Errorf("no default should fall to first sub (idx 2): %+v", sel2)
	}

	// Rung 3: no subs at all → no burn, clean encode.
	sel3 := SelectTracks(res, []Stream{video, audio})
	if sel3.BurnSub {
		t.Errorf("no subs must not burn: %+v", sel3)
	}
	if !reflect.DeepEqual(sel3.AudioStreamIndices, []int{1}) {
		t.Errorf("audio still selected without subs: %v", sel3.AudioStreamIndices)
	}
}

func TestSelectTracksMP4AudioFallback(t *testing.T) {
	// Audio: language match → default → first.
	video := Stream{Index: 0, Type: "video"}
	res := Resolved{Container: "mp4", AudioLanguages: []string{"en"}}

	// match
	sel := SelectTracks(res, []Stream{video,
		{Index: 1, Type: "audio", Lang: "ja", IsDefault: true},
		{Index: 2, Type: "audio", Lang: "en"},
	})
	if !reflect.DeepEqual(sel.AudioStreamIndices, []int{2}) {
		t.Errorf("audio lang match = %v, want [2]", sel.AudioStreamIndices)
	}

	// no match → default
	resJa := Resolved{Container: "mp4", AudioLanguages: []string{"de"}}
	selD := SelectTracks(resJa, []Stream{video,
		{Index: 1, Type: "audio", Lang: "ja"},
		{Index: 2, Type: "audio", Lang: "en", IsDefault: true},
	})
	if !reflect.DeepEqual(selD.AudioStreamIndices, []int{2}) {
		t.Errorf("audio default fallback = %v, want [2]", selD.AudioStreamIndices)
	}

	// no match, no default → first
	selF := SelectTracks(resJa, []Stream{video,
		{Index: 1, Type: "audio", Lang: "ja"},
		{Index: 2, Type: "audio", Lang: "en"},
	})
	if !reflect.DeepEqual(selF.AudioStreamIndices, []int{1}) {
		t.Errorf("audio first fallback = %v, want [1]", selF.AudioStreamIndices)
	}
}

func TestSelectTracksMP4WildcardAudioUsesDefault(t *testing.T) {
	res := Resolved{Container: "mp4"} // audio wildcard = Default track
	sel := SelectTracks(res, []Stream{
		{Index: 0, Type: "video"},
		{Index: 1, Type: "audio", Lang: "ja"},
		{Index: 2, Type: "audio", Lang: "en", IsDefault: true},
	})
	if !reflect.DeepEqual(sel.AudioStreamIndices, []int{2}) {
		t.Errorf("MP4 wildcard audio = %v, want [2] (default track)", sel.AudioStreamIndices)
	}
}

func TestSelectTracksMP4ForcesBurnForASSEvenWithoutBurnFlag(t *testing.T) {
	// burn_subs not set, but the source sub is ASS (not MP4-copyable) → forced burn.
	res := Resolved{Container: "mp4", BurnSubs: false}
	sel := SelectTracks(res, []Stream{
		{Index: 0, Type: "video"},
		{Index: 1, Type: "audio", Lang: "ja", IsDefault: true},
		{Index: 2, Type: "subtitle", Codec: "ass", Lang: "en", IsDefault: true},
	})
	if !sel.BurnSub {
		t.Error("MP4 + ASS source sub must force burn even without the burn flag")
	}
}

func TestSelectTracksMKVBurnDropsSoftSubs(t *testing.T) {
	res := Resolved{Container: "mkv", BurnSubs: true, SubtitleLanguages: []string{"en"}}
	sel := SelectTracks(res, []Stream{
		{Index: 0, Type: "video"},
		{Index: 1, Type: "audio", Lang: "ja", IsDefault: true},
		{Index: 2, Type: "subtitle", Codec: "ass", Lang: "en", IsDefault: true},
	})
	if !sel.BurnSub {
		t.Error("MKV with burn_subs must burn")
	}
	if len(sel.SubtitleStreamIndices) != 0 {
		t.Errorf("burn must not keep soft subs: %v", sel.SubtitleStreamIndices)
	}
}
