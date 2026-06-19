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

// BuildArgs assembles the complete ffmpeg argument list for one resolved profile
// at one target resolution, plus a JSON snapshot of the effective encode params
// for reproducibility (persisted on the encoded_outputs row). input/output are
// absolute file paths.
func BuildArgs(res Resolved, resolution int, tags ColorTags, input, output string) ([]string, string, error) {
	height, ok := resolutionHeights[resolution]
	if !ok {
		return nil, "", fmt.Errorf("unsupported target resolution %d", resolution)
	}

	x265 := buildX265Params(res, tags)
	vf := buildVideoFilters(res, height)

	args := []string{
		"-hide_banner",
		"-nostdin",
		"-i", input,
		"-map", "0",
		// Carry chapters and global metadata from the source into the output.
		"-map_chapters", "0",
		"-map_metadata", "0",
		"-c:v", "libx265",
		"-pix_fmt", "yuv420p",
		"-crf", strconv.FormatFloat(res.CRF, 'f', -1, 64),
		"-preset", res.Preset,
		"-x265-params", x265,
	}
	// Source color signaling: re-tag the container output so players read the
	// correct primaries/transfer/matrix instead of guessing. Only present tags
	// are emitted; an untagged source adds nothing.
	args = append(args, colorFlags(tags)...)
	args = append(args, "-vf", vf)
	args = append(args, audioArgs(res.Audio)...)
	// Subtitles + attachments: copy through for softsub-friendly mkv. ffmpeg
	// ignores a copy of an absent stream type, so this is safe across sources.
	args = append(args, "-c:s", "copy", "-c:t", "copy")
	// Explicit muxer (-f) so a temp output filename without a recognized
	// extension (e.g. "<final>.mkv.tmp") still selects the right container.
	args = append(args, "-f", muxerFor(res.Container))
	args = append(args, "-progress", "pipe:1", "-y", output)

	snapshot, err := buildSnapshot(res, resolution, height, x265, vf, tags)
	if err != nil {
		return nil, "", err
	}
	return args, snapshot, nil
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

// buildVideoFilters builds the -vf chain in the correct order: deinterlace
// (yadif) before any scaling, optional smartblur, then the scale to the target
// height (-2 width keeps the aspect ratio with an even dimension).
func buildVideoFilters(res Resolved, height int) string {
	var vf []string
	if res.Deinterlace {
		vf = append(vf, "yadif=1")
	}
	if res.SmartBlur {
		vf = append(vf, smartblurChain)
	}
	vf = append(vf, fmt.Sprintf("scale=-2:%d:flags=spline16+accurate_rnd+full_chroma_int", height))
	return strings.Join(vf, ",")
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
// string stored on the output row for reproducibility.
func buildSnapshot(res Resolved, resolution, height int, x265, vf string, tags ColorTags) (string, error) {
	snap := map[string]any{
		"profile_id":  res.ProfileID,
		"codec":       res.Codec,
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
