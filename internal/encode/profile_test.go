package encode

import (
	"reflect"
	"testing"
)

func TestResolveChainCoalescesChildOverParent(t *testing.T) {
	// Child (depth 0) overrides crf + smartblur; inherits the rest from the
	// builtin parent (depth 1).
	parent := chainRow{
		Codec: ptr("x265"), Crf: ptr(24.2), Preset: ptr("slow"),
		Smartblur: ptr[int64](1), Deinterlace: ptr[int64](0), Deblock: ptr("1,1"),
		PsyRd: ptr(1.0), PsyRdoq: ptr(1.0), AqStrength: ptr(1.0), AqMode: ptr[int64](3),
		Audio: ptr("copy"), Container: ptr("mkv"), OutputResolutions: ptr("[1080,720,480]"),
	}
	child := chainRow{
		Crf:       ptr(22.0),
		Smartblur: ptr[int64](0),
	}
	got := resolveChain([]chainRow{child, parent})

	if got.CRF != 22.0 {
		t.Errorf("CRF = %v, want 22 (child override)", got.CRF)
	}
	if got.SmartBlur {
		t.Errorf("SmartBlur = true, want false (child override)")
	}
	if got.Preset != "slow" {
		t.Errorf("Preset = %q, want slow (inherited)", got.Preset)
	}
	if got.Deblock != "1,1" || got.AQMode != 3 {
		t.Errorf("inherited knobs wrong: deblock=%q aqMode=%d", got.Deblock, got.AQMode)
	}
	if !reflect.DeepEqual(got.OutputResolutions, []int{1080, 720, 480}) {
		t.Errorf("OutputResolutions = %v, want [1080 720 480]", got.OutputResolutions)
	}
}

func TestResolveChainFillsDefaults(t *testing.T) {
	// An entirely empty single-node chain falls back to package defaults.
	got := resolveChain([]chainRow{{}})
	if got.CRF != defaultCRF || got.Preset != defaultPreset || got.AQMode != defaultAQMode {
		t.Errorf("defaults not applied: %+v", got)
	}
	if !reflect.DeepEqual(got.OutputResolutions, defaultOutputResolutions) {
		t.Errorf("default resolutions = %v, want %v", got.OutputResolutions, defaultOutputResolutions)
	}
}

func TestResolveChainDeepGrandparent(t *testing.T) {
	// child -> mid -> root; deblock only set at root, crf only at mid.
	root := chainRow{Deblock: ptr("2,2"), Preset: ptr("veryslow")}
	mid := chainRow{Crf: ptr(20.0)}
	child := chainRow{Smartblur: ptr[int64](1)}
	got := resolveChain([]chainRow{child, mid, root})
	if got.Deblock != "2,2" {
		t.Errorf("Deblock = %q, want 2,2 (grandparent)", got.Deblock)
	}
	if got.CRF != 20.0 {
		t.Errorf("CRF = %v, want 20 (parent)", got.CRF)
	}
	if !got.SmartBlur {
		t.Errorf("SmartBlur = false, want true (child)")
	}
	if got.Preset != "veryslow" {
		t.Errorf("Preset = %q, want veryslow (grandparent)", got.Preset)
	}
}

func TestResolveChainBitDepthDeband(t *testing.T) {
	// Child overrides both knobs over a parent that set the opposite values.
	parent := chainRow{BitDepth: ptr[int64](8), Deband: ptr[int64](0)}
	child := chainRow{BitDepth: ptr[int64](10), Deband: ptr[int64](1)}
	got := resolveChain([]chainRow{child, parent})
	if got.BitDepth != 10 {
		t.Errorf("BitDepth = %d, want 10 (child override)", got.BitDepth)
	}
	if !got.Deband {
		t.Errorf("Deband = false, want true (child override)")
	}

	// Empty chain falls back to package defaults (8-bit, deband off).
	empty := resolveChain([]chainRow{{}})
	if empty.BitDepth != defaultBitDepth || empty.BitDepth != 8 {
		t.Errorf("BitDepth = %d, want defaultBitDepth (8)", empty.BitDepth)
	}
	if empty.Deband != defaultDeband || empty.Deband {
		t.Errorf("Deband = %v, want defaultDeband (false)", empty.Deband)
	}
}

func TestParseResolutions(t *testing.T) {
	cases := []struct {
		in   *string
		want []int
	}{
		{nil, defaultOutputResolutions},
		{ptr(""), defaultOutputResolutions},
		{ptr("[720,480]"), []int{720, 480}},
		{ptr("not json"), defaultOutputResolutions},
		{ptr("[]"), defaultOutputResolutions},
	}
	for _, c := range cases {
		if got := parseResolutions(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseResolutions(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
