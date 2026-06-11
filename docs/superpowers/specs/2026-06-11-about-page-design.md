# About Page, Dynamic Version & Sponsor — Design (Spec A)

**Date:** 2026-06-11
**Status:** Approved (brainstormed with user)

This is **Spec A** of a two-part effort. Spec B (extensions + first-run setup, the larger
piece) is brainstormed separately. A does not depend on B and ships first.

## Goal

1. Give the app a single source of truth for its version — injected into the Go binary at
   build time, exposed over the API, never hardcoded in the UI again (the stale `v0.1`
   sidebar label was already removed).
2. Add an **About** page reachable from the sidebar: app description, the live version, and
   project links.
3. Add a **Sponsor / donation** button to the sidebar that opens the maintainer's GitHub
   Sponsors page.
4. Add a Seanime-style legal disclaimer to the README.

## Version — single source of truth

- New package `internal/version` with `var Version = "dev"` and `var Commit = ""`.
- Injected at build time via `-ldflags "-X github.com/modbender/ssanime-gui/internal/version.Version=<v> -X .../internal/version.Commit=<sha>"`:
  - **Mage** (`server`, `sidecar` targets): derive `<v>` from `git describe --tags --always --dirty`
    and `<sha>` from `git rev-parse --short HEAD`. With no tags reachable it falls back to the
    commit-ish / `"dev"`, which is honest for local builds.
  - **CI release builds** (`release-please.yml` and `release.yml` sidecar build steps): inject the
    same way. Those jobs check out the release tag, so `git describe --tags` yields the clean
    `vX.Y.Z`. The `ci.yml` PR-check builds are not shipped and need no injection.
- No change to the existing release-versioning mechanism: release-please still owns
  `tauri.conf.json` / the manifest; this only teaches the **Go binary** to report the same value
  the tag already encodes.

### Endpoint

`GET /api/version` -> `{ "version": string, "commit": string }` (standard `{data,error}`
envelope). Read straight from `internal/version`. No auth beyond the existing localGuard.
The leading `v` is stripped for display on the frontend, not in the payload.

## About page

- New route `/about` -> `frontend/src/pages/About.svelte`.
- Content (de-rounded, matching existing page styling):
  - App logo mark + name (**ssanime**) + a one-line description matching the README tagline.
  - **Version** fetched from `/api/version`, shown as `vX.Y.Z` with the short commit beside it
    (e.g. `v0.3.0 · a1b2c3d`). While the fetch is in flight, a small skeleton; on failure, the
    version line is simply omitted (best-effort, never an error screen).
  - A short paragraph on what the app does and that it is free / open-source (GPL-3.0).
  - **Link row:** GitHub repository, Report an issue (repo issues), License (GPL-3.0 — link to the
    repo `LICENSE`). All open externally in a new tab.
  - A prominent **Sponsor** button -> `https://github.com/sponsors/modbender` (external, new tab),
    with a one-line "support development" blurb.
- These static strings (description, repo URL, sponsor URL) live in the frontend — only the
  *version* is dynamic. "No hardcoding" applied specifically to the version, which is now
  API-sourced.

## Sidebar additions

Two controls at the bottom of the existing icon rail (`Sidebar.svelte`), below the nav group:

- **About** — an info (`i`) icon -> internal navigation to `/about`, styled like the other nav
  items (active-pip + hover tooltip "About").
- **Sponsor** — a heart icon button that opens `https://github.com/sponsors/modbender` in a new
  tab (external link, not a route). Tooltip "Sponsor". Visually distinct (e.g. a warm accent on
  hover) so it reads as a support action, not navigation.

## README legal disclaimer

Add a short disclaimer section near the License:

> ssanime-gui does not provide, host, or distribute any media content. Users are responsible for
> obtaining media through legal means and complying with the laws of their jurisdiction.

The "extensions are unaffiliated with ssanime-gui and may be removed if they violate copyright
laws" sentence is deferred to **Spec B**, when extensions become user-visible — it reads oddly
before any extension UI exists.

## Accent-contrast fix (folded in)

The primary "Download & track" button (`Button.svelte` `default` variant) is hardcoded
`bg-[var(--accent)] text-white`. The accent is the per-series AniList `cover_color`, so light
covers (e.g. Dr. STONE's cream) render white-on-light — unreadable. Fix with a luminance-derived
foreground:

- **`utils.ts`**: add `accentForeground(coverColor)` -> returns a near-black (`#0a0a0a`) for light
  accents and white (`#ffffff`) for dark ones, using relative luminance
  (`0.2126 R + 0.7152 G + 0.0722 B` on sRGB-linearized channels) with a sensible threshold
  (~0.6). Falls back to white for the default accent.
- **`app.css`**: add `--accent-fg: #ffffff;` to the `:root` accent block (the default violet is
  dark enough for white text).
- **`Hero.svelte` and `SeriesDetail.svelte`**: wherever `--accent` / `--accent-rgb` are set inline
  from a series' cover color, also set `--accent-fg: {accentForeground(...)}` so it cascades.
- **`Button.svelte`**: change the `default` variant from `text-white` to
  `text-[var(--accent-fg)]`. The `destructive` variant keeps `text-white` (the error red is always
  dark). Audit other `bg-[var(--accent)]` fills for the same swap; translucent accent fills
  (`rgb(var(--accent-rgb)/…)`) over dark surfaces are unaffected.

Verify against a light-cover title (Dr. STONE) and a dark-cover title that the button text stays
readable in both.

## Out of scope (and why)

- **`.github/FUNDING.yml`** — not added. The maintainer already has a global funding config in
  their community-health repo, which GitHub applies to this repo; a per-repo file would be
  redundant.
- Build date / richer build metadata — version + short commit is enough; YAGNI.
- Auto-update / "new version available" checks — separate concern, not requested.

## Testing

- **Go:** unit-test the `/api/version` handler returns the injected `version`/`commit` (override
  the package vars in the test); confirm the default is `"dev"` when not injected.
- **Build:** verify `mage server` injects a real `git describe` value (smoke-check the endpoint
  returns it, not `dev`).
- **Frontend:** `svelte-check` + build clean; live-verify the About page renders the fetched
  version and the Sponsor/About controls work (About navigates, Sponsor opens the external URL).
