# ssanime-gui — Design Spec

**Date:** 2026-06-06
**Status:** Approved (design); pending implementation plan
**Location:** this repo

## 1. Purpose

A local, UI-first anime **download → encode → archive** manager. It downloads single or
multiple videos (torrents + direct/streaming-site links), re-encodes them with ffmpeg into a
smaller permanent x265 file, and manages the resulting local library. It can run in the
background and auto-fetch new episodes from watched feeds.

This is a scoped-down, personal reimagining of the original Django `automin` release pipeline.
It deliberately **excludes** the distribution side of `automin`: no tracker uploads, no seedbox
FTP, no torrent creation, no URL shortening, no public-facing site.

### Differentiator vs. Seanime

Seanime (the reference architecture) is an anime **media server** — it organizes, streams, and
transcodes *ephemerally for playback*. ssanime-gui is a **download-and-shrink-to-archive** tool —
it transcodes *durably to store* a smaller permanent file you keep. Different purpose, shared
architecture.

## 2. Tech Stack

| Layer | Choice | Why |
|---|---|---|
| Core | **Go** (daemon) | Single-binary output; reuses existing `encoder.go`; good ffmpeg/exec story |
| State | **SQLite** | Embedded, zero-config, single-file DB |
| Frontend | **Plain Svelte 5 + Vite** (no SvelteKit) | Builds to static `dist/`, no SSR/meta-framework baggage to fight |
| UI kit | **Tailwind + shadcn-svelte** (bits-ui) | Tailwind-first, copy-in components you own |
| Delivery | SPA embedded via **`go:embed`**, served over `localhost` to the browser | True single `.exe` |
| Feeds | **`mmcdole/gofeed`** | Standard Go RSS/Atom parser |
| Torrents | **`anacrolix/torrent`** (embedded), behind a `Downloader` interface | No external qBittorrent; preserves single-binary |
| Direct/HLS | **managed `yt-dlp`** binary | Only realistic engine for streaming-site extraction + HLS |
| Encoding | **ffmpeg** (x265), managed binary | Reuse existing encoder logic |
| Live updates | **SSE** (server-sent events) | Simpler than WebSocket for one-way progress/log streaming |
| Tray | system-tray icon | Background mode UX (Open / Pause / Quit) |

### Rejected alternatives (recorded so they don't get re-litigated)

- **Electron** — multi-hundred-MB, multi-file; fails the single-binary goal. (The `ssanime-gui-nuxt`
  branch went this way and is abandoned.)
- **Wails** — native-window shell coupled UI lifecycle to the app; "Wails friction" + Nuxt-in-webview
  build pain drove repeated restarts. The browser-served daemon model removes this entire class of bug.
- **Nuxt / SvelteKit / Astro** — meta-frameworks. Nuxt's SSR/webview mismatch was the original pain;
  SvelteKit would re-add the same meta-framework config in an embedded SPA; Astro is MPA/content-first
  and fights stateful dashboards. Plain Svelte+Vite is the lean, consistent choice.
- **React** — user preference against it.

## 3. Architecture — daemon-first

On launch the binary starts a long-running core, binds an HTTP server on `localhost:<port>`,
opens the default browser to it, and shows a system-tray icon. **The UI is a window into the
daemon, not the app itself** — closing the browser tab leaves downloads/encodes running; the tray
keeps the process alive. This is what makes "runs in background if required" fall out for free, and
mirrors `automin`'s Celery-worker model (work happens in the background; the UI only observes/commands).

```
┌─ one .exe ────────────────────────────────────────────┐
│  Svelte SPA (go:embed)  ──HTTP REST + SSE──▶  Go core  │
│                                                │        │
│   ┌──────────── Go core (goroutine workers) ───┴─────┐  │
│   │ feeds → download queue → encode queue → library  │  │
│   │  (gofeed)   (anacrolix/  (ffmpeg x265)  (SQLite)  │  │
│   │             torrent +yt-dlp)                      │  │
│   └────────────── manages ffmpeg / yt-dlp binaries ──┘  │
└─ system tray: Open UI · Pause · Quit ───────────────────┘
```

## 4. Modules (Go packages)

| Package | Responsibility | Key dependency |
|---|---|---|
| `server` | HTTP, REST handlers, SSE hub, serves embedded SPA | net/http (or chi) |
| `store` | SQLite persistence + migrations | database/sql + sqlite driver |
| `feeds` | RSS/scrape watchers: poll on interval, apply filter rules, enqueue matches | `mmcdole/gofeed` |
| `download` | Download manager behind a `Downloader` interface; backends: torrent + direct/HLS | `anacrolix/torrent`, yt-dlp |
| `encode` | ffmpeg x265 wrapper (reuses `encoder.go`), encode queue, profiles, progress parsing | ffmpeg (exec) |
| `library` | Organizes finished files, metadata, browse views | — |
| `binaries` | Locates / provisions / self-updates ffmpeg & yt-dlp | — |
| `queue` | Worker pools replacing Celery; per-stage concurrency caps | goroutines + channels |
| `events` | Pub/sub bus → pushes progress/logs to SSE clients | — |

Each package has one clear purpose and communicates through a narrow interface (e.g. the
`Downloader` interface lets torrent vs. direct backends — or a future qBittorrent backend — plug in
without touching `feeds`, `queue`, or `encode`).

## 5. Data model (SQLite)

`automin`'s model, scoped down:

- **Series** — `title`, `poster`, `metadata`, `default_profile_id`
- **Feed** — `url`, `type` (rss | scrape), `series_id`, `filter_rules` (quality/regex), `interval`, `last_checked`
- **Item** (episode) — `series_id`, `title`, `source_url`/`magnet`, `status`, `resolution`,
  `source_path`, `encoded_path`, `source_size`, `encoded_size`
- **EncodeProfile** — `name`, `codec` (x265), `crf`, `preset`, `x265_params`, `audio`, `scale`,
  `filters` (smartblur/yadif) — reuses the existing profile shape from `ssanime-gui`'s `profiles.go`
- **Settings** — paths (download dir, archive dir), per-stage concurrency, binary locations

**Status pipeline:** `queued → downloading → downloaded → encoding → encoded → archived` (+ `error`).
Mirrors `automin`'s `dlfin → enc → fin`.

## 6. Binary management strategy

ffmpeg and yt-dlp are **auto-downloaded on first run** into an app-data dir (not `go:embed`).

Rationale: **yt-dlp breaks weekly** as streaming sites change and must self-update; embedding it
would force a new app release on every site breakage. Auto-download keeps the shipped binary tiny
(~15–20 MB), lets yt-dlp self-update silently, and keeps large ffmpeg out of the binary. The shipped
artifact is still a single `.exe`; it provisions its tools on first launch.

(Fallback for zero-network requirement: `go:embed` both at ~100 MB+ with manual tool updates. Not chosen.)

## 7. UI (shadcn-svelte dashboard)

Pages:
- **Library** — series grid + archived files, source→encoded size savings
- **Queue** — live download + encode progress (SSE), per-item status
- **Auto-downloader** — feeds list + filter rules
- **Profiles** — encode presets (CRF, preset, x265 params, scale, filters)
- **Settings** — paths, concurrency, binary management
- **Logs** — streamed log view (SSE)

Tray menu: **Open UI · Pause all · Quit**.

## 8. Out of scope (explicitly deferred)

- Tracker/multi-site uploads, torrent creation, seedbox FTP, URL shortening (the `automin`
  distribution pipeline)
- Streaming/direct-play/media-server playback (that's Seanime's domain)
- qBittorrent/Transmission/external-client backends — interface allows adding later, not built now
- Mobile/remote access hardening (LAN access works incidentally via the localhost server, but is
  not a designed feature)

## 9. Reuse from prior attempts

- **`ssanime-gui`'s `encoder.go`** — ffmpeg exec wrapper, context cancellation, stderr progress
  parsing → becomes the basis of the `encode` package.
- **`ssanime-gui`'s `profiles.go`** profile shape → `EncodeProfile` model.
- **`automin`'s** encoding parameters (x265 CRF ~24, preset slow, aq-mode/psy-rd/deblock,
  resolution scaling 1080/720/480, smartblur/yadif filters) → encode profile defaults.
- Prior **Vue/PrimeVue frontends are not reused** — rebuilt in Svelte.
