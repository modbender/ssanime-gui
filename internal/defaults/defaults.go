// Package defaults is the single source of truth for the app's shipped, tunable
// default values: encode-recipe knobs, the seeded encode profile(s) and settings,
// the source allowlist and tracker list, and the various background-loop intervals
// and caps. They are embedded into the binary as defaults.json (NOT an external
// runtime file) and parsed once at init.
//
// This package imports nothing from the rest of the app (stdlib + embed only), so
// every package can depend on it without risking an import cycle.
package defaults

import (
	_ "embed"
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed defaults.json
var raw []byte

// Config mirrors defaults.json exactly. DisallowUnknownFields is in effect during
// parse, so a key here that is missing/misspelled in the JSON (or vice versa)
// panics at startup instead of silently zero-valuing a field.
type Config struct {
	Server       Server    `json:"server"`
	Encode       Encode    `json:"encode"`
	Profiles     []Profile `json:"profiles"`
	SeedSettings Settings  `json:"seed_settings"`
	Source       Source    `json:"source"`
	Extensions   Extensions `json:"extensions"`
	Poller       Poller    `json:"poller"`
	AniList      AniList   `json:"anilist"`
	Metadata     Metadata  `json:"metadata"`
}

// Server holds daemon-level boot defaults.
type Server struct {
	DefaultPort int `json:"default_port"`
}

// Encode holds the fallback encode-knob defaults (used when neither a profile nor
// its parents specify a knob) plus the fixed tuned recipe pieces.
type Encode struct {
	DefaultCodec             string   `json:"default_codec"`
	DefaultCRF               float64  `json:"default_crf"`
	DefaultPreset            string   `json:"default_preset"`
	DefaultDeblock           string   `json:"default_deblock"`
	DefaultPsyRD             float64  `json:"default_psy_rd"`
	DefaultPsyRDOQ           float64  `json:"default_psy_rdoq"`
	DefaultAQStrength        float64  `json:"default_aq_strength"`
	DefaultAQMode            int      `json:"default_aq_mode"`
	DefaultAudio             string   `json:"default_audio"`
	DefaultContainer         string   `json:"default_container"`
	DefaultBitDepth          int      `json:"default_bit_depth"`
	DefaultDeband            bool     `json:"default_deband"`
	DefaultOutputResolutions []int    `json:"default_output_resolutions"`
	SmartBlurChain           string   `json:"smartblur_chain"`
	BaseX265Params           []string `json:"base_x265_params"`
}

// Profile is one shipped, builtin encode profile. Modeled as an array entry so
// adding a profile later is a single JSON entry, not a code change.
type Profile struct {
	Name              string  `json:"name"`
	Builtin           bool    `json:"builtin"`
	Codec             string  `json:"codec"`
	CRF               float64 `json:"crf"`
	Preset            string  `json:"preset"`
	SmartBlur         bool    `json:"smartblur"`
	Deinterlace       bool    `json:"deinterlace"`
	Deblock           string  `json:"deblock"`
	PsyRD             float64 `json:"psy_rd"`
	PsyRDOQ           float64 `json:"psy_rdoq"`
	AQStrength        float64 `json:"aq_strength"`
	AQMode            int64   `json:"aq_mode"`
	Audio             string  `json:"audio"`
	Container         string  `json:"container"`
	BitDepth          int64   `json:"bit_depth"`
	Deband            bool    `json:"deband"`
	OutputResolutions string  `json:"output_resolutions"`
}

// Settings holds the singleton settings row defaults a fresh install boots with.
type Settings struct {
	NamingTemplate       string   `json:"naming_template"`
	DownloadClientName   string   `json:"download_client_name"`
	ConcurrencyDownload  int64    `json:"concurrency_download"`
	ConcurrencyEncode    int64    `json:"concurrency_encode"`
	CleanupPolicy        string   `json:"cleanup_policy"`
	DohEnabled           bool     `json:"doh_enabled"`
	TrustedReleaseGroups []string `json:"trusted_release_groups"`
}

// TrustedReleaseGroupsJSON renders the seed trusted-group allowlist as the compact
// JSON string the settings row stores (e.g. `["SubsPlease","Erai-raws"]`). It
// mirrors the column default in goose migration 00013; keep them consistent.
func (s Settings) TrustedReleaseGroupsJSON() string {
	b, err := json.Marshal(s.TrustedReleaseGroups)
	if err != nil {
		panic(fmt.Sprintf("defaults: marshal trusted release groups: %v", err))
	}
	return string(b)
}

// Source holds sourcing-layer defaults: the original-release allowlist and the
// trackers appended when building a magnet from a bare info hash.
type Source struct {
	TrustedReleaseGroups []string `json:"trusted_release_groups"`
	MagnetTrackers       []string `json:"magnet_trackers"`
}

// Extensions holds the background auto-updater cadence.
type Extensions struct {
	AutoUpdateIntervalHours      float64 `json:"auto_update_interval_hours"`
	AutoUpdateFirstDelaySeconds  float64 `json:"auto_update_first_delay_seconds"`
}

// Poller holds the feed-poller scheduler tick. The default per-feed interval is
// not here: it has no Go consumer and lives only as the SQL column default in the
// goose migrations (which defaults.json deliberately does not drive).
type Poller struct {
	SchedulerTickSeconds float64 `json:"scheduler_tick_seconds"`
}

// AniList holds the AniList client caps and backoff knobs.
type AniList struct {
	RequestTimeoutSeconds float64 `json:"request_timeout_seconds"`
	CacheCap              int     `json:"cache_cap"`
	MaxRetries            int     `json:"max_retries"`
	MaxResponseBytes      int64   `json:"max_response_bytes"`
	BatchChunkSize        int     `json:"batch_chunk_size"`
}

// Metadata holds the background metadata refresher cadence and caps.
type Metadata struct {
	RefreshIntervalHours float64 `json:"refresh_interval_hours"`
	StalenessHours       float64 `json:"staleness_hours"`
	RefreshLimit         int64   `json:"refresh_limit"`
	FirstPassDelaySeconds float64 `json:"first_pass_delay_seconds"`
}

// Values is the singleton parsed from the embedded defaults.json. It is fully
// populated before any consumer's init or first use because this package's init
// runs before any importer's.
var Values = mustParse(raw)

// mustParse decodes the embedded JSON with unknown-field rejection. The data is
// compile-embedded, so any failure here is a build/programming error, not a
// runtime condition — panic so it surfaces at startup rather than zero-valuing.
func mustParse(b []byte) Config {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		panic(fmt.Sprintf("defaults: parse defaults.json: %v", err))
	}
	return c
}

// AutoUpdateInterval is the extension auto-updater re-check cadence.
func (e Extensions) AutoUpdateInterval() time.Duration {
	return hours(e.AutoUpdateIntervalHours)
}

// AutoUpdateFirstDelay is how long the first auto-update pass is held off after boot.
func (e Extensions) AutoUpdateFirstDelay() time.Duration {
	return seconds(e.AutoUpdateFirstDelaySeconds)
}

// SchedulerTick is how often the poller wakes to look for due feeds.
func (p Poller) SchedulerTick() time.Duration {
	return seconds(p.SchedulerTickSeconds)
}

// RequestTimeout is the AniList HTTP client timeout.
func (a AniList) RequestTimeout() time.Duration {
	return seconds(a.RequestTimeoutSeconds)
}

// RefreshInterval is how often the metadata refresher wakes.
func (m Metadata) RefreshInterval() time.Duration {
	return hours(m.RefreshIntervalHours)
}

// Staleness is how old metadata must be before it is eligible for refresh.
func (m Metadata) Staleness() time.Duration {
	return hours(m.StalenessHours)
}

// FirstPassDelay is how long the first metadata refresh pass is held off after boot.
func (m Metadata) FirstPassDelay() time.Duration {
	return seconds(m.FirstPassDelaySeconds)
}

func hours(h float64) time.Duration {
	return time.Duration(h * float64(time.Hour))
}

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}
