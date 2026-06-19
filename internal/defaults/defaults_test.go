package defaults

import (
	"reflect"
	"testing"
	"time"
)

// TestEmbedParses confirms the embedded JSON decodes (the package init already
// would have panicked otherwise, but this asserts Values is populated).
func TestEmbedParses(t *testing.T) {
	if Values.Server.DefaultPort == 0 {
		t.Fatal("Values not populated; embed/parse failed")
	}
}

// TestDisallowUnknownFields confirms the parser rejects an unknown JSON key, so a
// typo in defaults.json panics at startup instead of silently zero-valuing.
func TestDisallowUnknownFields(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on unknown field, got none")
		}
	}()
	mustParse([]byte(`{"server":{"default_port":1},"not_a_real_key":true}`))
}

// TestHistoricalValues locks every migrated value to the literal constant it
// replaced. A divergence here means the refactor changed behavior.
func TestHistoricalValues(t *testing.T) {
	// Server (internal/config.DefaultPort).
	if got := Values.Server.DefaultPort; got != 4773 {
		t.Errorf("server.default_port = %d, want 4773", got)
	}

	// Encode fallback knobs (internal/encode/profile.go).
	e := Values.Encode
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"default_codec", e.DefaultCodec, "x265"},
		{"default_crf", e.DefaultCRF, 24.2},
		{"default_preset", e.DefaultPreset, "slow"},
		{"default_deblock", e.DefaultDeblock, "1,1"},
		{"default_psy_rd", e.DefaultPsyRD, 1.0},
		{"default_psy_rdoq", e.DefaultPsyRDOQ, 1.0},
		{"default_aq_strength", e.DefaultAQStrength, 1.0},
		{"default_aq_mode", e.DefaultAQMode, 3},
		{"default_audio", e.DefaultAudio, "copy"},
		{"default_container", e.DefaultContainer, "mkv"},
		{"smartblur_chain", e.SmartBlurChain, "smartblur=1.5:-0.35:-3.5:0.65:0.25:2.0"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("encode.%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if !reflect.DeepEqual(e.DefaultOutputResolutions, []int{1080, 720, 480}) {
		t.Errorf("encode.default_output_resolutions = %v, want [1080 720 480]", e.DefaultOutputResolutions)
	}
	wantBase := []string{"me=2", "rd=4", "subme=7", "rdoq-level=2", "merange=57", "bframes=8", "b-adapt=2", "limit-sao=1", "frame-threads=3", "no-info=1"}
	if !reflect.DeepEqual(e.BaseX265Params, wantBase) {
		t.Errorf("encode.base_x265_params = %v, want %v", e.BaseX265Params, wantBase)
	}

	// Builtin seed profile (internal/store/seed.go). Note aq_mode=2 here while the
	// encode fallback above is aq_mode=3 — a deliberate, preserved divergence.
	if len(Values.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(Values.Profiles))
	}
	p := Values.Profiles[0]
	if p.Name != "Automin (x265)" || !p.Builtin {
		t.Errorf("profile name/builtin = %q/%v, want Automin (x265)/true", p.Name, p.Builtin)
	}
	if p.Codec != "x265" || p.CRF != 24.2 || p.Preset != "slow" {
		t.Errorf("profile codec/crf/preset = %q/%v/%q", p.Codec, p.CRF, p.Preset)
	}
	if !p.SmartBlur || p.Deinterlace {
		t.Errorf("profile smartblur/deinterlace = %v/%v, want true/false", p.SmartBlur, p.Deinterlace)
	}
	if p.Deblock != "1,1" || p.PsyRD != 1.0 || p.PsyRDOQ != 1.0 || p.AQStrength != 1.0 {
		t.Errorf("profile deblock/psy = %q/%v/%v/%v", p.Deblock, p.PsyRD, p.PsyRDOQ, p.AQStrength)
	}
	if p.AQMode != 2 {
		t.Errorf("profile aq_mode = %d, want 2 (preserved divergence from encode fallback 3)", p.AQMode)
	}
	if p.Audio != "copy" || p.Container != "mkv" || p.OutputResolutions != "[1080,720,480]" {
		t.Errorf("profile audio/container/res = %q/%q/%q", p.Audio, p.Container, p.OutputResolutions)
	}

	// Seed settings (internal/store/seed.go).
	s := Values.SeedSettings
	if s.NamingTemplate != "{series}/Season {season}/{res}/{series} - S{season}E{episode}.{ext}" {
		t.Errorf("seed naming_template = %q", s.NamingTemplate)
	}
	if s.DownloadClientName != "Embedded (anacrolix)" {
		t.Errorf("seed download_client_name = %q", s.DownloadClientName)
	}
	if s.ConcurrencyDownload != 3 || s.ConcurrencyEncode != 1 {
		t.Errorf("seed concurrency = %d/%d, want 3/1", s.ConcurrencyDownload, s.ConcurrencyEncode)
	}
	if s.CleanupPolicy != "delete" || !s.DohEnabled {
		t.Errorf("seed cleanup/doh = %q/%v, want delete/true", s.CleanupPolicy, s.DohEnabled)
	}
	if s.TrustedReleaseGroupsJSON() != `["SubsPlease","Erai-raws"]` {
		t.Errorf("seed trusted_release_groups JSON = %q", s.TrustedReleaseGroupsJSON())
	}

	// Source (internal/source).
	wantGroups := []string{"SubsPlease", "Erai-raws"}
	if !reflect.DeepEqual(Values.Source.TrustedReleaseGroups, wantGroups) {
		t.Errorf("source.trusted_release_groups = %v, want %v", Values.Source.TrustedReleaseGroups, wantGroups)
	}
	wantTrackers := []string{
		"udp://tracker.opentrackr.org:1337/announce",
		"udp://open.stealth.si:80/announce",
		"udp://exodus.desync.com:6969/announce",
	}
	if !reflect.DeepEqual(Values.Source.MagnetTrackers, wantTrackers) {
		t.Errorf("source.magnet_trackers = %v, want %v", Values.Source.MagnetTrackers, wantTrackers)
	}

	// Extensions (internal/extension/update.go).
	if got := Values.Extensions.AutoUpdateInterval(); got != 6*time.Hour {
		t.Errorf("extensions auto-update interval = %v, want 6h", got)
	}
	if got := Values.Extensions.AutoUpdateFirstDelay(); got != 60*time.Second {
		t.Errorf("extensions auto-update first delay = %v, want 60s", got)
	}

	// Poller (internal/poller).
	if got := Values.Poller.SchedulerTick(); got != 60*time.Second {
		t.Errorf("poller scheduler tick = %v, want 60s", got)
	}

	// AniList (internal/anilist).
	a := Values.AniList
	if got := a.RequestTimeout(); got != 15*time.Second {
		t.Errorf("anilist timeout = %v, want 15s", got)
	}
	if a.CacheCap != 512 || a.MaxRetries != 3 || a.BatchChunkSize != 50 {
		t.Errorf("anilist cap/retries/chunk = %d/%d/%d, want 512/3/50", a.CacheCap, a.MaxRetries, a.BatchChunkSize)
	}
	if a.MaxResponseBytes != 4<<20 {
		t.Errorf("anilist max_response_bytes = %d, want %d", a.MaxResponseBytes, 4<<20)
	}

	// Metadata (internal/metadata).
	m := Values.Metadata
	if got := m.RefreshInterval(); got != 3*time.Hour {
		t.Errorf("metadata interval = %v, want 3h", got)
	}
	if got := m.Staleness(); got != 24*time.Hour {
		t.Errorf("metadata staleness = %v, want 24h", got)
	}
	if got := m.FirstPassDelay(); got != 90*time.Second {
		t.Errorf("metadata first pass delay = %v, want 90s", got)
	}
	if m.RefreshLimit != 50 {
		t.Errorf("metadata refresh limit = %d, want 50", m.RefreshLimit)
	}
}
