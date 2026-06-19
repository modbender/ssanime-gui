# Roadmap

Deferred work and larger initiatives that are intentionally out of the current
change set. Each entry records the motivation, the concrete plan, and the trigger
for picking it up — so the context isn't lost between sessions.

## Smart poller (polling scalability)

**Status:** deferred. The current poller is a single global scheduler that is
correct and light on CPU, but naive at high subscription counts. Revisit when a
user actually tracks many series (target: smooth at 200+).

### Current design (as of 2026-06-13)

- **One global poller**, single goroutine (`internal/poller/poller.go`). Not
  per-series, not layered.
- **Scheduler tick:** 60s (`defaultInterval`). Each wake runs one
  `ListFeedsDueForPoll` query and acts on the result.
- **Per-feed interval:** 3600s / 1h (`feeds.interval_seconds`). A feed is due when
  `last_checked_at + interval_seconds <= now` (or `last_checked_at IS NULL`).
- **Sequential processing:** feeds are polled one at a time (`for _, feed := range
  feeds`), no fan-out. Gentle on sources, light on CPU.
- **Gate** (`db/queries/feeds.sql` `ListFeedsDueForPoll`): `feed.enabled = 1 AND
  series.subscribed = 1 AND series.watch_status = 'watching' AND airing_status NOT
  IN (CANCELLED, NOT_YET_RELEASED) AND interval elapsed`.

### Problems at scale (200+ subscribed series)

Not CPU — the machine is fine. The real issues:

1. **Thundering herd on first boot.** `last_checked_at IS NULL` counts as due, so a
   fresh start (or a bulk-subscribe) makes *every* feed due at once. The first pass
   fires N source requests back-to-back.
2. **Source-side rate-limiting / bans.** N rapid sequential requests to one source
   (nyaa, etc.) is the classic pattern that gets an IP throttled or temporarily
   banned. `offset_seconds` defaults to `0`, so there is no jitter; feeds added
   together re-cluster their due-times.
3. **Head-of-line blocking.** A slow/hanging source stalls the rest of that pass
   until its HTTP timeout fires.

### Planned mitigations (cheap, no architecture change)

- **Per-pass cap** — poll at most N feeds per 60s tick (e.g. 20). 200 due then drains
  over ~10 min instead of one burst, and stays naturally rate-limited.
- **Jitter on subscribe** — randomize `offset_seconds` / initial `last_checked_at` so
  due-times disperse instead of clustering.
- **Polite per-request spacing + a sane per-feed HTTP timeout** so one hung source
  can't stall a pass.

### Larger "smart poller" ideas (further out)

- Adaptive intervals: poll actively-airing series more often, finished/slow ones
  rarely; back off on repeated empty results.
- Per-source concurrency + rate-limit budgets (group feeds by provider, respect each
  source's politeness window independently).
- Priority queue keyed on next-due rather than re-scanning all feeds each tick.
- Surface poll health (last success, error streak, next-due) per series in the UI.

**Trigger to pick up:** a real user tracking enough series that the boot burst or a
source throttle is observed, or before any "bulk import / OPML subscribe" feature
ships.

## Non-torrent (direct / HLS) sources + yt-dlp download lane

**Status:** deferred. The whole app is torrent-first, and the sourcing contract is
torrent-*only* — there is currently no way for a source to be anything but a torrent.
This unlocks the dormant yt-dlp lane and covers the long tail of titles with no viable
torrent.

### Why it's wanted

A direct/HLS lane is a *fallback for the minority of episodes a torrent can't serve*:
dead/low-seeder torrents (niche, older, regional titles) and pre-torrent simulcasts (an
episode is on a streaming site hours before any torrent exists). It is **not** an
upgrade for the common case — the streaming sources actually worth pulling are
Widevine-DRM'd (yt-dlp can't touch those), and the DRM-free ones mostly rehost
re-encoded torrent rips. So: real value for the tail, low value for the bulk, and a
standing maintenance tax (yt-dlp breaks weekly as sites change).

### Current state (as of 2026-06-19) — torrent-only by contract

The download seam is already polymorphic, but the *source* contract is not:

- The `Downloader` interface (`internal/download`) was designed for multiple backends;
  only the embedded `anacrolix/torrent` backend exists. yt-dlp is provisioned-and-
  reachable in `internal/binaries` (`EnsureYtDlp`/`UpdateYtDlp`, kept dormant), but no
  code invokes it (startup provisioning was removed — nothing to download for).
- The blocker is upstream of the downloader: the `source.Provider` interface exposes
  only `GetTorrentMagnetLink` / `GetTorrentInfoHash` (`internal/source/types.go:156`),
  and the sole result type `AnimeTorrent` carries `Magnet` / `Link` / `InfoHash` only
  (`types.go:119`) — **no field can hold a direct/HLS video URL**. Extensions are
  tagged torrent-only: `IndexEntry.Type` is `"torrent"` and the only constant is
  `ExtTypeTorrent` (`internal/extension/types.go`). This is the Hayase format, whose
  extensions are all torrent providers. So a non-torrent source cannot even be
  *represented*, let alone downloaded.

### Plan (in dependency order — each step is useless without the prior)

1. **Grow the source contract.** Add a non-torrent source shape (a direct/HLS URL +
   kind) to the result type and a resolve method to `source.Provider`; add a non-torrent
   `ExtType` (e.g. `"hls"` / `"direct"`) and stop hardcoding `ExtTypeTorrent` on install.
2. **Route by source kind.** Teach autoselect + the download enqueue path to dispatch a
   non-torrent source to the right `Downloader` backend instead of assuming a magnet.
3. **Build the yt-dlp `Downloader` backend.** Invoke the (already-provisioned) yt-dlp
   binary for direct/HLS, with progress parsing into the existing pipeline; re-add the
   `EnsureYtDlp` startup provisioning and re-surface its Settings path field (both left
   dormant for exactly this).
4. **Fallback policy.** Define when the non-torrent lane is *preferred* (e.g. no
   seeders / no torrent within a window) vs an explicit alternate source, and how it
   shows in the UI.

### Open questions

- Does any extension in the ecosystem actually return direct/HLS sources, or would this
  require authoring a new extension *type* the Hayase format doesn't define? (If the
  latter, the contract change is ours to spec and there's no producer yet.)
- yt-dlp self-update: once the lane is live, wire `UpdateYtDlp` on the same silent
  background cadence as extension auto-update (it breaks weekly — same churn rationale).
- DRM reality check: scope which sources are even feasible before investing, so the lane
  isn't built for sources yt-dlp can't decrypt.

**Trigger to pick up:** a source extension actually starts returning non-torrent
(direct/HLS) links, or a concrete need for the no-viable-torrent tail is observed.
Until a *producer* of non-torrent sources exists, this is unreachable and stays
dormant.

## Subtitle burn-in (hardsub / MP4) + per-profile language preferences

**Status:** deferred. The encoder copies soft subs into mkv (`-c:s copy`) and has no
burn-in path and no audio/subtitle track selection. An MP4 output requires burning subs
(mp4 can't carry ASS/PGS soft subs), and doing that *well* pulls in a whole
language-preference layer — so the MP4/hardsub profile was skipped; the MKV softsub
profile + the encode-fidelity fixes (chapters, color, HQ scaler) shipped instead.

### Why deferred — it's bigger than automin's version

automin's MP4 path simply burned the **default** subtitle stream — ffmpeg's `subtitles=`
filter did everything, with no track-selection logic. That part is small. The size comes
from doing it properly with UI: users will expect to pick *which* language/track gets
burned (and which audio is kept), which is a real UI + backend task.

### Scope when picked up

- **Builtin (non-editable) profile:** burn the *default* subtitle stream, automin-style —
  no selection, simple.
- **User-editable profiles:** a preferred **audio language** and **subtitle language**,
  each with a **fallback**, chosen per profile.
- **UI:** straightforward — preferred + fallback language dropdowns for audio and subtitles
  on the Profiles editor.
- **Backend (the hard part):**
  - **Language normalization.** Stream language tags are inconsistent across releases —
    `Eng` / `EN` / `English` / `eng`, `jpn` / `Japanese`, etc. Needs an alias→canonical
    (ISO 639) map to match a user's preference against whatever the source tagged.
  - **Subtitle role distinction.** Releases sometimes split **Dialogue** subs from
    **Song/Signs** subs into separate tracks; selection must distinguish them (prefer full
    dialogue, optionally include signs/songs), not pick by language alone.
  - **Track selection + fallback resolution:** ffprobe the streams, match
    preferred → fallback → default, then map the chosen audio and burn the chosen subtitle.
- **Encoder capability:** the `subtitles=` burn-in vf step (with correct Windows
  filtergraph path escaping — `C\:/path`), subtitle-stream exclusion for mp4 (can't
  `-c:s copy` ASS/PGS), and chosen-audio mapping.

### Open questions

- Canonical language set + alias table (ISO 639 + common release spellings).
- How to expose the dialogue-vs-signs/songs choice without overcomplicating the UI.
- Whether audio/subtitle selection is per-profile or could vary per output resolution.

**Trigger to pick up:** when an MP4/hardsub output or user-facing audio/subtitle language
control is actually wanted. The encode-fidelity groundwork already shipped; this adds the
track-selection + burn-in layer on top.

## AI upscaling (super-resolution) — Anime4K / Real-ESRGAN

**Status:** deferred (exploratory). People upscale anime and re-upload it, and a "make my
low-res library HD" option is appealing — but quality upscaling is AI super-resolution, a
major GPU-dependent subsystem that fits awkwardly with this app's lean, GPU-agnostic,
shrink-to-archive design. Captured here; revisit on real demand.

### What it actually is

- **Anime4K** is primarily a set of GLSL shaders for *real-time playback* in mpv — it
  upscales while you watch, not to a file. By itself it is not an encode-to-archive tool.
- The engine used to upscale *and archive* anime is **Real-ESRGAN**
  (`realesrgan-ncnn-vulkan`, Vulkan/GPU, anime models e.g. `realesr-animevideov3`) or
  similar (`waifu2x-ncnn-vulkan`). Topaz Video AI is the commercial GUI equivalent (not
  integrable). A plain ffmpeg `scale`-up is *not* this — it adds no detail, just a bigger,
  softer file.

### Why it's a subsystem, not a profile

- **New GPU-dependent managed tool + models.** A second external binary (tens of MB + model
  files), Vulkan/GPU-bound — a yt-dlp-style provisioning + breakage burden, against the lean
  single-binary posture.
- **GPU-only and slow.** Super-res runs minutes-to-hours per episode on a GPU and is
  impractical on CPU. The daemon commonly runs headless on a NAS/mini-PC with no capable
  GPU, where the feature is unusable.
- **Breaks pipeline assumptions.** Needs a frames→super-res→re-encode stage (or a
  Vulkan-ffmpeg path), can't run concurrently with normal encodes (monopolizes the GPU), and
  produces much *bigger* files — orthogonal to the shrink-to-archive purpose.
- **Maintenance/quality variance.** Model updates, Vulkan driver issues, per-source quality
  variance.

### Approaches when picked up

1. **Real-ESRGAN stage (the real path).** Provision `realesrgan-ncnn-vulkan` + an anime
   model; GPU/Vulkan detection with a graceful "no GPU → feature disabled" fallback; extract
   frames → super-res → re-encode to x265; gate it so it never runs alongside normal encodes.
2. **ffmpeg `libplacebo` + Anime4K GLSL (lighter).** A single ffmpeg pass injecting Anime4K
   shaders via the `libplacebo` filter — no separate binary — *if* the managed ffmpeg build
   ships libplacebo+Vulkan (BtbN: needs verification; likely a separate/custom build) and the
   user has a GPU. Lower quality than Real-ESRGAN, still GPU-bound, fragile.

### Open questions

- Engine + model choice; where models are provisioned/stored.
- GPU/Vulkan detection + an honest no-GPU fallback (disable, don't silently CPU-grind).
- Can the managed ffmpeg carry libplacebo+Vulkan, or does Approach 2 need a separate build?
- Resource gating vs the normal encode queue (no concurrent GPU contention).
- Strictly opt-in (a per-profile flag), since it inverts the shrink-to-archive default.

**Trigger to pick up:** real user demand plus a GPU-equipped use case (e.g. archiving
DVD-era SD anime to HD) — not before. On a headless no-GPU host it can't run, so it only
makes sense once that audience is real.
