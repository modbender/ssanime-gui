# Development History: the prior Wails workspace → ssanime-gui Rewrite

Mined from the Claude Code transcript that produced the current project. This document captures
*why* the daemon-first Go + Svelte stack was chosen, what the three earlier attempts in the prior
Wails workspace tried, what broke, and what to carry forward.

> **Scope of the transcript:** This is almost entirely a **brainstorming / stack-decision
> session**, not an implementation session. It ends at the moment the design spec is written —
> no Go or Svelte code was written yet in the new project. The bulk of the technical substance
> comes from two `Explore` subagents that reverse-engineered (a) the original Django `automin`
> app and (b) the three prior Wails/Electron attempts. The design that came out of it is captured
> in `docs/reference/`.

---

## 1. Timeline

1. **Opening ask.** User: *"I have been trying to build a golang app version of the automin Django
   project, but I just can't find the right tech stack."* The idea: a local,
   UI-first video manager that downloads single/multiple anime videos, encodes them to a smaller
   format (ffmpeg), runs in the background when needed, and manages a local library.

2. **`superpowers:brainstorming` skill invoked** — enforced a design-before-code gate. No code
   was written this session as a result.

3. **Two parallel `Explore` subagents dispatched:**
   - **Subagent A** reverse-engineered the Django `automin` pipeline.
   - **Subagent B** audited the three folders in the prior Wails workspace.

4. **Findings synthesized.** Key realization: the three Go/desktop attempts had been *circling
   one piece* of automin — the **encoder** — and each restart lost that work. The assistant named
   the real problem: **stack ping-pong between Wails(Go) and Electron(Node), and between Nuxt
   3/4**, not the encoder itself.

5. **First clarifying-question batch (scope / source / pain point).** User answers:
   - Scope = **"Personal + auto-fetch"** (personal library + RSS auto-download; *no* upload/
     distribution side of automin).
   - Sources = **Torrents + direct links + scraping** (like automin).
   - Pain points = **"Nuxt fights desktop", "Wails friction"**, plus the explicit goal:
     *"my goal is to possibly ship out only 1 binary or 1 file not multiple, this will be an
     open source video manager."*

6. **User drops a reference: `https://github.com/5rahim/seanime`.** This is treated as decisive.
   A second stack-question batch was interrupted/rejected by the user so the Seanime angle could
   be explored first.

7. **Seanime fetched via WebFetch and analyzed.** It validated the daemon-first / single-binary /
   embedded-web-UI architecture (details below).

8. **Frontend framework converged through three more question rounds:**
   - User: *"reference arch, but I hate react, can we use something else? astro? vue? nuxt?"*
   - Astro rejected; Vue endorsed; then user chose **Svelte/SvelteKit** with *"mainly tailwind
     focused, torn between nuxt ui or shadcn."*
   - Resolved to **plain Svelte 5 + Vite (no SvelteKit) + Tailwind + shadcn-svelte.**

9. **Final design presented and approved** ("Yes, all of it"), including the two open
   recommendations (embed `anacrolix/torrent`; auto-download ffmpeg/yt-dlp).

10. **New project located in this repo** (per user) and the design spec
    written. Session ends asking the user to review the spec before moving to `writing-plans`.

---

## 2. Technical Decisions & Rationale

### Final stack (approved)
> **Go daemon core · SQLite state · embedded plain Svelte 5 + Vite SPA (Tailwind + shadcn-svelte)
> served over `localhost` to the browser · one shippable `.exe`.**

| Decision | Choice | Stated WHY |
|---|---|---|
| **Architecture** | **Daemon-first** — long-running Go core binds an HTTP server on `localhost`, opens the browser, drops a system-tray icon. UI is "a window into the daemon," not the app. | The user's requirements ("auto-fetches feeds, downloads, encodes, runs in background if required") describe *a background service that happens to have a UI*, not a UI that happens to do work. Once the daemon is the core, the window-shell tech stops being load-bearing — **this is the reframe that kills the restart cycle.** Mirrors automin's Celery-worker model. |
| **Backend language** | **Go** | Compiles to a single `.exe` with the frontend embedded (`go:embed`); reuses the existing `encoder.go`; good ffmpeg/exec story. The "ship one binary" requirement **eliminates Electron** (inherently a multi-hundred-MB folder) and favors Go. |
| **State** | **SQLite** | Embedded, zero-config, single-file DB. |
| **Frontend framework** | **Plain Svelte 5 + Vite** (explicitly **NOT SvelteKit**) | Builds straight to a static `dist/` that `go:embed` swallows whole; zero SSR/meta-framework concepts to disable. The consistency check the assistant flagged: *"SvelteKit is to Svelte what Nuxt is to Vue"* — choosing Kit would re-add the exact meta-framework config layer that caused "Nuxt fights desktop." |
| **UI kit** | **Tailwind + shadcn-svelte** (built on `bits-ui` primitives) | Matches the user's "mainly tailwind focused + shadcn" preference. Copy-in components you own. |
| **Feeds/RSS** | **`mmcdole/gofeed`** | Standard Go RSS/Atom parser; replaces automin's `feedparser`. |
| **Torrents** | **Embed `anacrolix/torrent`** (pure-Go), behind a `Downloader` interface | No external qBittorrent for the user to install/run → preserves single-binary. Interface lets a qBittorrent backend be added later without touching the rest. (Note: Seanime drives *external* clients, but it's a full media server; for a personal single-binary tool, embedding is the cleaner fit.) |
| **Direct/scraping downloads** | **Managed `yt-dlp` binary** | Only realistic engine for streaming-site extraction + HLS; pure-Go alternatives (`lux`) cover far fewer sites and lag badly. |
| **ffmpeg + yt-dlp delivery** | **Auto-download on first run** into an app-data dir (NOT `go:embed`) | **yt-dlp breaks weekly** as sites change and must self-update; embedding it would force a new release on every site breakage. Auto-download keeps the shipped binary tiny (~15–20 MB) and lets yt-dlp self-update silently; ffmpeg (huge) stays out of the binary. You still ship *one* `.exe`; it provisions tools on first launch. `go:embed` (~100 MB+, manual updates) is the zero-network fallback. |
| **Background queue** | Goroutine **worker pools** with separate concurrency caps per stage (downloads parallel; encodes 1–2 at a time, CPU-bound) | Direct replacement for automin's Celery fast/medium/slow queues, without Redis. |
| **Live UI updates** | **HTTP REST + SSE** hub (events pub/sub bus → SSE clients) | Hand-rolled bridge for live progress/logs; fits the browser-served daemon. |

### Frameworks explicitly REJECTED and why

- **Electron** — *"ship one binary eliminates Electron"* (multi-hundred-MB folder). The
  user's own `ssanime-gui-nuxt` Electron branch confirmed the bloat. *"Your instinct to leave the
  Electron branch was right."*
- **Nuxt** (both as Wails frontend and standalone) — root cause of *"Nuxt fights desktop."*
  Nuxt is an SSR/file-routing web meta-framework; *"almost everything it adds (server routes,
  hydration, universal rendering) is dead weight inside a desktop shell, and that mismatch is most
  of your friction."* Became *technically viable* once the webview was gone (run `ssr:false` +
  static generate) but unnecessary.
- **Astro** — *"wrong tool here."* Content/MPA-first (islands, static pages); this app is a
  *stateful dashboard* (live progress bars, queues, real-time SSE). *"You'd fight Astro's grain
  the whole way."*
- **Wails (as the primary shell)** — not rejected outright, but demoted. Originally "Approach A"
  (keep Wails, drop Nuxt). Superseded by the Seanime data point: a mature project in this exact
  niche chose a browser-served daemon, **not** a Wails-style native window, as the primary shell.
- **Tauri (Rust)** — considered ("Approach C"): most polished DX, smallest binaries, first-class
  ffmpeg/yt-dlp sidecar handling. Rejected because it means **rewriting the Go encoder/download
  logic in Rust** — throwing away the reusable Go work for a steeper curve.
- **React** — Seanime's choice, explicitly declined: user *"hates react."* The point made: in the
  Seanime model the frontend is *just a normal static web app embedded in the Go binary*, so React
  is Seanime's choice, not an architectural requirement.

### What Seanime (`5rahim/seanime`) confirmed
- Go core + embedded web UI + background server **accessed in a browser** = the chosen architecture
  (the old "Approach B"), validated by a shipped, popular project.
- **Single binary, cross-platform** — frontend (React + Rsbuild) bundled/embedded into the Go
  binary.
- Its optional native desktop wrapper ("Seanime Denshi") is a *separate* Electron app layered on
  top — i.e. native window kept **decoupled** from the core. Exactly the daemon-first reframe.
- **RSS auto-downloader with filters** already a solved pattern there.
- Even Seanime drives *external* torrent clients (qBittorrent/Transmission/Torbox/Real-Debrid)
  rather than embedding one — a signal weighed against, but ultimately overridden for the
  single-binary goal.
- **Key differentiator identified:** Seanime is a media *server* (anime Jellyfin) that transcodes
  *ephemerally for playback*. ssanime-gui is a **download-and-shrink-to-archive** tool that
  transcodes *durably to store* a smaller permanent x265 file. Different purpose, shared
  architecture.

---

## 3. Pain Points, Failures, Dead-Ends

The central failure was **repeated rescaffolding / stack ping-pong**, not any single bug. The
assistant's diagnosis: *"you've ping-ponged between Wails(Go) and Electron(Node), and between
Nuxt 3/4 — and each restart lost the encoder work. That ping-pong is the problem worth solving,
not the encoder itself."*

User's own words on the causes (from the question answers):
- *"Nuxt fights desktop, Wails friction, I found wails + my favorite stack of ui (Nuxt) not
  exactly fitting nicely, lots of bugs and such."*
- The single hard constraint that retroactively explains the failures: *"my goal is to possibly
  ship out only 1 binary or 1 file not multiple."*

Concrete dead-ends in the three prior attempts (from Subagent B):

| Folder | Stack | State / why it stalled |
|---|---|---|
| Wails `ssanime-gui` (prior local Wails attempt) | **Wails v2.10.2 + Go + Nuxt 4 (Vue 3, Nuxt UI, Pinia, pnpm)** | **Furthest along (~70% of the encoder).** Real Go encoder service. Module still named generic `wails-nuxt4-template`. Known gaps: many x265 profile fields defined but **only CRF + preset actually wired to the ffmpeg CLI**; progress is coarse (per-file count, not within-file). |
| Electron/Nuxt attempt (`ssanime-gui-nuxt`) | **Electron + Node/TS + Nuxt 3 + PrimeVue** | More UI polish (Queue, Progress, Settings tabs, full GitHub Actions release matrix) **but dropped Go for Node.** Left **5 stale, unused `.go` files** (`app.go`, `services/*.go`) in the repo with **no `go.mod`** — a red herring; the live code is `electron/services/*.ts`. Encoder reimplemented in TS via `child_process` + `ffmpeg-static`. |
| Bare Wails+Nuxt4 scaffold (`wails-template-nuxt4`) | Wails v2.10.2 + Nuxt 4 | Bare scaffold, only `Greet`/`GetAppInfo`/`GetSystemInfo`. **`go.mod` identical to the Wails ssanime-gui attempt** (template duplication). Untouched starter. |

Observations that point at the friction:
- All three carried the **generic template identity** (`wails-nuxt4-template`, output
  `wails-nuxt4-app`) — never specialized, a tell that energy went into rescaffolding.
- The Electron branch existed purely to chase richer UI (PrimeVue Queue/Progress) and a release
  pipeline, at the cost of abandoning the Go backend entirely.

The original **Django `automin`** is also a cautionary tale of scope: it is a *full release-group
distribution pipeline* (feeds → qBittorrent download → ffmpeg x265 → screenshots → torrent
creation → multi-site upload → seedbox FTP → Adfly shortlinks), driven by Celery + Redis with
fast/medium/slow queues. The new app **deliberately drops the entire distribution half** to stay
scoped.

---

## 4. What Actually Worked / Worth Carrying Forward

1. **The Go encoder (`encoder.go`) from the prior local Wails attempt** is the single reusable
   asset and is **frontend-agnostic.** Characteristics worth preserving:
   - FFmpeg wrapper running as a **goroutine**, x265 encoding, **context-based cancellation**,
     **progress parsing from stderr** (regexp/bufio over the ffmpeg `exec` stream).
   - `EncodingProfile` struct already models the full automin x265 knob set (CRF, deblock,
     smartblur, deinterlace, resolution, psy_rd, psy_rdoq, aq_strength, hardsubs, multi-resolution,
     me/rd/subme/aq_mode/merange/bframes/b_adapt/limit_sao/frame_threads).
   - `profiles.go` — CRUD persisting to `~/.ssanime-gui/profiles.json`, seeds 3 defaults
     (High Quality / Balanced / Fast).
   - `path_history.go` — recent input/output paths, capped at 50.
   - `logger.go` — leveled logging.

2. **automin's x265 recipe** (proven, tuned) — the baseline encode settings to reproduce:
   - `c:v libx265`, `c:a aac`, `profile:v main`, preset `slow`.
   - x265-params: `me=2 rd=4 subme=7 aq-mode=3 aq-strength=… deblock=… psy-rd=… psy-rdoq=…
     rdoq-level=2 merange=57 bframes=8 b-adapt=2 limit-sao=1 frame-threads=3 no-info=1`.
   - CRF ~24 (per-series configurable, 0–51).
   - Resolution/audio ladder: 1080p→192k, 720p→160k, 480p→96k. Optional `smartblur` (denoise) and
     `yadif` (deinterlace) filters; subtitle burn for hardsubs (MP4) vs softsubs (MKV).

3. **automin's clean separation of concerns** (feeders / encoders / uploaders / handlers /
   watchers) — *"will translate well to Go packages."* The new module split mirrors it:
   `server / store / feeds / download / encode / library / binaries / queue / events`.

4. **The data model** carries over (scoped down): `Series`, `Feed` (url, type, filter rules,
   interval, last_checked), `Item/Episode` (source url/magnet, status, resolution, source+encoded
   paths, sizes), `EncodeProfile`, `Settings`. Status pipeline
   `queued → downloading → downloaded → encoding → encoded → archived (+ error)` mirrors automin's
   `dlfin → enc → fin`.

---

## 5. Gotchas & Learnings

- **"Ship one binary" is the load-bearing constraint.** It silently eliminates Electron and
  selects Go. Surface it first on any future stack debate.
- **The Nuxt-fights-desktop pain was a webview+SSR mismatch, not a Vue problem.** Removing the
  *native webview* (browser-served daemon) dissolves the whole class of bug. Don't blame the UI
  framework for what was an architecture mismatch.
- **Meta-frameworks are the trap, twice over.** Nuxt→Vue and SvelteKit→Svelte are the same hazard:
  in an embedded-SPA-in-a-Go-binary you throw away SSR/server-routes/file-routing anyway, so the
  meta-framework is pure config overhead. Use **plain Svelte 5 + Vite**.
- **"One file" is genuinely in tension with external tools.** ffmpeg and yt-dlp are external
  binaries. The honest resolution: ship a tiny binary that *provisions* them on first run, because
  **yt-dlp must self-update or it rots weekly**. Embedding tools trades a clean update story for a
  literal-single-file bragging right — not worth it here.
- **Stale cross-stack artifacts mislead.** `ssanime-gui-nuxt` had `.go` files with no `go.mod`
  that were dead code. When auditing prior work, confirm what's actually *built* (check for
  `go.mod` / the real main process), not just what files exist.
- **Encoding is CPU-bound** — cap encode concurrency at 1–2 even though downloads can run many in
  parallel. Different worker-pool limits per stage.
- **Scope discipline vs automin.** automin is a *publishing* pipeline; pulling in its upload /
  seedbox / torrent-creation / Adfly half is the fast way to balloon scope. Explicitly out.
- **`anacrolix/torrent` maturity caveat.** Embedding it (vs driving qBittorrent) has *"some
  edge-case maturity gaps vs qBittorrent"* — the reason it sits behind a `Downloader` interface so
  a qBittorrent backend can be swapped in later.

---

## 6. Direct Guidance for the New Daemon-First Svelte Rewrite

Constraints and warnings the future build should heed:

1. **Keep the daemon the core.** Go process = long-running HTTP server on `localhost` +
   system-tray (Open UI · Pause all · Quit). Closing the browser tab must NOT stop downloads/
   encodes. This is the property that ends the restart cycle — do not regress to a
   "window that does work" model.

2. **Frontend = plain Svelte 5 + Vite only.** No SvelteKit. Build to static `dist/`, embed via
   `go:embed`, serve from the Go HTTP server. Tailwind + shadcn-svelte (bits-ui) for UI. Do not
   reintroduce a meta-framework "for DX."

3. **Reuse, don't rewrite, the Go encoder.** Port `encoder.go` (goroutine + context cancel +
   stderr progress parsing) and the `EncodingProfile` struct from the prior local Wails attempt's
   `services/` dir. **Finish the wiring that was incomplete there:** map
   *all* x265 profile fields to the ffmpeg CLI (the old build only used CRF + preset) and improve
   progress to within-file (not per-file count).

4. **Download layer behind a `Downloader` interface.** Backends: embedded `anacrolix/torrent` +
   direct/HLS via managed `yt-dlp`. Interface allows a future qBittorrent backend without churn.

5. **Binaries module provisions ffmpeg + yt-dlp on first run** into an app-data dir; let yt-dlp
   self-update. Keep the shipped binary small. `go:embed` is only the zero-network fallback.

6. **Replace Celery with goroutine worker pools** (no Redis). Separate concurrency caps:
   downloads parallel, encodes 1–2. Status pipeline:
   `queued → downloading → downloaded → encoding → encoded → archived (+ error)`.

7. **SSE for live progress/logs**, REST for everything else. Central `events` pub/sub bus pushes
   to SSE clients.

8. **Stay scoped.** This is download → encode → archive for personal use + RSS auto-fetch.
   **No** tracker uploads, seedbox FTP, torrent creation, URL shortening, or public site (all the
   automin distribution half). Don't let automin's surface area creep back in.

9. **UI pages planned:** Library (series grid + archived files), Queue (live download + encode
   progress), Auto-downloader (feeds + filter rules), Profiles (encode presets), Settings, Logs.

10. **First commit is pending.** At session end the new repo (this project) was
    **not yet a git repo** (left uncommitted per the user's "commit only when asked" rule). The
    next planned step was the `writing-plans` skill to sequence milestones
    (core daemon + SQLite + SPA shell → encode → download → feeds → polish).

---

## Source references

- Main transcript: the development transcript (brainstorming/decision session).
- Subagent A: Django `automin` architecture report.
- Subagent B: audit of the three prior Wails workspace attempts.
- Resulting design: captured in `docs/reference/`.
