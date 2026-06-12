package encode

import (
	"strings"
	"testing"
)

// p is a local pointer helper for building nullable chain rows in tests.
func ptr[T any](v T) *T { return &v }

func TestBuildArgsEmitsEveryKnob(t *testing.T) {
	res := Resolved{
		ProfileID:         1,
		Codec:             "x265",
		CRF:               24.2,
		Preset:            "slow",
		SmartBlur:         true,
		Deinterlace:       true,
		Deblock:           "1,1",
		PsyRD:             1.25,
		PsyRDOQ:           2.0,
		AQStrength:        0.9,
		AQMode:            3,
		Audio:             "copy",
		Container:         "mkv",
		X265Params:        "ctu=64:rc-lookahead=40",
		OutputResolutions: []int{1080, 720, 480},
	}
	args, snapshot, err := BuildArgs(res, 720, "/in/source.mkv", "/out/ep.mkv")
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	joined := strings.Join(args, " ")

	// Core codec + crf + preset.
	mustContainSeq(t, args, "-c:v", "libx265")
	mustContainSeq(t, args, "-crf", "24.2")
	mustContainSeq(t, args, "-preset", "slow")
	mustContainSeq(t, args, "-c:a", "copy")
	mustContain(t, args, "-y")
	mustContainSeq(t, args, "-progress", "pipe:1")
	mustContainSeq(t, args, "-f", "matroska")

	// x265-params must carry every tuned knob, the inheritable knobs, AND the
	// raw passthrough merged in — this is the regression guard for the old
	// CRF-only bug.
	x265 := argValue(t, args, "-x265-params")
	for _, want := range []string{
		"me=2", "rd=4", "subme=7", "rdoq-level=2", "merange=57", "bframes=8",
		"b-adapt=2", "limit-sao=1", "frame-threads=3", "no-info=1",
		"aq-mode=3", "aq-strength=0.9", "deblock=1,1", "psy-rd=1.25", "psy-rdoq=2",
		"ctu=64", "rc-lookahead=40", // passthrough merged
	} {
		if !strings.Contains(x265, want) {
			t.Errorf("x265-params missing %q\n got: %s", want, x265)
		}
	}

	// -vf chain order: yadif (deinterlace) -> smartblur -> scale=-2:720.
	vf := argValue(t, args, "-vf")
	wantVF := "yadif=1," + smartblurChain + ",scale=-2:720"
	if vf != wantVF {
		t.Errorf("-vf = %q, want %q", vf, wantVF)
	}

	// Snapshot is non-empty JSON capturing the resolution + x265 params.
	if !strings.Contains(snapshot, `"resolution":720`) || !strings.Contains(snapshot, "aq-mode=3") {
		t.Errorf("snapshot missing expected fields: %s", snapshot)
	}
	_ = joined
}

func TestBuildArgsNoOptionalFilters(t *testing.T) {
	res := Resolved{
		CRF: 23, Preset: "medium", Deblock: "0,0", PsyRD: 1, PsyRDOQ: 1,
		AQStrength: 1, AQMode: 2, Audio: "aac", Container: "mkv",
		OutputResolutions: []int{1080},
	}
	args, _, err := BuildArgs(res, 1080, "in.mkv", "out.mkv")
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	vf := argValue(t, args, "-vf")
	if vf != "scale=-2:1080" {
		t.Errorf("-vf = %q, want plain scale (no yadif/smartblur)", vf)
	}
	if got := argValue(t, args, "-c:a"); got != "aac" {
		t.Errorf("-c:a = %q, want aac (non-copy audio encoder)", got)
	}
}

func TestBuildArgsUnsupportedResolution(t *testing.T) {
	res := Resolved{OutputResolutions: []int{1080}}
	if _, _, err := BuildArgs(res, 999, "in", "out"); err == nil {
		t.Fatal("expected error for unsupported resolution")
	}
}

func TestX265ParamPassthroughOverridesDefault(t *testing.T) {
	res := Resolved{
		CRF: 24, Preset: "slow", Deblock: "1,1", PsyRD: 1, PsyRDOQ: 1,
		AQStrength: 1, AQMode: 2, Audio: "copy", Container: "mkv",
		X265Params: "aq-mode=4:bframes=4", // override two base/knob keys
	}
	x265 := buildX265Params(res)
	if !strings.Contains(x265, "aq-mode=4") || strings.Contains(x265, "aq-mode=2") {
		t.Errorf("passthrough should override aq-mode: %s", x265)
	}
	if !strings.Contains(x265, "bframes=4") || strings.Contains(x265, "bframes=8") {
		t.Errorf("passthrough should override bframes: %s", x265)
	}
}

func TestMuxerFor(t *testing.T) {
	cases := map[string]string{"mkv": "matroska", "MKV": "matroska", "mp4": "mp4", "": "matroska", "weird": "matroska"}
	for in, want := range cases {
		if got := muxerFor(in); got != want {
			t.Errorf("muxerFor(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- helpers ---

func mustContain(t *testing.T, args []string, want string) {
	t.Helper()
	for _, a := range args {
		if a == want {
			return
		}
	}
	t.Errorf("args missing %q: %v", want, args)
}

func mustContainSeq(t *testing.T, args []string, a, b string) {
	t.Helper()
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return
		}
	}
	t.Errorf("args missing sequence %q %q: %v", a, b, args)
}

func argValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	t.Fatalf("flag %q not found in %v", flag, args)
	return ""
}
