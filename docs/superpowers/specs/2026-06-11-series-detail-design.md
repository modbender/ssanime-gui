# Series Detail Page + Global De-rounding — Design

**Date:** 2026-06-11
**Status:** Approved (brainstormed with user; Hayase reference: `docs/reference/HayaseSeries1.png`)

## Goal

1. Turn the series page into a full Hayase-grade detail page — synopsis, genres, score,
   studio, trailer, relations, recommendations, and a unified episode grid with
   per-episode thumbnails — identical in shape for tracked series and untracked
   discovery previews.
2. Remove rounded corners across the entire UI (user preference: no rounded), with the
   only circular survivors being the ~6px status indicator dots and the loading spinner.

## Background / gaps today

- The untracked preview (`/series/anilist/:id`) renders only the hero + a "Not tracked
  yet" panel, and dies entirely if the title fell out of the in-memory discovery cache.
- Even the tracked view has no synopsis/genres/score/studio — neither `SeriesDetail` nor
  `DiscoveryItem` carries them; nothing in the backend fetches them.
- Episode rows are a plain pipeline table; no thumbnails, titles-from-metadata, air
  dates, or overviews.
- All other pages (Home, Downloads, Queue, Feeds, Profiles, Settings, Logs) were audited
  and are complete — this design touches them only for the de-rounding sweep.

## Data sources

| Source | Provides | Notes |
|---|---|---|
| AniList `Media` (full query) | description, genres, averageScore, studios(isMain), source, season/seasonYear, duration, trailer{id,site,thumbnail}, streamingEpisodes{title,thumbnail}, relations, recommendations, nextAiringEpisode | One GraphQL request; subject to the ~30 req/min limit the `anilist` package already manages |
| ani.zip mappings (`GET https://api.ani.zip/mappings?anilist_id=N`) | per-episode thumbnail (TVDB artwork), title, air date, runtime, overview | What Hayase itself uses; free, no auth, broad coverage incl. older shows |

Episode thumbnail precedence: **ani.zip → AniList `streamingEpisodes` → tinted
placeholder** (accent-tinted block with the episode number).

## Backend

### `GET /api/anilist/{id}/detail` (new)

- Looks up new table `anilist_detail_cache(anilist_id INTEGER PRIMARY KEY, payload TEXT
  NOT NULL, fetched_at INTEGER NOT NULL)` (goose migration + sqlc queries).
- Fresh row (< 24 h): unmarshal and serve.
- Stale/missing: fetch AniList Media and ani.zip **in parallel**, merge into one
  `AnilistDetail` payload, upsert, serve.
- Failure posture (matches discovery cache): on AniList 429/error or ani.zip error,
  serve the stale row if one exists; ani.zip failing alone degrades to
  AniList-only payload (episodes lose thumbnails/overviews, page still works). Only
  error out when there is neither a cache row nor a successful AniList response.
- The existing `POST /series/{id}/refresh` also deletes/refreshes this row so the
  Refresh button busts both metadata layers.

### `AnilistDetail` payload (served verbatim to the frontend)

```
{
  anilist_id, description_html_stripped, genres[], average_score, studio,
  source_material, season, season_year, duration_min, episode_count,
  next_airing: {episode, airing_at} | null,
  trailer: {site, video_id, thumbnail} | null,
  episodes: [{number, title, thumbnail, air_date, overview, runtime_min}],
  relations: [{anilist_id, relation_type, title_english, title_romaji,
               cover_image, cover_color, format, status}],
  recommendations: [{anilist_id, title_english, title_romaji, cover_image,
                     cover_color, format, status}]
}
```

`episodes` is the merged ani.zip + streamingEpisodes list, sorted by number. Relations
and recommendations carry enough to render `DiscoveryCard`s and navigate to
`/series/anilist/:id` — which now always resolves through this endpoint, removing the
"no longer in the discovery cache" dead end (the preview path stops depending on
`getPreview` alone; the discovery cache becomes an optimization, not a requirement).

## Frontend — series page layout (Hayase-aligned)

Top to bottom, same shape for tracked and untracked:

1. **Hero** (existing cinematic banner + poster left) — unchanged structurally; meta
   badges (episode count, format, airing status, year, archived count) stay under the
   title.
2. **Synopsis** — clamped to ~4 lines with a "more" toggle; plain text (backend strips
   AniList's HTML).
3. **Action row** — existing buttons (Download & track / Pause / Drop / Resume / Scan /
   Refresh / bulk-encode) plus a **Trailer** button when available — opens YouTube
   externally (no iframe; keeps CSP frame-free).
4. **Genre chips** — directly under the action row (Hayase ordering); display-only.
5. **About strip** — score, studio, source material, season/year, duration, next-airing
   countdown; compact label/value pairs.
6. **Unified episode grid** — two-column responsive grid of **horizontal cards**:
   thumbnail left (16:9, ~160px), then `E07` + title, a 2-line overview snippet, and a
   relative air date ("3 days ago"). Merges three layers keyed by episode number:
   - **ani.zip/AniList metadata** — thumbnail, title, air date, overview (all episodes).
   - **DB pipeline episodes** — status badge, live progress bar (SSE), output chips,
     retry action, bulk-encode checkbox (replaces the old pipeline table entirely).
   - **Source-check results** — "Check source" button stays in the section header; hits
     light matching cards up with a Download button in place (replaces the separate
     "Available at source" panel).
   Future episodes (air date in the future / beyond nextAiring) render dimmed with
   "airs ‹date›". Episodes without numbers (specials) append at the end. Untracked
   preview: identical grid, pipeline/download affordances replaced by the single
   "Download & track" CTA in the action row.
7. **Relations** then **Recommendations** — poster `Carousel` rows reusing
   `DiscoveryCard`; clicking navigates to that title's preview page.

Loading: skeleton blocks for synopsis/grid while `/detail` is in flight; the page
renders hero immediately from whatever it already has (series row or discovery preview).

## De-rounding sweep

- `--radius-card`, `--radius-lg`, `--radius-xl` → `0` in the Tailwind `@theme`.
- Remove/replace every `rounded`, `rounded-md`, `rounded-lg`, `rounded-xl`,
  `rounded-2xl`, `rounded-[…]`, and `rounded-full` utility across the audited files
  (~20: all pages, Sidebar, Hero, DiscoveryCard, PosterCard, Button, Modal, Badge,
  Input, Select, Carousel, CarouselSkeleton, ProgressBar, plus SeriesDetail).
- Exceptions (stay `rounded-full`): the small status indicator dots (≤8px) and the
  `Spinner`.
- Pills/badges become sharp rectangles; the progress bar becomes a hard bar; the Modal
  loses its bezel radii (including the hardcoded `rounded-[1.75rem]` pair).
- No `border-radius: 0 !important` global override — each usage is edited so the code
  reads as designed-square.

## CSP

`img-src` allowlist gains:
- `https://artworks.thetvdb.com` (ani.zip episode thumbnails)
- `https://img1.ak.crunchyroll.com` (AniList streamingEpisodes thumbnails)
- `https://i.ytimg.com` (trailer thumbnail)

No `frame-src` (trailer opens externally). `connect-src` stays `'self'` (ani.zip is
fetched by the daemon, not the browser).

## Error handling

- ani.zip down → AniList-only payload; grid renders with placeholders, no error UI.
- AniList 429 → stale cache row served; if none, endpoint returns the error and the
  page shows the existing error state with a retry affordance.
- Thumbnail `onerror` → swap to the tinted placeholder (broken TVDB art happens).
- Episode-number collisions between ani.zip and DB rows resolve in the DB row's favor
  (pipeline truth wins for status; metadata still decorates it).

## Testing

- **Go:** cache fresh/stale/429-stale paths; merge with ani.zip present/absent/partial;
  refresh busting; endpoint contract (httptest).
- **Frontend:** `svelte-check` + build clean.
- **Live:** tracked series and untracked preview, each verified with a title that has
  ani.zip coverage and one that doesn't; de-rounding eyeballed across all pages via
  Playwright screenshots.

## Out of scope

- Embedding the trailer player (external open only, for CSP).
- Persisting episode metadata per-row into the `episodes` table (the JSON cache is the
  persistence layer — B was satisfied by making the cache durable, not by schema
  expansion).
- Character/staff lists (Hayase shows none prominently; YAGNI).
