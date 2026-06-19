package encode

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/modbender/ssanime-gui/internal/defaults"
)

// resolutionHeights maps a target resolution label (1080/720/480) to the output
// frame height fed to the scale filter. scale uses -2 for width so the encoder
// derives an even, aspect-correct width from the source.
var resolutionHeights = map[int]int{
	1080: 1080,
	720:  720,
	480:  480,
}

// smartblurChain is automin's tuned smartblur filter string (proven values).
var smartblurChain = defaults.Values.Encode.SmartBlurChain

// containerMuxers maps a profile container to its ffmpeg muxer name. The muxer is
// passed explicitly with -f so the encoder writes to a temp file whose extension
// ffmpeg can't auto-detect (e.g. "<final>.mkv.tmp").
var containerMuxers = map[string]string{
	"mkv":      "matroska",
	"matroska": "matroska",
	"mp4":      "mp4",
	"webm":     "webm",
	"mov":      "mov",
}

// muxerFor returns the ffmpeg muxer for a container, defaulting to matroska.
func muxerFor(container string) string {
	if m, ok := containerMuxers[strings.ToLower(strings.TrimSpace(container))]; ok {
		return m
	}
	return "matroska"
}

// baseX265Params are the fixed, tuned x265 knobs from automin's proven recipe
// that are not exposed as inheritable profile columns. Per-profile knobs
// (aq-mode/aq-strength/deblock/psy-rd/psy-rdoq) and the raw passthrough are
// merged on top in buildX265Params.
var baseX265Params = defaults.Values.Encode.BaseX265Params

// isGPUCodec reports whether a profile codec selects the hardware lane. The
// virtual "gpu-auto" codec is resolved to a concrete encoder before BuildArgs.
func isGPUCodec(codec string) bool {
	return strings.EqualFold(strings.TrimSpace(codec), "gpu-auto")
}

// BuildArgs assembles the complete ffmpeg argument list for one resolved profile
// at one target resolution, plus a JSON snapshot of the effective encode params
// for reproducibility (persisted on the encoded_outputs row). input/output are
// absolute file paths. encoder is the concrete video encoder to use (libx265
// for the x265 lane, or the probed hardware encoder when the profile codec is
// gpu-auto). sel is the resolved per-track mapping from SelectTracks.
func BuildArgs(res Resolved, resolution int, tags ColorTags, sel TrackSelection, encoder, input, output string) ([]string, string, error) {
	height, ok := resolutionHeights[resolution]
	if !ok {
		return nil, "", fmt.Errorf("unsupported target resolution %d", resolution)
	}
	gpu := encoder != "" && encoder != cpuEncoder

	args := []string{
		"-hide_banner",
		"-nostdin",
		"-i", input,
	}
	// Stream mapping. The wildcard MKV-soft case keeps the legacy blind -map 0
	// (so chapters/attachments/data also carry through); every other case maps
	// video + the selected audio (and soft subs) explicitly.
	args = append(args, mapArgs(sel)...)
	// Carry chapters and global metadata from the source into the output.
	args = append(args, "-map_chapters", "0", "-map_metadata", "0")

	// Video codec + quality. GPU encoders ignore the entire x265 recipe.
	var x265, vf string
	if gpu {
		args = append(args, gpuVideoArgs(encoder, res, tags)...)
		vf = buildVideoFilters(res, height, sel, input, true)
	} else {
		x265 = buildX265Params(res, tags)
		vf = buildVideoFilters(res, height, sel, input, false)
		args = append(args,
			"-c:v", "libx265",
			"-pix_fmt", pixFmt(res.BitDepth),
			"-crf", strconv.FormatFloat(res.CRF, 'f', -1, 64),
			"-preset", res.Preset,
			"-x265-params", x265,
		)
	}
	// Source color signaling: re-tag the container output so players read the
	// correct primaries/transfer/matrix instead of guessing. Only present tags
	// are emitted; an untagged source adds nothing.
	args = append(args, colorFlags(tags)...)
	args = append(args, "-vf", vf)
	args = append(args, audioArgs(res.Audio)...)
	// Subtitles + attachments: copy through for softsub-friendly mkv when soft
	// subs are kept. When burning (no soft subs) the sub/attachment copy is
	// dropped — they would otherwise re-add the rendered track.
	if sel.SoftSubs && !sel.BurnSub {
		args = append(args, "-c:s", "copy", "-c:t", "copy")
	}
	// Explicit muxer (-f) so a temp output filename without a recognized
	// extension (e.g. "<final>.mkv.tmp") still selects the right container.
	args = append(args, "-f", muxerFor(res.Container))
	args = append(args, "-progress", "pipe:1", "-y", output)

	snapshot, err := buildSnapshot(res, resolution, height, x265, vf, tags, sel, encoder)
	if err != nil {
		return nil, "", err
	}
	return args, snapshot, nil
}

// mapArgs builds the -map flags for the selection. The non-explicit case is the
// MKV all-passthrough default: a single blind -map 0 (legacy behaviour). The
// explicit case maps video first, then each kept audio, then each kept soft sub.
func mapArgs(sel TrackSelection) []string {
	if !sel.Explicit {
		return []string{"-map", "0"}
	}
	// 0:V:0 (uppercase) selects the first real video stream, excluding
	// attached-picture streams (e.g. an embedded cover-art mjpeg) that 0:v:0
	// could otherwise grab and encode as a slideshow.
	args := []string{"-map", "0:V:0"}
	for _, idx := range sel.AudioStreamIndices {
		args = append(args, "-map", fmt.Sprintf("0:%d", idx))
	}
	if sel.SoftSubs && !sel.BurnSub {
		for _, idx := range sel.SubtitleStreamIndices {
			args = append(args, "-map", fmt.Sprintf("0:%d", idx))
		}
	}
	return args
}

// colorFlags emits the ffmpeg output color-signaling flags for the present tags,
// in a stable order (range, space, primaries, transfer). Absent tags emit nothing.
func colorFlags(tags ColorTags) []string {
	var args []string
	if tags.Range != "" {
		args = append(args, "-color_range", tags.Range)
	}
	if tags.Space != "" {
		args = append(args, "-colorspace", tags.Space)
	}
	if tags.Primaries != "" {
		args = append(args, "-color_primaries", tags.Primaries)
	}
	if tags.Transfer != "" {
		args = append(args, "-color_trc", tags.Transfer)
	}
	return args
}

// colorX265Params returns the x265 VUI params for the present color tags, in the
// same stable order as colorFlags. range uses the limited/full mapping.
func colorX265Params(tags ColorTags) []string {
	var parts []string
	if r := x265Range(tags.Range); r != "" {
		parts = append(parts, "range="+r)
	}
	if tags.Space != "" {
		parts = append(parts, "colormatrix="+tags.Space)
	}
	if tags.Primaries != "" {
		parts = append(parts, "colorprim="+tags.Primaries)
	}
	if tags.Transfer != "" {
		parts = append(parts, "transfer="+tags.Transfer)
	}
	return parts
}

// buildX265Params merges the fixed tuned base, the per-profile inheritable knobs,
// and the raw x265_params passthrough into one colon-joined x265-params string.
// Passthrough keys override earlier duplicates so a profile can always win.
func buildX265Params(res Resolved, tags ColorTags) string {
	parts := append([]string(nil), baseX265Params...)
	parts = append(parts,
		fmt.Sprintf("aq-mode=%d", res.AQMode),
		"aq-strength="+formatFloat(res.AQStrength),
		"deblock="+res.Deblock,
		"psy-rd="+formatFloat(res.PsyRD),
		"psy-rdoq="+formatFloat(res.PsyRDOQ),
	)
	// Source color VUI params, merged before the profile passthrough so a
	// profile can still override (dedupe keeps the last occurrence per key).
	parts = append(parts, colorX265Params(tags)...)
	if extra := splitParams(res.X265Params); len(extra) > 0 {
		parts = append(parts, extra...)
	}
	return dedupeLastWins(parts)
}

// gpuEncoderSpec maps a hardware HEVC encoder to its constant-quality argument
// shape. quality maps an x265-style CRF (0..51, lower = better) to that
// encoder's quality scale; flags(q) returns the encoder-specific arg tokens for
// the mapped quality. Verified against `ffmpeg -h encoder=<name>` (ffmpeg 7.0).
type gpuEncoderSpec struct {
	quality func(crf float64) int
	flags   func(q int) []string
}

// identityCQ keeps the CRF on the encoder's own 0..51 CQP/CQ scale (clamped),
// since NVENC/QSV/AMF/VAAPI all use a 0..51 quantizer where the CRF value maps
// directly closely enough for a constant-quality target.
func identityCQ(crf float64) int { return clampInt(int(crf+0.5), 0, 51) }

// videoToolboxQuality inverts CRF onto VideoToolbox's 0..100 scale (higher =
// better). CRF 0 -> 100, CRF 51 -> ~0; a linear inversion is the documented
// approximation since VideoToolbox exposes no CRF.
func videoToolboxQuality(crf float64) int {
	q := 100 - int(crf/51.0*100+0.5)
	return clampInt(q, 1, 100)
}

// gpuEncoderSpecs holds the per-encoder constant-quality recipe. NO x265 knobs
// (psy-rd/aq-mode/deblock/-x265-params) apply here — those are libx265-only.
var gpuEncoderSpecs = map[string]gpuEncoderSpec{
	"hevc_nvenc": {
		quality: identityCQ,
		flags: func(q int) []string {
			return []string{"-preset", "p7", "-tune", "hq", "-rc", "vbr", "-cq", strconv.Itoa(q), "-b:v", "0"}
		},
	},
	"hevc_qsv": {
		quality: identityCQ,
		flags: func(q int) []string {
			return []string{"-global_quality", strconv.Itoa(q), "-preset", "veryslow"}
		},
	},
	"hevc_amf": {
		quality: identityCQ,
		flags: func(q int) []string {
			qs := strconv.Itoa(q)
			return []string{"-rc", "cqp", "-qp_i", qs, "-qp_p", qs, "-qp_b", qs, "-quality", "quality"}
		},
	},
	"hevc_videotoolbox": {
		quality: videoToolboxQuality,
		flags: func(q int) []string {
			return []string{"-q:v", strconv.Itoa(q)}
		},
	},
}

// gpuVideoArgs builds the video-encoder args for a hardware HEVC encoder: the
// -c:v, GPU presets are always 8-bit yuv420p (device/player compatibility), the
// per-encoder constant-quality flags, and color signaling reuse. An unknown
// encoder (should not happen) falls back to a bare -c:v + cq.
func gpuVideoArgs(encoder string, res Resolved, tags ColorTags) []string {
	args := []string{"-c:v", encoder, "-pix_fmt", "yuv420p"}
	spec, ok := gpuEncoderSpecs[encoder]
	if !ok {
		return append(args, "-cq", strconv.Itoa(identityCQ(res.CRF)))
	}
	return append(args, spec.flags(spec.quality(res.CRF))...)
}

// clampInt bounds v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// pixFmt selects the output pixel format. 10-bit (yuv420p10le) nearly eliminates
// x265-introduced gradient banding on flat anime backgrounds; libx265 infers
// main10 from the 10-bit format, so no -profile:v is needed. Any other value is
// 8-bit (current behavior).
func pixFmt(bitDepth int) string {
	if bitDepth == 10 {
		return "yuv420p10le"
	}
	return "yuv420p"
}

// buildVideoFilters builds the -vf chain in the correct order: deinterlace
// (yadif) before any scaling, optional subtitle burn-in (rendered at the
// authored resolution so it scales proportionally with the video), optional
// smartblur, then the scale to the target height (-2 width keeps the aspect
// ratio with an even dimension), then optional deband (after scale so it
// operates at the output resolution). When gpu is true the smartblur/deband
// CPU filters still apply — only the encoder differs.
func buildVideoFilters(res Resolved, height int, sel TrackSelection, input string, gpu bool) string {
	var vf []string
	if res.Deinterlace {
		vf = append(vf, "yadif=1")
	}
	// Burn-in BEFORE scale: subs render at the source resolution then scale with
	// the video. si selects the chosen sub among subtitle streams.
	if sel.BurnSub {
		vf = append(vf, fmt.Sprintf("subtitles='%s':si=%d", escapeSubtitlesPath(input), sel.BurnSubFilterIndex))
	}
	if res.SmartBlur {
		vf = append(vf, smartblurChain)
	}
	vf = append(vf, fmt.Sprintf("scale=-2:%d:flags=spline16+accurate_rnd+full_chroma_int", height))
	if res.Deband {
		vf = append(vf, "deband")
	}
	return strings.Join(vf, ",")
}

// escapeSubtitlesPath escapes a path for the subtitles filter's quoted filename
// argument. The filtergraph parser unwraps one layer of single-quoting and the
// libavfilter option parser then treats `\` `:` specially, so a Windows path
// like C:\a'b must become C\:\\a\'b inside the single quotes. Order matters:
// backslash first so the escapes added for `:` and `'` are not re-escaped.
func escapeSubtitlesPath(p string) string {
	p = strings.ReplaceAll(p, `\`, `\\`)
	p = strings.ReplaceAll(p, `:`, `\:`)
	p = strings.ReplaceAll(p, `'`, `\'`)
	return p
}

// audioArgs maps the profile audio directive to ffmpeg args. "copy" passes the
// source audio through untouched; anything else is treated as an encoder name
// (e.g. "aac") so the data list stays open to new codecs without a switch.
func audioArgs(audio string) []string {
	a := strings.TrimSpace(audio)
	if a == "" || strings.EqualFold(a, "copy") {
		return []string{"-c:a", "copy"}
	}
	return []string{"-c:a", a}
}

// buildSnapshot serializes the effective encode parameters to a stable JSON
// string stored on the output row for reproducibility. It records the resolved
// encoder (e.g. hevc_nvenc or libx265 on fallback), burn-in, and the chosen
// audio/subtitle stream indices + languages so a re-encode is reproducible.
func buildSnapshot(res Resolved, resolution, height int, x265, vf string, tags ColorTags, sel TrackSelection, encoder string) (string, error) {
	snap := map[string]any{
		"profile_id":  res.ProfileID,
		"codec":       res.Codec,
		"encoder":     encoder,
		"crf":         res.CRF,
		"preset":      res.Preset,
		"resolution":  resolution,
		"height":      height,
		"audio":       res.Audio,
		"container":   res.Container,
		"x265_params": x265,
		"vf":          vf,
		"smartblur":   res.SmartBlur,
		"deinterlace": res.Deinterlace,
		"bit_depth":   res.BitDepth,
		"deband":      res.Deband,
		"burn_subs":   sel.BurnSub,
	}
	if len(sel.AudioStreamIndices) > 0 {
		snap["audio_stream_indices"] = sel.AudioStreamIndices
	}
	if sel.AudioLang != "" {
		snap["audio_lang"] = sel.AudioLang
	}
	if sel.BurnSub {
		snap["subtitle_stream_index"] = sel.BurnSubAbsoluteIndex
		snap["subtitle_lang"] = sel.BurnSubLang
	} else if len(sel.SubtitleStreamIndices) > 0 {
		snap["subtitle_stream_indices"] = sel.SubtitleStreamIndices
	}
	// Record only the present color tags for reproducibility.
	color := map[string]string{}
	if tags.Range != "" {
		color["range"] = tags.Range
	}
	if tags.Space != "" {
		color["space"] = tags.Space
	}
	if tags.Primaries != "" {
		color["primaries"] = tags.Primaries
	}
	if tags.Transfer != "" {
		color["transfer"] = tags.Transfer
	}
	if len(color) > 0 {
		snap["color"] = color
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return "", fmt.Errorf("marshal encode snapshot: %w", err)
	}
	return string(b), nil
}

// splitParams splits a raw x265_params string on ':' or ',' into individual
// key=value tokens, trimming blanks.
func splitParams(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ':' || r == ',' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// dedupeLastWins joins x265 params, keeping the last occurrence of each key so a
// passthrough override replaces an earlier default while preserving first-seen
// order for readability.
func dedupeLastWins(parts []string) string {
	type entry struct {
		idx int
		val string
	}
	seen := make(map[string]*entry, len(parts))
	order := make([]string, 0, len(parts))
	for _, p := range parts {
		key := p
		if i := strings.IndexByte(p, '='); i >= 0 {
			key = p[:i]
		}
		if e, ok := seen[key]; ok {
			e.val = p
			continue
		}
		seen[key] = &entry{idx: len(order), val: p}
		order = append(order, key)
	}
	out := make([]string, len(order))
	for i, key := range order {
		out[i] = seen[key].val
	}
	return strings.Join(out, ":")
}

// formatFloat renders a float without a trailing ".0" noise (1.0 -> "1", 1.25 ->
// "1.25") so x265-params read like automin's hand-written recipe.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// joinArgs renders an arg slice as a shell-ish command string for logging,
// quoting any token containing whitespace so the printed command is copy-runnable.
func joinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t") {
			parts[i] = `"` + a + `"`
		} else {
			parts[i] = a
		}
	}
	return strings.Join(parts, " ")
}
