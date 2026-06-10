# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current state

This repo is **implemented and building**. The Go daemon (`cmd/ssanime` + 17 `internal/` packages),
the embedded Svelte SPA (`frontend/`, built to `internal/server/dist` and `go:embed`-ed), and the
Tauri desktop shell (`desktop/`) all exist and compile; `go test ./...` is green (~97 tests across
17 packages) and CI gates every push/PR on `main`. The end-to-end pipeline (DoH→nyaa search →
embedded `anacrolix/torrent` download → ffmpeg multi-resolution encode → Jellyfin-style archive →
cleanup) has been validated against a real torrent. Remaining work is feature breadth, polish, and
distribution — not bring-up.

Implemented `internal/` packages: `anilist` (GraphQL metadata), `binaries` (ffmpeg/yt-dlp provision
+ checksum), `config`, `doh` (SSRF-guarded DNS-over-HTTPS), `download` (anacrolix + qBittorrent/
Transmission backends behind `Downloader`), `encode` (ffmpeg x265, ffprobe-anchored progress,
multi-res fan-out), `events` (SSE hub), `extension` (goja/hibike runtime), `poller` (RSS/scrape feed
watcher → enqueue), `procguard` (Windows job-object so a force-killed daemon doesn't orphan ffmpeg),
`server` (REST + SSE + `localGuard` CSRF/rebind defense), `source` (nyaa/subsplease providers,
habari parsing, autoselect), `store` (sqlc/goose/`modernc.org/sqlite`, dual read/write pool), `tray`.

The original spec at `docs/superpowers/specs/2026-06-06-ssanime-gui-design.md` was **substantially
refined** during build — the source of truth for the data model, sourcing, and decisions is
**`docs/reference/`**:
- `schema-from-automin.md` — the schema (derived from the proven `automin` Django models), encode-
  profile inheritance, sourcing, the comprehensive-v1 additions, and all resolved decisions.
- `db-layer-decision.md` — sqlc + goose + `modernc.org/sqlite` + the single-writer pool.
- `seanime-architecture.md` — patterns borrowed from the Seanime clone (`D:\Projects\gui\seanime`).
- `transcript-wails-*.md` — why the prior Wails/Electron attempts were abandoned.

Read `docs/reference/` before the old spec where they conflict. Key decisions baked into the build:

- **License: GPL-3.0** (the app reuses GPL `habari` + adapts GPL Seanime code).
- **Build posture: comprehensive, no rush** — include AniList metadata, multi-resolution output, a
  goja JS extension runtime (hibike interface), and external torrent-client backends.
- **Sourcing: torrents-primary**, with a **DoH resolver baked in** (the dev machine's ISP DNS-blocks
  nyaa.si; DoH via `1.1.1.1` defeats it — verified). yt-dlp/direct/HLS still deferred behind the
  `Downloader` interface.
- **DB: sqlc/goose/`modernc.org/sqlite`** (cgo-free), single-writer pool + WAL. The core entity table
  is **`episodes`** (not `items`).

## What this is

A local, UI-first anime **download → encode → archive** manager. It downloads videos (torrents +
direct/streaming-site links), re-encodes them with ffmpeg into smaller permanent x265 files, and
manages the resulting local library. It runs in the background and auto-fetches new episodes from
watched RSS/scrape feeds.

It is a scoped-down personal reimagining of the Django `automin` release pipeline, deliberately
**excluding** the distribution side (no tracker uploads, no seedbox FTP, no torrent creation, no
URL shortening, no public site). Unlike Seanime (which streams/transcodes *ephemerally for
playback*), this tool transcodes *durably to store* a smaller permanent file.

## Architecture — daemon-first

The single `.exe` starts a long-running Go core, binds an HTTP server on `localhost:<port>`, opens
the default browser to it, and shows a system-tray icon. **The UI is a window into the daemon, not
the app itself** — closing the browser tab leaves downloads/encodes running; the tray keeps the
process alive. This is what makes background operation fall out for free.

```
one .exe:
  Svelte SPA (go:embed)  ──HTTP REST + SSE──▶  Go core (goroutine workers)
                                                feeds → download queue → encode queue → library
  system tray: Open UI · Pause all · Quit
```

Pipeline of statuses (mirrors automin's `dlfin → enc → fin`):
`queued → downloading → downloaded → encoding → encoded → archived` (+ `error`).

## Go packages

Each package has one purpose and communicates through a narrow interface. The `Downloader`
interface is the key seam: torrent (embedded `anacrolix`) and external-client (qBittorrent/
Transmission) backends plug in without touching `poller` or `encode`. Per-stage worker pools live
inside `download` and `encode` themselves rather than in a separate `queue` package.

| Package | Responsibility |
|---|---|
| `server` | HTTP, REST handlers, SSE hub, serves embedded SPA, `localGuard` (CSRF/DNS-rebind defense) |
| `store` | SQLite persistence + sqlc/goose migrations, dual read/write pool |
| `poller` | RSS/scrape feed watchers: poll on interval, apply filter rules, autoselect, enqueue (`mmcdole/gofeed`) |
| `source` | Providers (nyaa, subsplease), habari release-name parsing, SmartSearch + autoselect |
| `download` | Download manager behind `Downloader`; backends: embedded `anacrolix/torrent`, qBittorrent, Transmission. Owns its worker pool |
| `encode` | ffmpeg x265 wrapper, encode worker pool, profiles, ffprobe-anchored progress, multi-resolution fan-out |
| `anilist` | AniList GraphQL metadata (cover image/color, banner, titles, airing status) |
| `binaries` | Locates / provisions / checksum-verifies ffmpeg & yt-dlp into app-data |
| `doh` | SSRF-guarded DNS-over-HTTPS resolver (bypasses ISP nyaa.si DNS block) |
| `extension` | goja JS extension runtime implementing the hibike provider interface |
| `events` | Pub/sub bus → pushes progress/logs to SSE clients |
| `procguard` | Windows job-object so a force-killed daemon reaps its ffmpeg children (no orphans) |
| `tray` | System-tray icon (Open UI / Pause all / Quit) |
| `config` | App-data paths, settings load/save |

## Data model (SQLite)

- **Series** — `title`, `poster`, `metadata`, `default_profile_id`
- **Feed** — `url`, `type` (rss \| scrape), `series_id`, `filter_rules` (quality/regex), `interval`, `last_checked`
- **Item** (episode) — `series_id`, `title`, `source_url`/`magnet`, `status`, `resolution`, `source_path`, `encoded_path`, `source_size`, `encoded_size`
- **EncodeProfile** — `name`, `codec` (x265), `crf`, `preset`, `x265_params`, `audio`, `scale`, `filters` (smartblur/yadif)
- **Settings** — paths (download dir, archive dir), per-stage concurrency, binary locations

## Binary management

ffmpeg and yt-dlp are **auto-downloaded on first run** into an app-data dir (not `go:embed`).
Rationale: yt-dlp breaks weekly as streaming sites change and must self-update; embedding it would
force a release on every breakage. Auto-download keeps the shipped binary ~15–20 MB. Do not embed
these binaries — that decision was made deliberately.

## Locked decisions — do not re-litigate

These were settled in the spec after repeated restarts. Do not switch frameworks/shells without
explicit user approval (per the "design pivots require approval" rule):

- **Go daemon + browser-served SPA**, not a native-window shell. Rejected: **Electron**
  (multi-hundred-MB, fails single-binary goal — the `ssanime-gui-nuxt` branch went this way and is
  abandoned); **Wails** (native-window lifecycle coupling + Nuxt-in-webview pain).
- **Plain Svelte 5 + Vite** (no SvelteKit), building to a static `dist/` embedded via `go:embed`.
  Rejected: **Nuxt / SvelteKit / Astro** (meta-framework SSR/config baggage in an embedded SPA);
  **React** (user preference against it).
- **Tailwind + shadcn-svelte (bits-ui)** for UI; **SSE** (not WebSocket) for one-way progress/log
  streaming; **anacrolix/torrent** embedded (no external qBittorrent); **managed yt-dlp** for
  streaming-site/HLS extraction.

## Frontend tooling

Per global preference, use **`bun`** for the Svelte/Vite frontend (`bun install`, `bun add`,
`bun run`, `bunx`), not npm/pnpm/yarn.

## Reuse from prior attempts

The abandoned Wails build lives at `D:\Projects\wails\ssanime-gui` (Go + Wails v2 + Nuxt 4). Mine
it for the encode logic; do **not** copy its shell architecture (Wails native window — rejected).

- **`services/encoder.go`** → basis of the `encode` package. It already has: ffmpeg discovery
  (`exec.LookPath` + common Windows paths), `exec.CommandContext` in a goroutine, `Stop()` via
  context-cancel + process-kill, `sync.RWMutex`-guarded state, and stderr progress parsing.
  Two known gaps to fix when porting, not inherit:
  - `buildFFmpegArgs` only emits `-c:v libx265 -crf <CRF> -preset medium -c:a copy` — the rich
    `EncodingProfile` fields (`PsyRD`, `AQMode`, `BFrames`, `Deblock`, `SmartBlur`, `Deinterlace`,
    `MultiResolution`, scale, etc.) are **defined but not wired**. Wire them through here.
  - Progress is per-file-count, not within-file — it parses `speed=` but never reads total
    duration. Real percent needs the input duration (ffprobe / parse `Duration:` from stderr).
- **`services/profiles.go`** `EncodingProfile` struct → `EncodeProfile` model. Prior storage was
  `~/.ssanime-gui/profiles.json`; this project moves profiles into **SQLite**. Three seeded
  defaults to carry over: **High Quality** (CRF 18), **Balanced** (CRF 23), **Fast** (CRF 28).
- Prior single-encode-at-a-time mutex is replaced by the `queue` package's per-stage worker pools.
- `automin`'s encoding params (x265 CRF ~24, preset slow, aq-mode/psy-rd/deblock, 1080/720/480
  scaling, smartblur/yadif) → encode profile defaults.
- The Wails Go module is named `wails-nuxt4-template` (template cruft) — irrelevant here; new
  module gets a real path. Prior Vue/PrimeVue, Nuxt, and Electron frontends are **not** reused.

## Reference: Seanime (architecture model, not a dependency)

`https://github.com/5rahim/seanime` is the shared-architecture reference: Go backend, embedded
web UI, single binary. ssanime-gui borrows the **daemon + embedded-SPA + Go-worker** shape but
diverges deliberately: Seanime uses React/Rsbuild, an Electron desktop shell ("Denshi"), **external**
torrent clients (qBittorrent/Transmission/Torbox/Real-Debrid), and **on-the-fly transcoding for
playback**. ssanime-gui uses Svelte, a **browser-served** daemon (no Electron), an **embedded**
`anacrolix/torrent` (no external client), and **durable transcode-to-archive** (no playback). Read
Seanime for patterns; don't assume feature parity.

## Out of scope (explicitly deferred)

Tracker/multi-site uploads, torrent creation, seedbox FTP, URL shortening; streaming/media-server
playback; external torrent-client backends (interface allows adding later, not built now); mobile/
remote-access hardening.
