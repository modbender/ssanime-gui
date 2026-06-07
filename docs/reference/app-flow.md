# App flow, folder structure & cleanup (decided 2026-06-06)

UI is **Seanime-shaped**, but every "play/stream" affordance is replaced by **select → download →
encode → organize → clean up**. Two entry points (manual selection + automatic feeds) converge on one
pipeline.

## Pages (mirror Seanime)

- **Library** — AniList-poster grid of series; per-series archived/available counts + space saved.
- **Series detail** — episode list. Each episode shows its state (below). Seanime's ▶ Play becomes a
  checkbox + **Download & Encode**. Supports **single, multi-select, range, and "all"**.
- **Queue** — live download + encode progress (SSE), per job and per output resolution.
- **Auto-downloader** — watched-series feeds; new episodes auto-run the same pipeline.
- **Profiles** — encode profiles (inheritance, all x265 knobs, `output_resolutions`).
- **Settings** — paths, naming, cleanup, concurrency, providers, download clients, default profile.
- **Logs** — streamed log view (SSE).

## The pipeline (manual select and feed-auto both use this)

```
search providers (habari + AniList smart-match)
  → autoselect best original release (trusted group, native res)
  → download original  (anacrolix embedded / external client)        [episodes.status=downloading]
  → downloaded                                                        [downloaded]
  → encode: fan into ONE encoded_outputs row per chosen resolution    [encoding]
  → per output: encode → thumbnail → archive (move into library)      [encoded → thumbnailing → archived]
  → when ALL outputs archived: clean up original                      [episode fully archived]
```

`episodes.status` tracks the source/overall; each resolution's progress lives on its
`encoded_outputs` row. An episode is **fully archived** only when *every* selected output is archived.

## Folder structure (FINAL)

- **Naming:** Jellyfin/Plex standard — `<Series> - S{season:02}E{episode:02}.{ext}`.
- **Layout:** per-resolution subfolder under each season.
- **Single encoded root**, resolution as a subfolder (not separate roots).

```
<encoded_root>/
  <Series Name>/
    Season 01/
      1080p/  <Series Name> - S01E01.mkv
      720p/   <Series Name> - S01E01.mkv
      480p/   <Series Name> - S01E01.mkv
```

### Path-builder rules
- `<Series Name>` = AniList English (fallback romaji) title, **filesystem-sanitized** (strip
  `\ / : * ? " < > |`, trim trailing dots/spaces).
- **Season** = `series.season_number` (default 1, editable per series; auto-suggested from habari
  season parse / AniList). Anime via AniList is usually one season per entry → `Season 01`.
- **Episode** = `episodes.episode_no` (from habari/feed), zero-padded to 2 (3+ if the number needs it,
  e.g. long-runners `E1090`). Movies/OVAs/specials (`episode_no` NULL) → `Season 00` / `<Series> -
  S00E01` (Jellyfin specials convention) or a `Movies/` form — handle as specials.
- `{ext}` = `encode_profiles.container` (default **mkv** — softsubs/multi-audio friendly).
- The computed absolute path is stored in `encoded_outputs.encoded_path`.
- **Originals** download to `<download_root>/...` (working area, can be uuid/temp-named; structure
  there doesn't matter since it's deleted).

## Cleanup policy (FINAL — auto-delete)

- After **all** selected `encoded_outputs` for an episode reach `archived` (encode + thumbnail +
  moved into library): **stop seeding**, then **delete the original** source file(s).
- Triggers only on full success; on any output `error` the original is **kept** so it can be retried.
- Configurable in Settings: `cleanup_policy` = `delete` (default) | `keep` | `move`
  (`move` → `<processed_dir>`). `delete` removes the anacrolix torrent + data after seeding stops.

## Subscriptions, favorites & derived status

- **Favorite** (`series.favorite`) — list membership only; curate your collection. No polling.
- **Subscribe** (`series.subscribed`) — turns on **auto-poll + auto-download + auto-encode** of new
  episodes via the series' feeds. This is the "watched series" mechanism.
- **Derived status** (computed from AniList `airing_status` × local archive completeness — *not* a
  MAL-style watch flag). Episodes archived = count of episodes whose every selected output is archived.

| status | condition | auto-poll |
|---|---|---|
| `not_aired` | AniList `NOT_YET_RELEASED` | no (nothing to fetch) |
| `airing` | `RELEASING`, latest aired episode **not** yet archived | yes (if subscribed) |
| `up_to_date` | `RELEASING`, all aired episodes archived, more to come | yes — waiting for next weekly drop |
| `incomplete` | `FINISHED`/`HIATUS`, some episodes still missing/unarchived | yes (if subscribed) — catch up |
| `completed` | `FINISHED` **and** all episodes archived | **no — polling auto-stops** |
| `cancelled` | AniList `CANCELLED` | no |

**Auto-poll rule:** poll a feed only when `series.subscribed AND status ∉ {completed, cancelled,
not_aired}`. So a finished + fully-archived series stops polling automatically (your "Completed → no
auto-poll" requirement), while a weekly-airing subscribed series keeps checking for the next episode.
Status is computed on read (cheap join: archived-episode count vs `episode_count` + `airing_status`);
cache into a column only if the Library grid needs it for sorting/filtering.

## Settings (implied fields)

`download_root`, `encoded_root`, `cleanup_policy` (+ `processed_dir` when `move`),
`naming_template` (default the Jellyfin pattern; tokens `{series} {season} {episode} {res} {group}
{ext} {crc}`), `concurrency_download`, `concurrency_encode`, `default_profile_id`,
`download_backend` → `download_clients`, provider/extension config, `port`.
