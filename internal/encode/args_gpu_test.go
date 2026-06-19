package encode

import (
	"strings"
	"testing"
)

func TestBuildArgsGPUEmitsEncoderQualityNoX265(t *testing.T) {
	res := Resolved{
		Codec: "gpu-auto", CRF: 24, Preset: "slow", Audio: "aac", Container: "mkv",
		OutputResolutions: []int{1080},
	}
	cases := map[string][]string{
		"hevc_nvenc":        {"-cq", "-rc", "vbr"},
		"hevc_qsv":          {"-global_quality", "-preset", "veryslow"},
		"hevc_amf":          {"-rc", "cqp", "-qp_i", "-qp_p"},
		"hevc_videotoolbox": {"-q:v"},
	}
	for enc, wantFlags := range cases {
		args, snapshot, err := BuildArgs(res, 1080, ColorTags{}, mkvSel(), enc, "/in", "/out")
		if err != nil {
			t.Fatalf("%s BuildArgs: %v", enc, err)
		}
		mustContainSeq(t, args, "-c:v", enc)
		// GPU presets are 8-bit yuv420p.
		mustContainSeq(t, args, "-pix_fmt", "yuv420p")
		for _, f := range wantFlags {
			if !contains(args, f) {
				t.Errorf("%s: args missing %q: %v", enc, f, args)
			}
		}
		// The whole x265 recipe must be absent.
		if contains(args, "-x265-params") {
			t.Errorf("%s: GPU path must not emit -x265-params: %v", enc, args)
		}
		if contains(args, "-crf") {
			t.Errorf("%s: GPU path must not emit -crf: %v", enc, args)
		}
		if !strings.Contains(snapshot, `"encoder":"`+enc+`"`) {
			t.Errorf("%s: snapshot missing encoder: %s", enc, snapshot)
		}
	}
}

func TestGPUQualityMapping(t *testing.T) {
	// CQP-scale encoders keep the CRF on a 0..51 scale (clamped).
	if got := identityCQ(24); got != 24 {
		t.Errorf("identityCQ(24) = %d, want 24", got)
	}
	if got := identityCQ(-5); got != 0 {
		t.Errorf("identityCQ clamps low: got %d, want 0", got)
	}
	if got := identityCQ(99); got != 51 {
		t.Errorf("identityCQ clamps high: got %d, want 51", got)
	}
	// VideoToolbox inverts onto 0..100 (higher = better): CRF 0 -> 100.
	if got := videoToolboxQuality(0); got != 100 {
		t.Errorf("videoToolboxQuality(0) = %d, want 100", got)
	}
	if got := videoToolboxQuality(51); got != 1 {
		t.Errorf("videoToolboxQuality(51) = %d, want 1", got)
	}
}

func TestBuildArgsBurnSubBeforeScaleWithSI(t *testing.T) {
	res := Resolved{
		CRF: 24, Preset: "slow", Deblock: "1,1", PsyRD: 1, PsyRDOQ: 1,
		AQStrength: 1, AQMode: 2, Audio: "aac", Container: "mp4",
		SmartBlur: true, OutputResolutions: []int{1080},
	}
	sel := TrackSelection{
		Explicit: true, BurnSub: true, BurnSubFilterIndex: 1,
		BurnSubAbsoluteIndex: 4, BurnSubLang: "en",
		AudioStreamIndices: []int{2}, AudioLang: "ja",
	}
	args, snapshot, err := BuildArgs(res, 1080, ColorTags{}, sel, cpuEncoder, "/in/source.mkv", "/out/ep.mp4")
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	vf := argValue(t, args, "-vf")
	subIdx := strings.Index(vf, "subtitles=")
	scaleIdx := strings.Index(vf, "scale=")
	smartIdx := strings.Index(vf, "smartblur")
	if subIdx < 0 {
		t.Fatalf("-vf missing subtitles filter: %q", vf)
	}
	if subIdx > scaleIdx {
		t.Errorf("burn must come before scale: %q", vf)
	}
	if smartIdx >= 0 && subIdx > smartIdx {
		t.Errorf("burn must come before smartblur: %q", vf)
	}
	if !strings.Contains(vf, "si=1") {
		t.Errorf("-vf missing si=1 (filter index among subs): %q", vf)
	}
	// Burn drops soft sub/attachment copy.
	if contains(args, "-c:s") {
		t.Errorf("burn must drop -c:s copy: %v", args)
	}
	// Explicit map: video + chosen audio only (no blind -map 0).
	mustContainSeq(t, args, "-map", "0:V:0")
	mustContainSeq(t, args, "-map", "0:2")
	if contains2(args, "-map", "0") {
		t.Errorf("burn must not emit blind -map 0: %v", args)
	}
	for _, want := range []string{`"burn_subs":true`, `"subtitle_lang":"en"`, `"subtitle_stream_index":4`, `"audio_lang":"ja"`} {
		if !strings.Contains(snapshot, want) {
			t.Errorf("snapshot missing %q: %s", want, snapshot)
		}
	}
}

func TestEscapeSubtitlesPathWindows(t *testing.T) {
	cases := map[string]string{
		`C:\anime\ep.mkv`:    `C\:\\anime\\ep.mkv`,
		`C:\a'b\ep.mkv`:      `C\:\\a\'b\\ep.mkv`,
		`/home/user/ep.mkv`:  `/home/user/ep.mkv`,
	}
	for in, want := range cases {
		if got := escapeSubtitlesPath(in); got != want {
			t.Errorf("escapeSubtitlesPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildArgsMKVExplicitMaps(t *testing.T) {
	res := Resolved{
		CRF: 24, Preset: "slow", Deblock: "1,1", PsyRD: 1, PsyRDOQ: 1,
		AQStrength: 1, AQMode: 2, Audio: "copy", Container: "mkv",
		OutputResolutions: []int{1080},
	}
	sel := TrackSelection{
		Explicit: true, SoftSubs: true,
		AudioStreamIndices: []int{1}, SubtitleStreamIndices: []int{3},
	}
	args, _, err := BuildArgs(res, 1080, ColorTags{}, sel, cpuEncoder, "/in", "/out")
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	mustContainSeq(t, args, "-map", "0:V:0")
	mustContainSeq(t, args, "-map", "0:1")
	mustContainSeq(t, args, "-map", "0:3")
	mustContainSeq(t, args, "-c:s", "copy")
}

// --- helpers ---

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// contains2 reports whether the exact sequence a,b appears in args.
func contains2(args []string, a, b string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return true
		}
	}
	return false
}
