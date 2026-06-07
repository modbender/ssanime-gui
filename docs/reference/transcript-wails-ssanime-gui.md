# Reference: Wails + Nuxt 4 `ssanime-gui` — Development History (Mined Transcript)

Source transcript: `C:\Users\Neo\.claude\projects\D--Projects-wails-ssanime-gui\3e1e598e-6fba-461e-afa1-5672938ae177.jsonl`
Original workspace: `D:\Projects\wails\ssanime-gui` (now abandoned).
Successor: daemon-first Go + Svelte rewrite at `D:\Projects\gui\ssanime-gui`.

This document captures what the old Wails build contained, what was decided, what hurt, and what to carry forward into the rewrite. It is reconstructed from a relatively short session (≈189 events): the bulk of the *code* already existed before the transcript; the session itself was (1) a `/init` codebase analysis producing a `CLAUDE.md`, and (2) a research conversation comparing stacks and probing Wails build constraints. The abandonment decision is implied by the research thread, not narrated as a dramatic blow-up.

---

## 1. Timeline / phases

The codebase pre-dated this session. Within the transcript:

1. **Phase A — `/init` codebase audit.** Claude read `app.go`, `main.go`, all of `services/` (`encoder.go`, `profiles.go`, `path_history.go`, `logger.go`), `README.md`, `wails.json`, `go.mod`, `frontend/package.json`, `frontend/nuxt.config.ts`, the Vue files, and the generated `wailsjs` bindings, then wrote a `CLAUDE.md`. This is the most complete snapshot of the project's actual state.
2. **Phase B — competitive stack research.** User asked what tech stack `5rahim/seanime` (a mature self-hosted anime media server) uses. Findings below. This planted the seed for the architecture pivot: seanime runs **Go as a standalone server + React SPA, wrapped in Electron**, decoupling backend from UI — the opposite of Wails' embedded-webview single-binary model.
3. **Phase C — Wails build-constraint probe.** User asked "can this app be built from WSL? I guess not because of UI right?" The answer surfaced the real Wails friction (CGO + native webview, **no cross-compilation**), which is the strongest documented rationale in the transcript for moving off Wails.

The transcript ends there. The actual "abandon Wails, start daemon-first Svelte rewrite" decision happened after (or outside) this transcript, but its technical justification is fully present here.

### Project genesis (important context)
The project was **bootstrapped from a "Wails + Nuxt 4 Template"**, not built clean. Consequences that bled through everywhere:
- Go module is still named `wails-nuxt4-template`; all imports read `wails-nuxt4-template/services`. Load-bearing — never casually renamed.
- `wails.json` `name` = `wails-nuxt4-template`, `outputfilename` = `wails-nuxt4-app`, author = "Your Name". Template cruft never cleaned.
- `frontend/app/pages/index.vue` was still the template demo page (`🚀 Wails + Nuxt 4`, greet-the-user box) — the real encoding UI lived only in `app/components/encoding/Main.vue` + `Profiles.vue`.

---

## 2. Technical stack (as-built, Wails version)

### Backend — Go + Wails v2
- `github.com/wailsapp/wails/v2 v2.10.2`, `go 1.22` / toolchain `go1.24.4`.
- `main.go` embeds `//go:embed all:frontend/dist`, runs `wails.Run` with window 1024×768, background `RGBA{27,38,54}`, lifecycle hooks `startup`/`domReady`/`beforeClose`/`shutdown`, `Bind: []interface{}{app}`.
- `app.go` is the **auto-bound API surface**: every exported method on `*App` becomes a frontend-callable function after binding regeneration. `App` holds `logger`, `encoder`, `profilesService`, `pathHistory`, all constructed once in `NewApp()`. It is a thin façade delegating to `services/`.
- Bound API (from generated `App.d.ts`): `StartEncoding(files[], outputDir, profile)`, `StopEncoding`, `IsEncodingRunning`, `GetEncodingProgress`, `GetEncodingProfiles`/`GetEncodingProfile`/`SetEncodingProfile`/`DeleteEncodingProfile`, `SelectFiles`, `SelectOutputDirectory`, `GetInputPathHistory`/`GetOutputPathHistory`/`GetMostRecentOutputPath`, `GetAppInfo`, `GetSystemInfo`, `Greet`.

### Frontend — Nuxt 4 (Vue 3)
- Nuxt `^4.0.3`, `srcDir: 'app/'`, `ssr: false`, `nitro.preset: 'static'` so `pnpm build` emits a static `dist/` for Go to embed. `app.baseURL: './'`, `buildAssetsDir: '/'`.
- Nuxt UI `^2.18.7` (`U*` components), Pinia `^3.0.3`, `@nuxtjs/color-mode`, `@nuxt/icon` (heroicons/lucide/tabler), Tailwind + SCSS.
- **Package manager: pnpm** (committed `pnpm-lock.yaml`, `packageManager` field, `wails.json` invokes `pnpm install`/`pnpm run build`). This overrode the usual bun default.
- Vue→Go calls go through generated `frontend/wailsjs/go/main/App` (functions) and `frontend/wailsjs/go/models.ts` (types) — generated, never hand-edited, regenerated on dev/build. Example import path from a component: `import { SelectFiles, GetEncodingProfiles } from '../../../wailsjs/go/main/App'` (note the brittle relative depth).
- `frontend/app/types/wails.ts` declared a `window.wails`/`window.go.main.App` global; `app.vue` guarded with `if (window.wails)`. UI-only browser dev (`pnpm dev`) leaves these unresolved by design.

### Commands (no real test suite existed)
- Full app + hot reload: `wails dev` (boots Go + Nuxt dev server on `localhost:3000` inside the native window; regenerates bindings on backend change).
- UI-only: `cd frontend && pnpm dev`.
- Build: `wails build` → `build/bin/`; `-debug` for debug; `-platform ...` flag exists but see cross-compile caveat below.
- "Tests" = `pnpm test` = `lint` + `format:check` + `type-check`. **No Go tests, no runtime tests.**

---

## 3. The encoder — what worked and is worth porting

`services/encoder.go` is the most valuable carry-forward asset. Design:

- **Concurrency model:** single job at a time. `Encoder` guards `isEncoding`, `progress`, `cmd`, `ctx`/`cancel` behind a `sync.RWMutex`. `EncodeFiles` rejects re-entry (`"encoding is already in progress"`), then spawns `go e.performEncoding(...)`.
- **Process control:** `exec.CommandContext(e.ctx, ffmpegPath, args...)`. `Stop()` calls `e.cancel()` then `e.cmd.Process.Kill()`. On cancel, `cmd.Wait()` error is reclassified to `"encoding cancelled"` by checking `e.ctx.Err()`.
- **FFmpeg discovery (`initializeFFmpeg`):** `exec.LookPath("ffmpeg")` first, then Windows fallbacks: `C:\ffmpeg\bin\ffmpeg.exe`, `C:\Program Files\ffmpeg\bin\ffmpeg.exe`, `C:\Program Files (x86)\ffmpeg\bin\ffmpeg.exe`. Defaults to bare `"ffmpeg"` if none found.
- **Output naming:** `<basename>_encoded.<format>` joined into `outputDir`.
- **Per-file loop:** checks `ctx.Err()` between files; on a single-file failure it logs and `continue`s (one bad file doesn't abort the batch).

### Encoder weaknesses to FIX in the rewrite (documented gotchas)
- **`buildFFmpegArgs` ignores almost the entire profile.** Despite ~20 x265 fields on the struct, it emits only:
  ```
  -i <in> -c:v libx265 -crf <CRF> -preset medium -c:a copy -y <out>
  ```
  `PsyRD`, `PsyRDOQ`, `AQMode`, `AQStrength`, `BFrames`, `BAdapt`, `ME`, `RD`, `SubME`, `MERange`, `LimitSAO`, `FrameThreads`, `Deblock`, `SmartBlur`, `Deinterlace`, `Resolution`, `MultiResolution`/`OutputResolutions`, `HardSubs` — **none are wired to FFmpeg.** They exist as data only. The rewrite must actually assemble `-x265-params` (e.g. `psy-rd=...:psy-rdoq=...:aq-mode=...:aq-strength=...:bframes=...:b-adapt=...:me=...:rd=...:subme=...:merange=...:limit-sao=...:frame-threads=...:deblock=...`), `-vf` scale/deinterlace/smartblur chains, subtitle burn-in for `HardSubs`, and the multi-resolution fan-out.
- **Progress is coarse and fake.** `parseProgress` runs two regexes over stderr:
  - `time=(\d+):(\d+):(\d+\.\d+)` — matched but **discarded**; it sets `progress.Speed = "Processing..."` and computes nothing. The comment literally says "you'd need to know the total duration."
  - `speed=\s*(\d+\.?\d*)x` — captured into `progress.Speed`.
  The reported **percent is per-file-count** (`float64(i)/float64(N)*100`), not within-file. The rewrite should probe duration (`ffprobe`) and compute real percent from `time=` / `out_time_ms`, ideally via `-progress pipe:1` instead of scraping stderr.

---

## 4. Profile model

`EncodingProfile` struct (JSON-tagged) — port the *shape*, it's solid:

```
CRF int; Deblock string; SmartBlur bool; Deinterlace bool; Resolution int;
PsyRD int; PsyRDOQ int; AQStrength int; HardSubs bool;
MultiResolution bool; OutputResolutions []int;
ME int; RD int; SubME int; AQMode int; MERange int; BFrames int; BAdapt int;
LimitSAO bool; FrameThreads int; Format string;
```

`services/profiles.go` does JSON-file CRUD with three seeded defaults:

| Profile | CRF | Res | ME | RD | SubME | AQMode | BFrames | BAdapt | LimitSAO |
|---|---|---|---|---|---|---|---|---|---|
| High Quality | 18 | 1080 | 3 | 4 | 3 | 2 | 4 | 2 | false |
| Balanced | 23 | 1080 | 2 | 3 | 2 | 2 | 3 | 2 | false |
| Fast | 28 | 720 | 1 | 2 | 1 | 1 | 2 | 1 | true |

All three share `Deblock "1:1:1"`, `PsyRD/PsyRDOQ/AQStrength = 1`, `MERange 16`, `FrameThreads 0`, `Format "mp4"`, smartblur/deinterlace/hardsubs off.

**Smell to fix:** defaults are duplicated verbatim in *both* `encoder.go` (`initializeDefaultProfiles`) and `profiles.go` (`initializeDefaultProfiles`). Two sources of truth. The encoder also keeps its own copy of the profile map (`encoder.SetProfiles(profilesService.GetProfiles())` at startup). The rewrite should have one owner of profile data.

`profiles.go` API: `GetProfiles`, `GetProfile(name) (p, ok)`, `SetProfile`, `DeleteProfile`, `GetProfileNames`. Saves via `json.MarshalIndent` to disk.

---

## 5. Persistence & paths

- Config dir: **`~/.ssanime-gui/`** (`os.UserHomeDir()` + `MkdirAll(0755)`), holding `profiles.json` and `path_history.json`. Created on first run.
- `path_history.go`: `PathHistory{Path, Timestamp}`, separate input/output lists, **capped at 50** (`maxHistorySize`), most-recent-first, JSON-persisted (`HistoryData{InputPaths, OutputPaths}`).
- `logger.go`: trivial leveled logger (`Debug/Info/Warn/Error` + `Infof`) to stdout via `log.New(os.Stdout, "", 0)`, timestamp `2006-01-02 15:04:05`. Replace with structured logging (zerolog/slog) in the daemon.

---

## 6. Pain points, friction, and the abandonment rationale

The transcript does not contain an emotional blow-up; the drivers are architectural and tooling-shaped:

1. **Wails does NOT cross-compile — the decisive constraint.** Verified against Wails docs via context7 (the docs site `wails.io` **403s WebFetch**, so context7 was used). Wails v2 relies on **CGO + each OS's native webview**:
   - Windows → WebView2 (`go-webview2`)
   - Linux → WebKit2GTK + GTK3 (`libgtk-3-dev`, `libwebkit2gtk-4.0/4.1-dev`)
   - macOS → WebKit
   Wails' own "multi-platform build" guidance **doesn't cross-compile** — it runs a CI matrix (`ubuntu-latest`/`windows-latest`/`macos-latest`) building each target on its own native runner. So you cannot produce a Windows `.exe` from WSL/Linux; every target needs its own native toolchain + webview libs. (Assistant's verbatim framing: "**The UI isn't the blocker — cross-compilation is.**")
2. **Template cruft never resolved** — mismatched module name (`wails-nuxt4-template`) vs product, leftover demo page, placeholder `wails.json` author/product fields. The project never fully shed its scaffolding.
3. **Brittle generated-bindings coupling.** Frontend reached into Go via deep relative imports (`../../../wailsjs/go/main/App`) and a generated `models.ts`; bindings must be regenerated on every backend signature change. UI-only dev leaves all backend calls unresolved.
4. **Half-wired feature surface.** The profile struct promised rich x265 control the encoder never delivered (see §3). The UI (`Main.vue` 693 lines, `Profiles.vue` 558 lines) exposed quality/resolution/format selectors over a backend that only honored CRF.
5. **Sandbox/tooling friction in the session itself** (minor, but recurring): `curl` blocked (`Exit code 35 FAILED: curl`); `rg` not on PATH (rtk fallback warning); a bash `grep` with embedded Windows `"` paths failed with `unexpected EOF while looking for matching "`; `wails.io` 403'd WebFetch forcing context7.

**The pivot rationale (assistant's own summary, Phase B):** seanime "took a **different desktop path than yours**... runs Go as a **standalone server** with a separate React SPA, then wraps it in **Electron**... heavier (ships Chromium) but **decouples the server from the UI so the same backend serves browser, desktop, and mobile clients.**" That decoupling is exactly what the daemon-first rewrite adopts (minus Electron's weight).

---

## 7. seanime (5rahim/seanime) reference findings — the competitive blueprint

Worth heeding since the new project is in the same domain. Backend (Go 54%): **Echo v4** HTTP framework, **GORM + `glebarez/sqlite`** (pure-Go SQLite, **no CGO**), **gorilla/websocket** for realtime push, **gqlgenc** (generated AniList GraphQL client), **anacrolix/torrent** (embedded torrent client), **dop251/goja** (in-process JS runtime for user extensions/plugins), **chromedp** (headless scraping), `imroc/req` (HTTP), `gofeed` (RSS torrent feeds), **viper** (config), **zerolog** (logging). Core logic in `internal/`, generators in `codegen/`.

Web (React 43%): **React 19 + React Compiler**, built by **Rsbuild/Rspack** (not Next/Vite), **TanStack Router/Query/Table**, **Jotai** (heavy: immer/optics/family/scope), Tailwind 3 + CVA + **Radix UI**, React Hook Form + **Zod**, Motion, **HLS.js**, CodeMirror, Sonner. In `seanime-web/`.

Desktop: **Electron** ("Seanime Denshi", in `seanime-denshi/`) with a custom player (SSA/ASS subtitle rendering, Anime4K upscaling).

**One-line architecture:** a single Go server (Echo + SQLite + websockets, embedded torrent + JS-plugin runtimes) serving a React SPA, optionally Electron-wrapped for desktop.

Takeaways for the rewrite:
- **Pure-Go SQLite (`glebarez/sqlite`) avoids CGO** — the exact thing that made Wails cross-compilation impossible. A CGO-free daemon cross-compiles freely.
- **Daemon + websocket push** is the proven pattern for live encode progress to a browser/desktop UI.
- A standalone HTTP/WS server backend serves browser, desktop, and mobile from one binary.

---

## 8. Guidance for the daemon-first Go + Svelte rewrite

**Port (with fixes):**
- `encoder.go`'s process model: `exec.CommandContext` + `sync.RWMutex` + context-cancel + `Process.Kill()` + per-file `continue`-on-error loop. Solid skeleton.
- `initializeFFmpeg` PATH-then-Windows-fallback discovery (extend with bundled-binary / config-override path).
- `EncodingProfile` struct shape and the three default presets (HQ/Balanced/Fast) and their values.
- JSON-file persistence pattern and the `~/.ssanime-gui/` config location (or migrate to a daemon config dir / SQLite).
- Path-history (recent input/output, capped) is a nice UX touch worth keeping.

**Discard / rebuild:**
- All of Wails (`main.go` embed, `app.go` façade binding, `wailsjs/` generated glue). Replace with an HTTP/WebSocket API (Echo or stdlib + gorilla/websocket) — the daemon model.
- Nuxt/Vue frontend → Svelte. Drop the deep relative `wailsjs` imports; talk to the daemon over HTTP/WS + a typed client.
- The template module name `wails-nuxt4-template`, demo `index.vue`, placeholder `wails.json`.
- The stdout `logger.go` → structured logging (zerolog like seanime, or slog).
- Duplicated default-profile definitions → single source of truth.

**Must-fix functional gaps:**
- **Actually wire profile fields into FFmpeg args** (`-x265-params`, `-vf` filter chains, subtitle burn-in, multi-resolution fan-out). This was the biggest unfinished promise.
- **Real progress:** probe duration with `ffprobe`, use `-progress pipe:1` (or parse `time=`/`out_time_ms`) for true within-file percent, push over websocket. Don't ship the per-file-count fake percent.

**Architecture constraints to honor:**
- **Keep the daemon CGO-free** so it cross-compiles to all platforms from one host (the lesson that killed Wails). Pure-Go SQLite if a DB is needed.
- Decouple backend from UI (daemon serves an API; Svelte SPA is a client) — enables browser + desktop + future mobile from one backend.
- Windows specifics still apply: ffmpeg `.exe` path discovery, `filepath.Join`, `os.UserHomeDir()`.

---

## 9. Concrete artifacts / quotes index

- User, Phase B: *"can find out what tech stack https://github.com/5rahim/seanime is using, you can clone it if required"*
- User, Phase C: *"can this app be built from wsl? I guess not because of UI right?"*
- Assistant verdict: *"The UI isn't the blocker — cross-compilation is."* / *"Wails v2 doesn't cross-compile. It relies on CGO + each OS's native webview library."*
- FFmpeg args actually emitted: `-i <in> -c:v libx265 -crf <CRF> -preset medium -c:a copy -y <out>`
- Progress regexes: `time=(\d+):(\d+):(\d+\.\d+)` (discarded), `speed=\s*(\d+\.?\d*)x` (kept).
- Config home: `~/.ssanime-gui/{profiles.json,path_history.json}`; history cap 50.
- Tooling friction observed: `curl` blocked (exit 35), `rg` not on PATH, bash+Windows-quote `unexpected EOF`, `wails.io` 403 on WebFetch (context7 used instead).
