# Seanime Architecture — Patterns for ssanime-gui

Deep-dive of the Seanime Go anime media server (a local Seanime checkout), extracting
architecture patterns relevant to **ssanime-gui** (daemon-first Go core, embedded Svelte SPA,
SSE, SQLite, goroutine worker pools, embedded torrent + yt-dlp behind a `Downloader` interface,
durable x265 transcode, auto-provisioned binaries).

Verdict legend: **BORROW** = copy the pattern nearly as-is · **ADAPT** = good shape, change for our
divergence · **SKIP** = Seanime-specific, not applicable.

---

## 1. HTTP server + embedded frontend serving

**Stack:** [Echo v4](https://echo.labstack.com/) (not stdlib/chi) + `go:embed` for the SPA.

- **Embed** (`main.go:8`): the whole built web dir is embedded as a single FS.
  ```go
  //go:embed all:web
  var WebFS embed.FS
  //go:embed internal/icon/logo.png
  var logo []byte
  ```
- **Serve SPA** (`internal/core/echo.go:17-99`): `fs.Sub(webFS, "web")` strips the prefix, then
  `middleware.StaticWithConfig{ Filesystem: http.FS(distFS), HTML5: true, Skipper: ... }`.
  `HTML5: true` is the SPA fallback — any unknown path returns `index.html` so client-side routing
  works. A **Skipper** excludes `/api`, `/events`, `/assets`, etc. from the static handler so those
  fall through to real routes. COOP/COEP headers are set for non-API paths only.
- **Custom JSON serializer** (`echo.go:101-111`): swaps Echo's encoder for `goccy/go-json` (faster).
- **Routing** (`internal/handlers/routes.go`): one `Handler{App *core.App}` struct; all handlers are
  methods on it (`func (h *Handler) HandleX(c echo.Context) error`). Routes registered flat under a
  versioned group: `v1 := e.Group("/api").Group("/v1")` then `v1.GET("/settings", h.HandleGetSettings)`,
  `v1.PATCH(...)`, etc. (`routes.go:122-179`). REST verbs map cleanly to CRUD.
- **Response envelope** (`internal/handlers/response.go`): generic `SeaResponse[R]{ Error, Data }`;
  helpers `h.RespondWithData(c, x)` / `h.RespondWithError(c, err)`. Every endpoint returns the same
  `{ "data": ..., "error": ... }` shape — trivial for the frontend to consume.
- **Handler doc comments** (`auto_downloader.go:15-21`) carry `@summary/@route/@returns` annotations
  that generate OpenAPI + a typed TS client. Worth mirroring if we want a generated Svelte client.
- **Startup** (`internal/core/echo.go:113-141`): server runs in a goroutine; `RunEchoServer` does a
  100 ms sleep then logs the URL. TLS optional with self-signed cert auto-gen.

**Server bootstrap loop** (`internal/server/server.go:66-111`): `startAppLoop` is a `for` loop that
either runs the server or, on a self-update signal, calls `app.Cleanup()` and re-enters in update
mode. The Echo app + routes + cron jobs are wired here.

> **Verdict: BORROW** the `go:embed all:dist` + `HTML5:true` static-with-skipper pattern verbatim —
> it is exactly our "single .exe serves Svelte SPA" requirement. **ADAPT** the framework choice: Echo
> is fine, but our scope (≈15 endpoints + 1 SSE stream) is small enough that stdlib `net/http` +
> `http.ServeMux` (Go 1.22 method-pattern routing) or chi avoids a dependency. Keep the
> `Handler{App}` method pattern and the generic `Response[T]{Data,Error}` envelope regardless.

---

## 2. Events / live updates

**Seanime uses WebSocket** (`gorilla/websocket`), **not SSE.** This is the one place we deliberately
diverge (spec chose SSE for one-way progress/logs).

- **Hub** (`internal/events/websocket.go`): `WSEventManager` holds `Conns []*WSConn` (each = id +
  platform + `*websocket.Conn`), guarded by a `sync.Mutex`. `SendEvent(type, payload)` broadcasts a
  `WSEvent{Type string, Payload interface{}}` as JSON to every conn (`websocket.go:168-200`).
  `SendEventTo(clientId, ...)` is a targeted unicast (`:203`).
- **Event-type registry** (`internal/events/events.go`): a big block of string consts
  (`EventScanProgress`, `AutoDownloaderItemAdded`, `ActiveTorrentCountUpdated`, `InfoToast`,
  `InvalidateQueries`, …). The frontend switches on `event.type`. `InvalidateQueries` is a neat
  trick: the server tells the client to refetch a React-Query/Svelte-Query key rather than pushing
  full state.
- **Inbound client events** (`internal/events/websocket.go:266-339`): a per-channel pub/sub —
  `SubscribeToClientEvents(id)` returns a buffered `chan *WebsocketClientEvent` (buffer 900/100);
  `OnClientEvent` fans out non-blockingly (`select { case ch<-e: default: drop }`) so a slow client
  can't stall the hub. Subscribers stored in a concurrent `result.Map`.
- **Connection handler** (`internal/handlers/websocket.go`): upgrades, auth via `?token=` /
  origin allowlist, registers conn, then a `for { ReadMessage() }` loop. Handles `ping`→`pong` and
  app-level client events. On read error, `RemoveConn(id)`.
- **Push-from-anywhere**: `WSEventManager` is on `App`, and a `GlobalWSEventManager` wrapper
  (`events.go:28-53`) lets deep packages emit without a reference. Producers push from their own
  goroutines, e.g. the active-torrent-count ticker `wsEventManager.SendEvent(ActiveTorrentCountUpdated, count)`
  (`torrent_client/repository.go:96-98`).

> **Verdict: ADAPT.** Keep the **hub shape** (a manager holding clients + a registry of event-type
> string consts + a `Broadcast(type, payload)` method + the `InvalidateQueries` refetch trick), but
> implement the transport as **SSE**: each client is an `http.ResponseWriter` with `http.Flusher`;
> `Broadcast` writes `event: <type>\ndata: <json>\n\n` then `flusher.Flush()`. We have no inbound
> client→server channel needs (commands go over REST POST), so we **SKIP** the entire
> `SubscribeToClientEvents`/`OnClientEvent` inbound half. Keep the non-blocking
> `select{case ch<-e: default:}` drop pattern per SSE client to prevent one stalled browser tab from
> blocking the encode worker. Buffer-per-client + drop-oldest is the key durability lesson.

---

## 3. Database layer

**DB:** SQLite via `glebarez/sqlite` (pure-Go, **cgo-free** — preserves single-binary cross-compile).
**ORM:** GORM with `AutoMigrate`.

- **Open** (`internal/database/db/db.go:28-84`):
  ```go
  gorm.Open(sqlite.Open(path+"?_busy_timeout=30000&_journal_mode=WAL&_synchronous=NORMAL"+
      "&_cache_size=1000&_foreign_keys=on"), &gorm.Config{ Logger: ... })
  ```
  WAL + busy_timeout + foreign_keys are the load-bearing pragmas for a concurrent
  worker app. Connection pool is deliberately tiny: `SetMaxOpenConns(3)`, `SetMaxIdleConns(2)` —
  SQLite is single-writer, so a small pool avoids `SQLITE_BUSY` thrash.
- **Migrations** (`db.go:87-124`): no migration files — `db.AutoMigrate(&models.X{}, &models.Y{}, …)`
  over a flat list of struct pointers, run on every boot. Schema lives in Go structs
  (`internal/database/models/models.go`, ~29 KB, all models in one file) with a shared `BaseModel`
  (id/created/updated). `:memory:` used when `TEST_ENV=true`.
- **Query methods** hang off `*Database` (e.g. `chapter_downloader_queue.go`): thin wrappers like
  `GetNextChapterDownloadQueueItem()`, `UpdateChapterDownloadQueueItemStatus(...)`,
  `DequeueChapterDownloadQueueItem()`. The DB *is* the queue persistence layer.
- **Cleanup manager** (`db.go:126-129`, `cleanup_manager.go`): periodic pruning of stale rows.

> **Verdict: BORROW** almost wholesale — this is our exact stack (SQLite, single binary, worker
> concurrency). Copy the pragma string, the tiny pool, and `AutoMigrate(list...)`. One judgment
> call: GORM is heavy; for our ~5 tables (Series/Feed/Item/EncodeProfile/Settings) plain
> `database/sql` + `modernc.org/sqlite` (also cgo-free) or `sqlc` gives more control over the
> single-writer queue transitions. Either way: **WAL + busy_timeout + MaxOpenConns≈3 is mandatory**.
> Split models into per-domain files rather than one 29 KB `models.go`.

---

## 4. Download / torrent abstraction

Two distinct subsystems — study both, they map to our two `Downloader` backends.

### 4a. External-client abstraction (`internal/torrent_clients/`)
- `torrent_client/repository.go`: a `Repository` wrapping qBittorrent + Transmission clients. **It is
  NOT a Go interface** — it dispatches with `switch r.provider { case Qbittorrent: ...; case
  Transmission: ... }` inside every method (`GetList`, `AddMagnets`, `RemoveTorrents`,
  `PauseTorrents`, `GetFiles`, …). Provider consts: `QbittorrentClient/TransmissionClient/NoneClient`.
- `GetFiles` (`repository.go:451-498`) is a useful pattern: polls the client on a 1 s ticker with a
  2 min `context.WithTimeout`, blocking until files appear — handles the "metadata not ready yet"
  race when a magnet is first added.

### 4b. Embedded engine (`internal/torrentstream/`) — **this is our anacrolix path**
- `client.go`: wraps `anacrolix/torrent`. `NewDefaultClientConfig()`, `cfg.Seed`, `cfg.ListenPort`,
  `cfg.DefaultStorage = storage.NewFileByInfoHash(downloadDir)` → files land in
  `{downloadDir}/{infohash}/` (`client.go:97-120`). Client lifecycle is guarded by a
  `context.WithCancel`; `initializeClient` cancels the prior context before recreating
  (`client.go:84-130`). Uses `samber/mo` `Option` types for nullable client/torrent/file.
- Designed for **one torrent at a time** for streaming; on init it **drops all torrents**. (We want
  *many concurrent* archive downloads — diverge here.)

> **Verdict: ADAPT — and improve on Seanime.** Define the **real Go interface** Seanime lacks:
> ```go
> type Downloader interface {
>     Add(ctx, src DownloadSource) (handle string, err error)
>     Progress(handle string) (Progress, error)   // bytes, %, speed, ETA
>     Cancel(handle string) error
>     OnComplete() <-chan DownloadResult           // file path(s)
> }
> ```
> Two impls: a `TorrentDownloader` (BORROW the `anacrolix` config/storage/`context.WithCancel` setup
> from `torrentstream/client.go`, but allow **N concurrent torrents** and **do not seed/drop** —
> ours is download-to-archive, not stream-and-seed), and a `YtDlpDownloader` (managed binary, exec +
> stderr parse, no Seanime equivalent — new code). The `GetFiles` poll-with-timeout pattern is worth
> borrowing for "wait until torrent metadata resolves." **SKIP** the qB/Transmission `switch`
> dispatch entirely — our interface replaces it; the future qBittorrent backend the spec mentions
> just becomes a third impl of `Downloader`.

---

## 5. Transcoding / ffmpeg

Seanime transcodes **ephemerally for HLS playback** (segments on demand), the opposite of our
**durable single-file x265 archive** encode. The *invocation mechanics* are reusable; the
*architecture* is not.

- **Invocation** (`internal/mediastream/transcoder/stream.go:484-619`):
  `cmd := util.NewCmdCtx(ctx, ffmpegPath, args...)` (a wrapper that sets platform-specific
  `SysProcAttr` to hide the console window on Windows — `internal/util/cmd_win.go`). Then
  `StdoutPipe()`, `StdinPipe()`, `cmd.Stderr = &strings.Builder`, `cmd.Start()`.
- **Progress** is parsed by `bufio.Scanner` over **stdout** — but note Seanime parses *segment
  filenames* (`-f segment` output), not ffmpeg's `-progress` stream, because it tracks HLS segment
  completion (`stream.go:515-581`). For our single-file encode we want the standard `-progress pipe:1`
  / stderr `frame=… time=… speed=…` parse instead (this is what ssanime-gui's existing `encoder.go`
  already does — keep it).
- **Cancellation — two mechanisms, both worth copying** (`stream.go:540-592`):
  1. Graceful: write `"q"` to ffmpeg's **stdin** (`stdin.Write([]byte("q"))`) — clean shutdown,
     flushes output. Used when a segment is already done.
  2. Hard: a goroutine selects on `ctx.Done()` and writes `q`/closes stdin to abort.
- **Wait/exit handling** (`stream.go:594-619`): a goroutine on `cmd.Wait()` inspects
  `*exec.ExitError`; exit code 255 = "we terminated it" (not an error); also sniffs stderr for
  "hwaccel … failed" to detect and warn on HW-accel fallback to CPU.
- **Transcoder struct** (`transcoder/transcoder.go`): holds a `result.Map[path]*FileStream` (one
  ffmpeg pipeline per file), a `clientChan`, a `Tracker` (idle-stream GC), and `Settings{FfmpegPath,
  FfprobePath, HwAccel}`. On `Destroy()` it kills all streams and wipes the temp dir.
- **HW accel** (`transcoder/hwaccel.go`): config table mapping kind (nvenc/qsv/vaapi) → decode/encode
  flags — exactly the "data list over conditionals" shape.

> **Verdict: ADAPT the mechanics, SKIP the architecture.** **BORROW**: `NewCmdCtx`-style console-hiding
> exec wrapper (`util/cmd_win.go` — important on Windows tray apps), the **dual cancellation** (`q` to
> stdin for graceful + `ctx.Done()` goroutine for hard-abort), and the `cmd.Wait()` exit-code-255
> "we-killed-it-isn't-an-error" handling. **SKIP**: the per-segment HLS scanner, `FileStream` map,
> on-demand segment model, and idle-stream tracker — irrelevant to a one-shot archive encode. Our
> `encode` package is a durable job: one input → one `-c:v libx265 -crf 24 -preset slow` run →
> progress via `-progress pipe:1` (`out_time_ms`/`speed`) → SSE → final file. The HW-accel config-table
> shape is worth keeping for an optional fast path.

---

## 6. Binary management / self-update

Two separate concerns; Seanime covers app self-update well but **does NOT auto-provision ffmpeg**.

- **App self-update** (`internal/updater/`): `check.go` queries GitHub releases; `download.go`
  downloads the asset and decompresses (`.zip` via `archive/zip`, `.tar.gz` via
  `archive/tar`+`gzip`). Hardened against zip-slip: every entry path goes through
  `util.ResolveArchiveEntryPath(dest, name)` and symlinks/irregular entries are rejected
  (`download.go:106-112, 199-228`) — **copy this; it's the correct way to extract untrusted
  archives.** `selfupdate.go` swaps the running binary; `validateUpdateURL` allowlists hosts.
- **Update mode** (`server/server.go:66-111`): self-update can't replace a running exe, so the app
  re-execs itself in `-update` mode — `startAppLoop` detects `selfupdater.Started()`, calls
  `app.Cleanup()`, and loops back into update mode.
- **ffmpeg/ffprobe**: Seanime **expects user-supplied paths** stored in settings
  (`models.MediastreamSettings.FfmpegPath`); there is **no download-on-first-run**. This is a *gap*
  vs. our spec — we must build provisioning ourselves.

> **Verdict: BORROW the archive-extraction + GitHub-release-download code** (`updater/download.go`)
> nearly verbatim for our `binaries` package — it's exactly what we need to fetch ffmpeg/yt-dlp ZIPs,
> and the zip-slip hardening is non-negotiable. **ADAPT** it to target a per-tool app-data dir
> (`%LOCALAPPDATA%/ssanime/bin/`) and to handle yt-dlp's self-update (`yt-dlp -U`) rather than our
> shipping it. **SKIP** the whole-app self-replace/re-exec dance (we have one binary + two tools;
> overkill). Our `binaries.Ensure(tool)` = check path → if missing, download release asset →
> verify → extract → cache path in SQLite Settings.

---

## 7. Worker / queue / concurrency

The **manga chapter downloader** (`internal/manga/downloader/`) is the closest analogue to our
`download → encode` pipeline — a DB-backed, single-worker queue. This is the highest-value pattern.

- **Queue** (`downloader/queue.go`): the **SQLite table is the source of truth** for queued work
  (`ChapterDownloadQueueItem` rows with `status: not_started|downloading|errored`). The in-memory
  `Queue` just tracks `current *QueueInfo` and feeds a worker.
- **Worker** (`downloader/chapter_downloader.go:108-120`): one long-lived goroutine:
  ```go
  func (cd *Downloader) Start() {
      go func() {
          for { select { case qi := <-cd.runCh: cd.run(qi) } }
      }()
  }
  ```
  `runCh chan *QueueInfo` is the hand-off; `chapterDownloadedCh` signals completion downstream.
- **Pump loop** (`queue.go:152-210`): `runNext()` guards on `current != nil` (serial — one at a
  time), pulls `GetNextChapterDownloadQueueItem()` from SQLite, flips status to `downloading`, sets
  `current`, and sends to `runCh`. On completion `HasCompleted()` (`queue.go:92-120`) either
  `Dequeue`s (success → delete row) or marks `errored`, then calls `runNext()` again — a
  self-clocking chain.
- **Crash recovery**: `ResetDownloadingChapterDownloadQueueItems()` flips orphaned `downloading` rows
  back to `not_started` on boot (`chapter_downloader_queue.go:126`) — **durable resume after a crash
  mid-job.** `ResetErroredChapterDownloadQueueItems()` enables retry.
- **Cancellation**: `cancelChannels map[DownloadID]chan struct{}` — per-job cancel channels
  (`chapter_downloader.go:46`).
- **Panic isolation**: `defer util.HandlePanicInModuleThen(...)` in every goroutine
  (`queue.go:157`) so one bad job never kills the worker.
- **Concurrency caps elsewhere**: simple ticker + semaphore patterns, e.g. the active-count ticker
  (`torrent_client/repository.go:89-101`) running every 5 s under a `context.WithCancel`.

> **Verdict: BORROW — this is our blueprint.** Two of these queues chained: **download queue → encode
> queue**, each a SQLite-backed `status`-column state machine with a worker goroutine and a `runCh`
> hand-off. Take wholesale: (1) **DB-as-queue** with status transitions
> `queued→downloading→downloaded→encoding→encoded→archived|error` (matches our spec §5 pipeline
> exactly); (2) **boot-time orphan reset** (`downloading→queued`) for crash-durable resume — critical
> for a long encode that may outlive a crash; (3) **per-job cancel channel/context**; (4)
> **`HandlePanicInModule` in every goroutine**. **ADAPT**: Seanime's queue is strictly serial
> (`current != nil` guard); we want a **per-stage concurrency cap** (spec §4 `queue` pkg) — replace
> the single `current` with a worker pool of size N (buffered semaphore channel) per stage, each
> worker running the same pull-from-SQLite loop. Encode stays cap=1 (CPU-bound); download can be 2-3.

---

## 8. Overall app wiring / bootstrap

- **Composition root** (`internal/core/app.go`): one giant `App` struct holding **every** subsystem
  as a field — `Config`, `Database`, `Logger`, `WSEventManager`, `TorrentClientRepository`,
  `PlaybackManager`, `AutoDownloader`, `MetadataProviderRef`, etc. (`app.go:61-90+`). Constructed in
  `NewApp(...)` and `initModulesOnce`/`modules.go` (~25 KB), which instantiates subsystems in
  dependency order and cross-wires them (each gets `app.Logger`, `app.Database`, `app.WSEventManager`).
- **Manual DI**: no DI framework — plain struct fields + constructor options structs
  (`NewXOptions{...}`) everywhere. Subsystems take what they need via their options struct, not the
  whole `App`, keeping them testable.
- **Global escape hatch**: `events.GlobalWSEventManager` wrapper lets leaf packages emit events
  without threading the manager through every call.
- **Boot sequence** (`server/server.go` → `core/echo.go` → `handlers.InitRoutes` →
  `RunEchoServer` → `cron.RunJobs`): create App+config+logger → build Echo + embed FS → register
  routes → start server goroutine → start cron → (Windows) wrap in systray.
- **System tray** (`server/server_windows.go`): `fyne.io/systray` with menu items (Open UI / Open
  dirs / Quit); `cli/browser.OpenURL` opens the SPA; `w32` hides the console window. The tray's
  goroutine *is* the app lifetime — `startAppLoop` runs inside `onReady`. This is **exactly our
  daemon-first tray model** (Open UI · Pause · Quit).

> **Verdict: BORROW the shape, scaled down.** A single `App`/`Core` struct as composition root with
> manual DI (struct fields + `NewXOptions` constructors) is right for our size — no DI framework.
> **BORROW** the systray wiring (`fyne.io/systray` + `cli/browser` + console-hide via `w32`) almost
> verbatim — it delivers the spec's "Open UI · Pause all · Quit" tray and background-daemon lifetime
> for free. **ADAPT** the boot sequence to: build Core (config, SQLite, logger, SSE hub, binaries,
> Downloader, encoder, queues) → register routes → serve embedded SPA → open browser → run tray.
> Consider a lighter global-emitter than Seanime's `GlobalWSEventManager` (pass the SSE hub
> explicitly where practical; reserve the global for deep leaf packages only).

---

## Cross-cutting borrow/skip summary

| Area | Verdict | One-liner |
|---|---|---|
| `go:embed all:dist` + Echo `Static{HTML5:true, Skipper}` | **BORROW** | exact single-exe SPA serving |
| Generic `Response[T]{Data,Error}` + `Handler{App}` methods | **BORROW** | uniform API the SPA consumes |
| WebSocket hub | **ADAPT→SSE** | keep hub+event-const-registry+`InvalidateQueries`; transport = SSE flush; drop inbound half |
| SQLite pragmas (WAL, busy_timeout, FK) + tiny pool + `AutoMigrate(list)` | **BORROW** | our exact DB stack |
| qB/Transmission `switch` dispatch | **SKIP** | replace with a real `Downloader` interface |
| `anacrolix` client config/storage/ctx-cancel | **ADAPT** | allow N concurrent, no seed/drop |
| ffmpeg exec: `NewCmdCtx`, `q`-to-stdin + `ctx.Done()` cancel, exit-255 handling | **BORROW** | mechanics; not the HLS architecture |
| HLS segment scanner / FileStream map / idle tracker | **SKIP** | we do durable single-file encode |
| `updater/download.go` GitHub-fetch + zip-slip-safe extract | **BORROW** | basis of `binaries` provisioning |
| App self-replace/re-exec update mode | **SKIP** | overkill for one binary + 2 tools |
| DB-as-queue + worker goroutine + `runCh` + status state machine | **BORROW** | core download→encode pipeline |
| Boot-time orphan-status reset (crash resume) | **BORROW** | durable resume for long encodes |
| `HandlePanicInModule` in every goroutine | **BORROW** | one bad job never kills the worker |
| Serial `current != nil` queue | **ADAPT** | → per-stage semaphore worker pool (download 2-3, encode 1) |
| `App` composition root + manual DI + `NewXOptions` | **BORROW** | right-sized DI |
| `fyne.io/systray` + `cli/browser` + `w32` console-hide | **BORROW** | delivers our tray/daemon model |

### Highest-value, lowest-risk borrows (do these first)
1. **DB-as-queue worker pipeline** with status state machine + boot-time orphan reset (§7) — the spine of the app.
2. **`go:embed` + HTML5-fallback static serving** (§1) — single-exe SPA, solved.
3. **SQLite pragmas + tiny pool** (§3) — correct concurrency under single-writer SQLite.
4. **systray + browser-open daemon wiring** (§8) — the daemon-first UX, nearly verbatim.
5. **ffmpeg cancel + exit-handling mechanics** (§5) and **zip-slip-safe release extraction** (§6).

### Where we must build net-new (no Seanime equivalent)
- **SSE transport** (Seanime is WS-only) — but reuse the hub *shape*.
- **yt-dlp managed backend** behind `Downloader` — Seanime has nothing here.
- **ffmpeg/yt-dlp auto-provisioning on first run** — Seanime expects user-supplied ffmpeg.
- **A real `Downloader` interface** — Seanime uses a `switch`, not an interface.
- **Multi-concurrent archive downloads** — Seanime's embedded engine is one-torrent-at-a-time.
