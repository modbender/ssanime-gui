# Discovery-first home with automatic + manually-overridable tracking

**Status:** approved (brainstormed 2026-06-11). Source of truth for the implementing
super-backend / super-frontend agents.

## Goal

Replace ssanime-gui's library-add-centric home (the "Add your first series" empty
state) with a **Hayase-style discovery-first home**, and make tracking a
**side-effect of activity**: starting a download tracks the series and the existing
pipeline takes over fully automatically. Layered on top, the user can **manually
override** the parts automation can never infer (Pause/On-hold, Drop).

Reference: `docs/reference/HayaseHome1.png`..`HayaseHome3.png` — a full-bleed hero
+ stacked horizontal carousels (Continue Watching, Popular This Season, Trending
Now, All-Time Popular, genre rows), all from AniList, no manual library step.

ssanime is a **download → encode → archive** tool (not a player), so "watch" maps
to "download/encode," and "Continue Watching" maps to "Currently downloading."

## Approved product decisions

1. **Discovery-first home**, always full (never the old empty state). Hero + rows in
   this order: **Currently downloading** (active tracked series; hidden when empty) →
   **Trending Now** → **Popular This Season** → **All-Time Popular** → **genre rows**
   (Action, Romance — a small fixed set for v1).
2. **Download = track (fully automatic).** The ONLY way a series becomes tracked is
   clicking **"Download & track"** (from a discovery card or the series page). That
   auto-creates the series (subscribed) **+ its feed/subscription** and hands off to
   the existing poller → download → encode → archive pipeline. No "add to library,"
   no separate watchlist, no manual feed setup.
3. **Status is automatic by default, manually overridable** (see state machine).
4. **AniList rate limits (~30 req/min)** are designed around with a **server-side
   discovery cache** (hourly refresh; zero AniList calls per page-load; serve stale
   on 429).
5. The old **Library page becomes the auto-populated "Downloads"** grid, grouped by
   status.

## Status state machine (the core of this design)

Automation is **gated on status** — background work runs **only** for Active series,
so a manual override can never be overwritten by a background tick.

| Status | Type | Background automation (feed poll → auto download→encode→archive) | Series-page on-demand source check |
|---|---|---|---|
| **Active** | automatic (derived) | ON — polls source, auto-grabs new episodes through the whole pipeline | live |
| **Completed** | automatic (derived) | idle — series finished airing AND all episodes archived | live (surfaces continuations) |
| **Paused** | manual | OFF | still runs a one-time check, lists available episodes |
| **Dropped** | manual | OFF (separate bucket; same behavior as Paused) | still runs a one-time check, lists available episodes |
| **Error** | automatic | per existing retry policy | shows the failing episode |

### Transitions

- Discovery → **Download & track** → **Active** (create series + feed + subscribe).
- Active → all episodes archived **and** series finished airing → **Completed** (derived, automatic).
- Active → **Pause** / **Drop** (manual) → Paused / Dropped.
- Paused/Dropped → **Resume** (manual) → **Active**.
- Paused/Dropped → **open series page** → on-demand source check runs and lists new
  available episodes, but the **status is unchanged**.
- Paused/Dropped → **manually download an episode** → **re-engage → Active**
  (downloading an episode means "I'm back on this"; background auto-download resumes
  for future episodes too).

### Rules

- **No status change ever deletes files.** Paused/Dropped keep everything on disk.
- Pause/Drop make the feed **dormant** (no background poll/download); in-flight jobs
  finish, then quiet.
- Resume, and manual-download-while-paused, both return the series to fully automatic.

## Data model

Additive, minimal. The codebase already derives status live (`internal/server/series.go`
`derivedStatus`), so we do NOT add a stored status column; we add one **manual
override layer**:

- **New migration `db/migrations/00004_series_user_status.sql`** (goose; mirror the
  `00003` convention with `-- +goose Up/Down` + `StatementBegin/End`): add
  `user_status TEXT` to `series`, nullable, default `NULL`. Allowed values: `NULL`
  (automatic), `'paused'`, `'dropped'`.
- Regenerate sqlc; add queries `SetSeriesUserStatus(id, user_status)` and ensure the
  series read queries (`ListSeriesWithProgress`, `GetSeries`) select `user_status`.
- **Semantics:**
  - `user_status IS NULL` → fully automatic; `derivedStatus` governs the displayed
    status (Active/Completed/Error) and background automation runs.
  - `user_status = 'paused' | 'dropped'` → displayed status is that manual value;
    background automation is **skipped** for this series.

## Backend design (super-backend, opus)

### AniList discovery feed queries — `internal/anilist`
Reuse the existing client (`Client.fetch`, the 429-aware core), `mediaFields`
selection set, `decodeMediaList`, and `mapMedia`/`safeImageURL`. Add list queries
over one shape: `Page(page,perPage){ media(type:ANIME, sort:[...], season?, seasonYear?,
genre_in?, isAdult:false){ <mediaFields> } }`.
- Trending → `sort: TRENDING_DESC`
- All-time popular → `sort: POPULARITY_DESC`
- Popular this season → `sort: POPULARITY_DESC` + current `season`/`seasonYear`
- Genre row → `genre_in:[genre]`, `sort: POPULARITY_DESC`
- `perPage ≈ 24`, `isAdult:false`.
Add one method, e.g. `func (c *Client) ListByFeed(ctx, spec FeedSpec) ([]Media, error)`.

### Discovery cache service — new package `internal/discovery`
Model exactly on `internal/metadata/refresher.go` (Start/Stop/loop/firstPassDelay/
ticker/top-level recover).
- Dependency interface (for tests): `type AniList interface { ListByFeed(ctx, FeedSpec) ([]anilist.Media, error) }`.
- State: `map[FeedKey][]anilist.Media` + per-key `fetchedAt`, guarded by `sync.RWMutex`.
- `feedSpecs []FeedSpec` is a single static list (one place to add/remove a row):
  `trending`, `seasonal`, `popular_all_time`, `genre:Action`, `genre:Romance`.
- Cadence: `defaultInterval = 1h`; `firstPassDelay ≈ 5s` after boot so rows populate
  fast. Sequential fetch (~5–7 requests/refresh) with ~250ms spacing → well under 30/min.
- **Degrade on 429/error:** keep the previous cached slice for that key, leave
  `fetchedAt` unchanged, retry next tick (serve-stale, same as `RefreshDue`). Cold
  boot before first success → empty slice → frontend hides/skeletons that row.
- **Readers never trigger a live fetch** — only the loop fetches. Zero AniList calls
  per page-load.
- Read API: `Snapshot() map[FeedKey][]anilist.Media`, `Feed(key) ([]Media, time.Time)`.
- Wire in `cmd/ssanime/main.go startDaemon`: construct after `anilistClient`,
  `Start()`, register `Stop` in cleanups, pass into `server.Config`.

### REST endpoints — `internal/server`
Register in the `r.Route("/api", …)` block. All use the existing `Response[T]{data,error}`
envelope.
- `GET /api/discovery` → all rows in one payload (reads `discovery.Snapshot()`, maps
  `anilist.Media` → `DiscoveryItem`). 200 with empty rows when cold (never an error).
- `GET /api/tracked` → `{ in_progress, completed }` (+ `paused`, `dropped`). Reuse
  `ListSeriesWithProgress` + `derivedStatus`; honor `user_status` for the Paused/Dropped
  buckets; union with `ListEpisodesByStatus("downloading"/"encoding")` series ids so an
  actively-downloading series floats to the top of in-progress.
- `POST /api/track` → create series + feed + subscribe (see below).
- `POST /api/series/{id}/pause`, `/drop`, `/resume` → set/clear `user_status`
  (`pause`→'paused', `drop`→'dropped', `resume`→NULL). On pause/drop also ensure the
  feed is left dormant (the poller gate handles "don't poll," so this can be just the
  column write; disabling the feed row is optional but cleaner).
- `GET /api/series/{id}/available` → on-demand source check: run the source provider
  search for this series NOW (independent of status) and return episodes available at
  the source that are not yet downloaded, for the per-episode "download" UI. Works for
  Paused/Dropped series too.

### "Download & track" — `handleTrackSeries` in `internal/server/series.go`
The single tracked-series creation path. Composes three existing operations:
1. **Create series** — reuse `handleCreateSeries`'s body: dedupe via
   `GetSeriesByAnilistID` (idempotent — if it exists, proceed to ensure feed +
   subscription), fetch metadata via `GetMedia`, `applyMediaToCreate`, but set
   `Subscribed: 1` and stamp `MetadataRefreshedAt`. If AniList is unreachable, still
   create with available data (mirror `handleRefreshSeries` tolerance; the metadata
   refresher fills in later).
2. **Auto-create the feed** — the piece missing today (`handleCreateSeries` creates
   NO feed). Build `store.CreateFeedParams` like `handleCreateFeed`: `SeriesID`, a
   default provider `Site` the poller can resolve (`poller.providerFor` →
   subsplease/nyaa — pick the project default), `IntervalSeconds = 3600`, `Enabled = 1`,
   quality default. A structured URL isn't required — the poller drives `SmartSearch`
   from series metadata.
3. **Hand off** — nothing to call. The running `poller.PollDue` picks up the due,
   enabled feed and enqueues `status:"queued"` episodes; the download/encode queues
   take over.
Response: the created `SeriesProgress` + `series_id` + `feed_id`. Re-tracking an
existing series sets `subscribed=1`, clears `user_status` (→ Active), and creates the
feed if missing.

### Manual-download re-engage
When `/series/{id}/available` → user downloads a specific episode (enqueue endpoint),
that handler must also **clear `user_status` (→ NULL)** so the series re-engages to
Active. (Wherever the per-episode manual enqueue lives — add the `user_status` clear there.)

### Automation gating — `internal/poller` + `internal/encode`/`internal/download`
The background poller must **skip series with `user_status` set**. Cleanest: the
feed-due query / poll loop joins series and filters `user_status IS NULL` (or the
feed is disabled on pause/drop). Verify in-flight encode/download jobs are allowed to
finish (we only stop *new* background work). Add/adjust the store query
(`ListFeedsDueForPoll`) or the poller's per-feed guard accordingly.

### DTOs — `internal/server/dto.go`
Add `DiscoveryItem`, `DiscoveryRow`, `DiscoveryResponse`, `TrackedResponse`,
`TrackRequest`, `TrackResponse`, `AvailableEpisode`/`AvailableResponse`. `SeriesProgress`
must carry `user_status` (or a unified `status` that already reflects it) so the
frontend can render the right badge and bucket.

## Frontend design (super-frontend, opus)

### New `frontend/src/pages/Home.svelte` (discovery-first)
Top → bottom:
- **Hero** — top item(s) of the Trending feed (rotating). Primary CTA **"Download &
  track"** (calls `api.trackSeries`). Reuse `Hero.svelte` via a discovery-item adapter.
- **Row: Currently downloading** — `api.getTracked().in_progress`, rendered with
  `PosterCard` (real `SeriesProgress`, status pill, live SSE progress). **Hidden when
  empty** — the home stays full via the discovery rows below (this REPLACES the old
  global empty state).
- **Rows: Trending / Popular This Season / All-Time Popular / Genre(Action, Romance)**
  — `api.getDiscovery()`, rendered with a **discovery variant** of `PosterCard` (cover
  + title + hover "Download & track").
- Use the existing generic `Carousel.svelte` for each row. **Skeletons** while data is
  in flight (cold cache first paint).

### "Download & track" flow
- Discovery card / Hero CTA → `api.trackSeries({ anilist_id })`.
- Optimistic: show "Tracking…", on success insert the returned `SeriesProgress` at the
  head of "Currently downloading" and flip the card to a tracked state. 409/idempotent
  = treat as success ("Already tracking").
- Live progress via existing `sse.svelte.ts` (`download.progress`/`encode.progress`/
  `episode.status`) — no SSE changes needed.

### Series page — unified (`frontend/src/pages/SeriesDetail.svelte`)
Works in two modes off one page:
- **Untracked** (opened from a discovery card with only an anilist id): AniList preview
  + "Download & track."
- **Tracked**: pipeline status, per-episode rows, live progress.
Always offers an **on-demand "available episodes" list** (`api.getAvailable(id)`) of
source-available-but-not-downloaded episodes, each with a **Download** button — the
entry point for "grab one episode while paused" (which re-engages → Active).
Controls: **Pause · Drop · Resume** (calls the matching endpoints).

### Library → "Downloads" (`frontend/src/pages/Library.svelte`)
- Rename to **Downloads** (file may be renamed `Downloads.svelte`). Update `App.svelte`
  routing (`/` → `<Home />`; move the grid to `/downloads`) and `Sidebar.svelte` nav.
- Remove the global empty state and the Add modal (tracking happens from discovery).
- Auto-populated grid **grouped by status**: Active / Completed / Paused / Dropped
  sections. Pause/Drop/Resume per card.

### `frontend/src/lib/api.ts`
Add types + methods (see contract). `sse.svelte.ts` — no change.

## API contract (FROZEN — build backend & frontend in parallel against this)

```ts
// GET /api/discovery
interface DiscoveryItem {
  anilist_id: number; romaji_title: string; english_title: string;
  format: string; status: string; episode_count: number | null;
  cover_image: string; banner_image: string; cover_color: string;
  season: string; season_year: number | null; is_adult: boolean;
}
interface DiscoveryRow { key: string; title: string; items: DiscoveryItem[] }
interface DiscoveryResponse { rows: DiscoveryRow[] }   // hero = rows.find(key==='trending').items[0..n]

// GET /api/tracked
interface TrackedResponse {
  in_progress: SeriesProgress[];   // Active (incl. actively downloading, floated up)
  completed:   SeriesProgress[];
  paused:      SeriesProgress[];
  dropped:     SeriesProgress[];
}

// POST /api/track  { anilist_id }
interface TrackResponse { series: SeriesProgress; series_id: number; feed_id: number }
// 201 create; 200 idempotent if already tracked (returns existing).

// POST /api/series/{id}/pause | /drop | /resume  -> { series: SeriesProgress }

// GET /api/series/{id}/available
interface AvailableEpisode { number: number; title: string; source_url: string; size: number | null; resolution: string }
interface AvailableResponse { episodes: AvailableEpisode[] }
```
- `cover_image`/`banner_image` are `""` when not on the CSP image allowlist
  (`s4.anilist.co`/`img.anili.st`) → frontend shows a placeholder. Empty `items` ⇒
  frontend hides that row. `SeriesProgress` keeps its existing shape (`dto.go`) plus a
  status that reflects `user_status`.

## Critical files (verified by the planning pass)

- `internal/anilist/query.go`, `internal/anilist/anilist.go` — add feed queries + `ListByFeed`.
- `internal/metadata/refresher.go` — lifecycle template for `internal/discovery`.
- `internal/server/series.go` — `handleCreateSeries`, `derivedStatus`, add `handleTrackSeries` + pause/drop/resume/available.
- `internal/server/server.go` — Handler/Config + route registration.
- `internal/server/dto.go` — new DTOs.
- `internal/store/series.sql.go`, `feeds.sql.go`, `db/migrations/` — `user_status` column + queries; feed create.
- `internal/poller/poller.go` — gate background work on `user_status IS NULL`.
- `cmd/ssanime/main.go` — wire the discovery service.
- `frontend/src/App.svelte`, `pages/Home.svelte` (new), `pages/Library.svelte` (→ Downloads), `pages/SeriesDetail.svelte`, `lib/components/{Hero,Carousel,PosterCard,Sidebar}.svelte`, `lib/api.ts`.

## Phasing

- **Phase 0 — contract freeze** (this doc). Backend may stub `/discovery`, `/tracked`,
  `/track` with canned data so the frontend starts immediately.
- **Phase 1 — backend discovery** (super-backend): AniList feed queries, `internal/discovery`
  service + wiring, `GET /discovery` + DTOs.
- **Phase 2 — frontend home** (super-frontend, parallel with Phase 1): `Home.svelte`,
  discovery `PosterCard` variant, Hero adapter, skeletons, `api.getDiscovery`, route swap.
- **Phase 3 — tracking + status** (super-backend): `00004` migration + `user_status`,
  `handleTrackSeries`, pause/drop/resume, `/available`, `/tracked` grouping, poller gating,
  manual-download re-engage.
- **Phase 4 — frontend tracking** (super-frontend): "Download & track" wiring, series page
  unification (Pause/Drop/Resume + available episodes), Library → Downloads reframe.

Backend and frontend touch disjoint directories (`internal/*`/`cmd`/`db` vs `frontend/*`)
so they run in parallel safely. Both treat the API contract above as frozen.

## Risks & edge cases

- **Rate limit:** in-memory cache → 0 AniList calls per load; ~5–7 background requests/hour;
  serve stale on 429. The `/track` per-id `GetMedia` is one extra call/action, already cached + 429-tolerant.
- **Empty "Currently downloading":** hidden when nothing tracked; home still full via discovery rows. Old global empty state removed.
- **Cold cache first load:** rows empty for ~5s after boot → skeletons; `/discovery` returns 200 (never error).
- **Image/CSP:** covers/banners only from `s4.anilist.co`/`img.anili.st` (already in CSP `img-src`, pinned by `safeImageURL`); non-allowlisted → "" → placeholder. No CSP change; SPA never calls AniList directly (`connect-src 'self'` stays).
- **Status derivation vs manual override:** automation gated on `user_status IS NULL`; a manual Pause/Drop is never overwritten. Manual episode download clears `user_status` → Active.
- **Idempotent track:** dedupe by anilist id; ensure feed + subscription exist on re-track (a series added earlier with `subscribed=0`/no feed gets upgraded).
- **Provider/feed on auto-track:** default provider + SmartSearch from metadata; sparse titles may match poorly at first (acceptable v1; existing autoselect guards quality).
- **`is_adult`:** discovery queries pass `isAdult:false` to keep the home SFW.
