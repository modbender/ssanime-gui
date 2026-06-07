# Implementation plan — phased, subagent-driven

Module: `github.com/modbender/ssanime-gui` · License **GPL-3.0** · Go 1.24 · single cgo-free binary.
Each phase has a clear deliverable + verification gate; later phases depend on earlier ones. Source
of truth for design: `docs/reference/{schema-from-automin,db-layer-decision,app-flow,seanime-architecture}.md`.

Dependency order: **0 → 1 → {2,3} → 4 → 5 → {6,7} → 8 → 9**.

## Phase 0 — Foundation (lead-built scaffold)
Module init, directory layout, `go.mod` deps, `LICENSE` (GPL-3.0), `.gitignore`, tooling
(`sqlc.yaml` engine=sqlite, `db/migrations/` for goose, `Taskfile`/`Makefile`), the **DoH resolver**
(port the validated `nyaa-test` dialer → `internal/doh`), config loader, and a `main.go` skeleton that
boots an HTTP server on `localhost:<port>`. **Gate:** `go build ./...` green.

## Phase 1 — Store / DB  (super-backend)
`db/schema.sql` (all ~10 tables, indexes, status enums), goose initial migration, `sqlc` queries +
generated code, `internal/store`: **dual pool** (write `MaxOpenConns(1)` + read pool), WAL/
busy_timeout/foreign_keys/synchronous pragmas, `_txlock=immediate`. Migration runner on boot;
**crash-recovery** reset of orphaned `downloading`/`encoding` rows. Seed: builtin automin profiles
(immutable, inheritance base), embedded `download_clients` row, singleton `settings`.
**Gate:** unit tests for status transitions + crash-recovery; `go test ./internal/store/...`.

## Phase 2 — Events + HTTP server  (super-backend)
`internal/events` SSE hub (pub/sub; adapt Seanime's hub shape, drop the inbound WS half),
`internal/server` (chi or net/http) with the `Response[T]{Data,Error}` envelope + handler pattern,
`//go:embed` the Svelte `dist` (placeholder now) with HTML5 fallback. **Gate:** server starts, SSE
client receives a heartbeat, static fallback serves.

## Phase 3 — Sourcing + metadata  (super-ai for AniList/match; super-backend for providers)
hibike **`AnimeProvider`** interface (MIT, reimplement), native Go providers **SubsPlease** + **Nyaa
(via DoH)** using gofeed + **habari**, **autoselect** best original (trusted group/native res).
`internal/anilist` GraphQL client (search, metadata, episode counts, posters). Feed registry +
**poller** honoring the subscribe/derived-status rules in `app-flow.md`. **Gate:** integration test
hitting a recorded fixture (no live net in CI); manual `go run` smoke vs live nyaa via DoH.

## Phase 4 — Download  (super-backend)
`Downloader` interface; **embedded anacrolix** backend (N concurrent, stop-seed-then-remove);
external **qBittorrent/Transmission** backends; download **queue** worker pool (cap =
`concurrency_download`); progress → events; DB state durable, resumable. **Gate:** download a small
public-domain torrent end-to-end behind the interface.

## Phase 5 — Encode + library  (super-languages: Go/ffmpeg)
Port `encoder.go` → `internal/encode`; **full x265 arg builder** (every profile knob + smartblur/
yadif/scale), profile **inheritance resolution**, **ffprobe** real progress, **multi-resolution
fan-out** → one `encoded_outputs` row per resolution; encode **queue** worker pool (cap =
`concurrency_encode`); **thumbnail** pass → `screenshots`; **Jellyfin path builder** + archive move;
**original cleanup** (delete after all outputs archived, per `cleanup_policy`). **Gate:** encode a
sample file at 2 resolutions, verify paths + sizes + cleanup + DB state.

## Phase 6 — Extension runtime (goja)  (super-languages)
`internal/extension` goja runtime + hibike JS loader; `extension_repos` sync/marketplace; register JS
providers alongside native ones; run Seanime/Hayase JS extensions. **Gate:** load `hayase/nyaa.js`
through goja and return parsed results.

## Phase 7 — Binary provisioning  (super-cloud)
Auto-provision ffmpeg (+ later yt-dlp) into app-data, zip-slip-safe GitHub-release extraction (adapt
Seanime `updater`), self-update. **Gate:** first-run provisions ffmpeg; checksum verified.

## Phase 8 — Frontend (Svelte)  (super-frontend)
Plain Svelte 5 + Vite + Tailwind + shadcn-svelte. Pages: **Library, Series detail (select/bulk
encode), Queue (SSE), Auto-downloader, Profiles, Settings, Logs** — Seanime-shaped. REST + SSE client.
**Gate:** `bun run build` emits `dist/`; embedded build serves it; core flows wired.

## Phase 9 — Tray + packaging  (super-cloud)
`fyne.io/systray` (Open UI · Pause · Quit) + browser-open + console-hide; single-binary build with
embedded `dist`; cross-compile matrix. **Gate:** one `.exe` runs daemon + tray + opens browser.

## Conventions (all phases)
- **Use context7 + current docs for EVERY library.** Before using any dependency (anacrolix/torrent,
  goja, fyne/systray, chi, sqlc, AniList, Svelte 5 runes, shadcn-svelte, Tailwind v4, Vite, etc.),
  resolve it via the context7 MCP (`resolve-library-id` → `query-docs`) and use the **latest
  features/APIs** — do not rely on training-cutoff memory, which may be stale. Prefer modern idioms
  over deprecated ones. Cite the version when an API choice depends on it.
- cgo-free always (`modernc.org/sqlite`); no `mattn/go-sqlite3`.
- Errors wrapped with context; durable state in DB, transient progress over SSE only.
- Registry/data-list over switch; interface seams at `Downloader` and `AnimeProvider`.
- Tests beside code; `go vet ./...` + `gofmt` clean before a phase gate is "done".
