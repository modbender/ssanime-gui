// Package encode is the encode + library stage of the pipeline: it resolves an
// encode profile's inheritance chain into an effective config, builds the full
// libx265 ffmpeg command (every tuned knob, not just CRF), runs ffmpeg with real
// ffprobe-anchored progress, fans one downloaded episode out into one encoded
// output per target resolution, generates library thumbnails, moves each output
// into its Jellyfin/Plex library path, and cleans up the original once every
// output is archived.
package encode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modbender/ssanime-gui/internal/defaults"
	"github.com/modbender/ssanime-gui/internal/store"
)

// Default knob values used when neither a profile nor its parents specify one.
// They mirror automin's tuned x265 recipe (the proven base) so an empty profile
// still produces a sane encode.
var (
	defaultCodec      = defaults.Values.Encode.DefaultCodec
	defaultCRF        = defaults.Values.Encode.DefaultCRF
	defaultPreset     = defaults.Values.Encode.DefaultPreset
	defaultDeblock    = defaults.Values.Encode.DefaultDeblock
	defaultPsyRD      = defaults.Values.Encode.DefaultPsyRD
	defaultPsyRDOQ    = defaults.Values.Encode.DefaultPsyRDOQ
	defaultAQStrength = defaults.Values.Encode.DefaultAQStrength
	defaultAQMode     = defaults.Values.Encode.DefaultAQMode
	defaultAudio      = defaults.Values.Encode.DefaultAudio
	defaultContainer  = defaults.Values.Encode.DefaultContainer
	defaultBitDepth   = defaults.Values.Encode.DefaultBitDepth
	defaultDeband     = defaults.Values.Encode.DefaultDeband
)

// defaultOutputResolutions is used when no profile in the chain declares an
// output_resolutions set.
var defaultOutputResolutions = defaults.Values.Encode.DefaultOutputResolutions

// Resolved is the effective, fully-specified encode config produced by walking a
// profile's parent_id chain and COALESCE-ing each nullable knob child->parent,
// then filling any still-missing knob from the package defaults. Every field is
// concrete so the arg builder never has to reason about inheritance again.
type Resolved struct {
	ProfileID         int64
	Codec             string
	CRF               float64
	Preset            string
	SmartBlur         bool
	Deinterlace       bool
	Deblock           string
	PsyRD             float64
	PsyRDOQ           float64
	AQStrength        float64
	AQMode            int
	Audio             string
	Container         string
	X265Params        string // raw passthrough merged into -x265-params
	BitDepth          int    // 8 or 10; 10 emits yuv420p10le to curb banding
	Deband            bool
	BurnSubs          bool
	// AudioLanguages / SubtitleLanguages are the per-track language selection.
	// nil = the wildcard/passthrough sentinel (MKV: All; MP4: Default track). A
	// non-nil (possibly empty) slice is Specific mode: normalized language codes
	// in priority order.
	AudioLanguages    []string
	SubtitleLanguages []string
	OutputResolutions []int
}

// chainRow is the minimal field set the resolver needs from one profile in the
// inheritance chain. Both store.ResolveProfileChainRow and store.EncodeProfile
// satisfy it structurally via the adapters below, so the resolver is testable
// without a DB.
type chainRow struct {
	Codec             *string
	Crf               *float64
	Preset            *string
	Smartblur         *int64
	Deinterlace       *int64
	Deblock           *string
	PsyRd             *float64
	PsyRdoq           *float64
	AqStrength        *float64
	AqMode            *int64
	Audio             *string
	Container         *string
	X265Params        *string
	BitDepth          *int64
	Deband            *int64
	BurnSubs          *int64
	AudioLanguages    *string
	SubtitleLanguages *string
	OutputResolutions *string
}

func rowFromChain(r store.ResolveProfileChainRow) chainRow {
	return chainRow{
		Codec: r.Codec, Crf: r.Crf, Preset: r.Preset, Smartblur: r.Smartblur,
		Deinterlace: r.Deinterlace, Deblock: r.Deblock, PsyRd: r.PsyRd,
		PsyRdoq: r.PsyRdoq, AqStrength: r.AqStrength, AqMode: r.AqMode,
		Audio: r.Audio, Container: r.Container, X265Params: r.X265Params,
		BitDepth: r.BitDepth, Deband: r.Deband, BurnSubs: r.BurnSubs,
		AudioLanguages: r.AudioLanguages, SubtitleLanguages: r.SubtitleLanguages,
		OutputResolutions: r.OutputResolutions,
	}
}

// ProfileResolver loads and resolves encode profiles from the store. It is the
// only place inheritance is interpreted; everything downstream works on Resolved.
type ProfileResolver struct {
	store interface {
		Read() *store.Queries
	}
}

// NewProfileResolver builds a resolver over the given store.
func NewProfileResolver(st interface{ Read() *store.Queries }) *ProfileResolver {
	return &ProfileResolver{store: st}
}

// Resolve walks the profile chain for profileID and returns the effective config.
// The chain rows arrive child-first (depth ASC); the first non-NULL value for
// each knob wins, then defaults fill the rest.
func (r *ProfileResolver) Resolve(ctx context.Context, profileID int64) (Resolved, error) {
	rows, err := r.store.Read().ResolveProfileChain(ctx, profileID)
	if err != nil {
		return Resolved{}, fmt.Errorf("resolve profile chain %d: %w", profileID, err)
	}
	if len(rows) == 0 {
		return Resolved{}, fmt.Errorf("profile %d not found", profileID)
	}
	chain := make([]chainRow, len(rows))
	for i, row := range rows {
		chain[i] = rowFromChain(row)
	}
	res := resolveChain(chain)
	res.ProfileID = profileID
	return res, nil
}

// resolveChain is the pure COALESCE-child->parent-then-default resolver. Exposed
// for unit tests with hand-built chains (no DB).
func resolveChain(chain []chainRow) Resolved {
	res := Resolved{
		Codec:      defaultCodec,
		CRF:        defaultCRF,
		Preset:     defaultPreset,
		Deblock:    defaultDeblock,
		PsyRD:      defaultPsyRD,
		PsyRDOQ:    defaultPsyRDOQ,
		AQStrength: defaultAQStrength,
		AQMode:     defaultAQMode,
		Audio:      defaultAudio,
		Container:  defaultContainer,
		BitDepth:   defaultBitDepth,
		Deband:     defaultDeband,
	}

	var (
		codec, preset, deblock, audio, container, x265, outRes *string
		audioLangs, subLangs                                   *string
		crf, psyRD, psyRDOQ, aqStrength                        *float64
		aqMode                                                 *int64
		smartblur, deinterlace, bitDepth, deband, burnSubs     *int64
	)
	// First non-NULL (child-first order) wins for each knob.
	pickStr := func(dst **string, v *string) {
		if *dst == nil && v != nil {
			*dst = v
		}
	}
	pickF := func(dst **float64, v *float64) {
		if *dst == nil && v != nil {
			*dst = v
		}
	}
	pickI := func(dst **int64, v *int64) {
		if *dst == nil && v != nil {
			*dst = v
		}
	}
	for _, row := range chain {
		pickStr(&codec, row.Codec)
		pickStr(&preset, row.Preset)
		pickStr(&deblock, row.Deblock)
		pickStr(&audio, row.Audio)
		pickStr(&container, row.Container)
		pickStr(&x265, row.X265Params)
		pickStr(&outRes, row.OutputResolutions)
		pickStr(&audioLangs, row.AudioLanguages)
		pickStr(&subLangs, row.SubtitleLanguages)
		pickF(&crf, row.Crf)
		pickF(&psyRD, row.PsyRd)
		pickF(&psyRDOQ, row.PsyRdoq)
		pickF(&aqStrength, row.AqStrength)
		pickI(&aqMode, row.AqMode)
		pickI(&smartblur, row.Smartblur)
		pickI(&deinterlace, row.Deinterlace)
		pickI(&bitDepth, row.BitDepth)
		pickI(&deband, row.Deband)
		pickI(&burnSubs, row.BurnSubs)
	}

	if codec != nil {
		res.Codec = *codec
	}
	if crf != nil {
		res.CRF = *crf
	}
	if preset != nil {
		res.Preset = *preset
	}
	if deblock != nil {
		res.Deblock = *deblock
	}
	if psyRD != nil {
		res.PsyRD = *psyRD
	}
	if psyRDOQ != nil {
		res.PsyRDOQ = *psyRDOQ
	}
	if aqStrength != nil {
		res.AQStrength = *aqStrength
	}
	if aqMode != nil {
		res.AQMode = int(*aqMode)
	}
	if audio != nil {
		res.Audio = *audio
	}
	if container != nil {
		res.Container = *container
	}
	if x265 != nil {
		res.X265Params = *x265
	}
	res.SmartBlur = smartblur != nil && *smartblur == 1
	res.Deinterlace = deinterlace != nil && *deinterlace == 1
	if bitDepth != nil {
		res.BitDepth = int(*bitDepth)
	}
	res.Deband = deband != nil && *deband == 1
	res.BurnSubs = burnSubs != nil && *burnSubs == 1
	// nil pointer (whole chain NULL) stays the wildcard sentinel (nil slice).
	res.AudioLanguages = parseLanguages(audioLangs)
	res.SubtitleLanguages = parseLanguages(subLangs)
	res.OutputResolutions = parseResolutions(outRes)

	return res
}

// parseLanguages decodes the JSON language array. A nil/blank pointer is the
// wildcard sentinel (nil slice). An explicit array — even empty — is Specific
// mode and returns a non-nil slice so callers distinguish "[]" from wildcard.
func parseLanguages(raw *string) []string {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(*raw), &out); err != nil {
		return nil
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// parseResolutions decodes the json int set in output_resolutions, falling back
// to the package default when absent or malformed.
func parseResolutions(raw *string) []int {
	if raw == nil || *raw == "" {
		return append([]int(nil), defaultOutputResolutions...)
	}
	var out []int
	if err := json.Unmarshal([]byte(*raw), &out); err != nil || len(out) == 0 {
		return append([]int(nil), defaultOutputResolutions...)
	}
	return out
}
