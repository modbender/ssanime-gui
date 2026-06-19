# GPU encode lane + MP4/hardsub profiles — design

**Status: design — pending review.** Adds two capabilities to the encode engine and seeds two new
presets. The engine changes are orthogonal (composable axes), so profiles become any
codec×container×subtitle combination; the presets are just the shipped defaults.

Companion: `docs/roadmap.md` (the burn-in + language-preference entry this partially implements).

## Goals

Three encode intents, no redundancy:

| Preset | Codec | Container | Subtitles | Audio | Intent |
|---|---|---|---|---|---|
| **Automin (x265)** *(default, exists)* | x265 (CPU) | MKV | soft-copy | copy | Archive — smallest, best quality-per-byte |
| **Fast (GPU)** *(new)* | `gpu-auto` | MKV | soft-copy | copy | Same library shape, fast encode (don't care about size) |
| **Compatible (GPU, MP4)** *(new)* | `gpu-auto` | MP4 | hardsub (burn default) | AAC | Plays anywhere — mobile/browser/old TVs |

Non-goals (stay roadmapped): explicit English/language *preference* selection, Dialogue-vs-Signs/Songs
role disambiguation, fixing mistagged tracks. v1 uses the **default-flagged** track — verified more
reliable than language tags on the real library.

## Composable axes (the real engine work)

The `encode_profiles` table already has `codec` and `container`. Three additions make profiles fully
composable:

1. **`codec: "gpu-auto"`** — a virtual codec resolved at encode time to the best available hardware
   HEVC encoder (detect + probe). Existing `"x265"` is unchanged.
2. **`burn_subs` (new nullable bool column)** — inheritable like the other knobs (child → parent →
   fallback default `false`). When true, the default subtitle track is rendered into the video and
   `-c:s`/`-c:t` are dropped.
3. **MP4 muxer** — already wired (`containerMuxers["mp4"] = "mp4"`). Add a guard: MP4 + a non-text
   (ASS/PGS) source subtitle ⇒ `burn_subs` is forced true (copying ASS into MP4 fails). This is the
   only implicit coupling; otherwise the axes are independent.

## GPU detection + probe (`internal/encode/hwprobe.go`, new)

No ffmpeg "auto GPU encoder" exists (`-hwaccel auto` is decode-only), so we detect ourselves.

- **Candidate encoders by platform** (HEVC), in priority order:
  - Windows: `hevc_nvenc`, `hevc_qsv`, `hevc_amf`
  - Linux: `hevc_nvenc`, `hevc_vaapi`, `hevc_qsv`
  - macOS: `hevc_videotoolbox`
- **Probe:** for each candidate, run a throwaway encode of a tiny synthetic source
  (`-f lavfi -i nullsrc=...:d=0.1 -c:v <enc> -f null -`). Compiled-in ≠ functional — only a real
  encode proves the GPU + driver are present. First success wins.
- **Cache:** probe once per process (sync.Once / lazy), store the resolved encoder name + a "none"
  sentinel. Re-probe is cheap to skip; a daemon restart re-probes (handles eGPU/driver changes).
- **Fallback:** if no hardware encoder probes successfully, `gpu-auto` falls back to **libx265**
  (CPU) and logs a warning + emits an event, so a GPU profile still produces output on a GPU-less
  host rather than failing every episode. The output snapshot records the *actual* encoder used.
- **Exposure:** a `ResolveGPUEncoder() (name string, isCPUFallback bool)` consumed by the args
  builder.

## Args builder: codec-family dispatch (`internal/encode/args.go`)

`BuildArgs` branches on codec family. The x265 path is unchanged. The GPU path is a separate builder
because hardware encoders **ignore the entire x265 recipe** (`psy-rd`, `aq-mode`, `deblock`, `me`,
`-x265-params`, etc. are all x265-only).

- **x265 path:** current behavior (unchanged).
- **GPU path (per encoder):** quality-targeted constant-quality flags, e.g.
  - NVENC: `-c:v hevc_nvenc -preset p7 -tune hq -rc vbr -cq <crf-mapped> -b:v 0`
  - QSV: `-c:v hevc_qsv -global_quality <q> -preset veryslow`
  - AMF: `-c:v hevc_amf -quality quality -rc cqp -qp_i/-qp_p <q>`
  - VideoToolbox: `-c:v hevc_videotoolbox -q:v <q>` (or `-b:v`)
  - VAAPI: `-c:v hevc_vaapi -rc_mode CQP -qp <q>` (+ `hwupload`/format setup)

  The profile `crf` is mapped to each encoder's quality scale via a small per-encoder mapping table
  (not a 1:1 reuse — NVENC `cq` ≈ CRF, others differ). **8-bit (`yuv420p`) for GPU presets** (max
  device compat; 10-bit NVENC needs Pascal+ and many MP4 players reject 10-bit). `bit_depth` is still
  honored if a user sets it and the encoder supports it.
- **Exact flag names + quality mapping are verified against `ffmpeg -h encoder=<name>` / context7 at
  implementation time** (per project rule). The spec fixes the *shape*, the implementer fixes the
  literals.

## Subtitle burn-in (when `burn_subs` is true)

- **Track selection:** ffprobe the source subtitle streams; pick the **disposition=default** track,
  else the first text-based (ASS/SRT) track, else PGS/image if that's all there is. Map to the
  subtitle filter's `si` index (index *among subtitle streams*, not absolute). If no subtitle stream
  exists → skip burn (encode clean video), log info.
- **Filter:** `subtitles='<escaped input path>':si=<n>` (carries ASS styling). Burn **before** the
  scale so subs render at the authored resolution then scale proportionally with the video:
  `[yadif] → subtitles → smartblur? → scale → deband?`. Path escaping on Windows (`\`, `:`, `'`)
  handled by an escape helper + test.
- **Stream mapping:** with burn-in we no longer `-map 0` blindly (that would also try to mux the
  now-redundant sub/attachment streams). Map video + the chosen audio; drop `-c:s`/`-c:t`.

## Audio for MP4

MP4 can't reliably carry FLAC/Opus. The Compatible preset uses **AAC** (`-c:a aac -b:a 192k`,
default audio track). The two MKV presets keep `audio: "copy"`. This is just the existing `audio`
profile field set to `aac` vs `copy` — no new mechanism, but `audioArgs` already treats any
non-"copy" value as an encoder name, so `"aac"` works today.

## Data model + defaults

- **Migration `00015_encode_profiles_burn_subs.sql`** (goose): `ALTER TABLE encode_profiles ADD
  COLUMN burn_subs INTEGER` (nullable; NULL = inherit/fallback false). Regenerate sqlc.
- **`internal/defaults`:** add `default_burn_subs: false` to the encode fallback block; add the two
  new profile entries to the `profiles` array (codec `gpu-auto`, the Fast/MKV and Compatible/MP4
  shapes above). `Profile` + `Encode` structs grow the field. `defaults_test.go` updated.
- **`internal/store/seed.go`:** already iterates `defaults.Values.Profiles` (post central-defaults),
  so seeding the two new presets is automatic once they're in the JSON. Verify `seedBuiltinProfiles`
  passes `burn_subs` through.
- **`ProfileResolver` / `Resolved`:** add `BurnSubs bool`; resolve via the same COALESCE
  child→parent→fallback chain. `codec` already flows through.

## Reproducibility (snapshot)

The encode snapshot records `codec` as the **resolved** encoder (e.g. `hevc_nvenc` or `libx265`
on fallback), plus `burn_subs`, the chosen subtitle stream index, and audio codec — so a re-encode is
reproducible and the UI can show "encoded on GPU/CPU".

## Failure modes (premortem)

1. **GPU probe false-positive** (encoder present, encode fails mid-stream on real content) → the
   per-output error path already parks the output in `error` and keeps the source for retry; a user
   can switch that series to the CPU preset. Acceptable; logged.
2. **No subtitle track but `burn_subs` true** → skip burn, encode clean (don't fail the output).
3. **ASS into MP4 without burn** → the MP4+non-text-sub guard forces `burn_subs`, so this can't ship
   a broken profile.
4. **Path escaping** for the `subtitles` filter on Windows — explicit helper + unit test (a colon in
   a drive letter breaks the naive form).
5. **GPU + multi-res concurrency** — unchanged: still one episode at a time, resolutions sequential;
   each GPU encode saturates the encoder. No new concurrency.

## Out of scope (roadmap)

- Language/role-aware track selection (prefer-English subs, Dialogue vs Signs/Songs, mistag repair).
- 10-bit GPU presets, HDR passthrough.
- The single-decode fan-out (separate optimization, still flagged).

## Test plan

- `hwprobe`: table test of platform→candidate ordering; probe parsing with a fake runner; fallback to
  libx265 when all probes fail.
- `args` (GPU): each encoder family emits its expected `-c:v` + quality flags and **no** `-x265-params`.
- `args` (burn): filter chain order with burn before scale; `si` index selection; no-subs skip; MP4
  forces burn for ASS; Windows path escaping.
- `defaults`/`seed`: three builtin profiles parsed + seeded with correct codec/container/burn_subs.
- Existing encode/store tests stay green.
