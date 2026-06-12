# Extensions-Only Sourcing + First-Run Setup — Design (Spec B)

**Date:** 2026-06-11
**Status:** Approved (brainstormed with user)

Spec B of the legitimize-the-app effort (Spec A shipped the About page / version / sponsor /
contrast work). This moves all sourcing out of built-in scrapers and into user-installed
extensions, the Hayase model — so the binary ships no site-specific scraper.

**No backward compatibility.** This is a pre-release personal project; older code is removed
cleanly, with no compat shims or data migration.

## Goal

1. The app ships **zero** built-in sources. All sourcing comes from user-installed JS extensions
   (the goja/hibike runtime that already exists). Nyaa/SubsPlease scrapers are deleted from the
   binary.
2. A first-run welcome + an Extensions management page let a user add a source. Browsing works
   without one; downloading is soft-gated until one exists.

## Background (verified)

The extension runtime already implements the same `source.Provider` interface the poller and
search handlers consume, and the manager registers enabled JS extensions into the same
`source.Registry`. So this is mostly deletion + UI + a few correctness fixes, not new plumbing.
Key facts from the code audit:
- `extension.JSProvider` satisfies `source.Provider` (compile-time asserted). The hibike-shaped JS
  contract (`single`/`search`/`smartSearch`/`getLatest`) is already adapted.
- `Manager.LoadAndRegisterAll` compiles enabled `torrent` extensions at boot and registers them.
- A **type-tag bug** exists: Go uses `"torrent"`, the DB CHECK + seed use `"anime-torrent"`, so
  `ListEnabledExtensionsByType("torrent")` matches nothing and installs would violate the CHECK.
- Enable/disable only takes effect on next boot (registry has no `Unregister`).
- No first-run flag exists; `settings` is a singleton row.
- The Hayase index format (`{id,name,version,type:"torrent",code:<raw-js-url>,nsfw,accuracy,icon,update,languages}`)
  maps onto the existing `IndexEntry` struct.

## 1. Sourcing model (backend)

- **Delete** `internal/source/nyaa.go`, `internal/source/subsplease.go`, and their registration in
  `NewRegistry` — the registry starts **empty**. Remove `seedBuiltinExtensions` and the native
  `is_builtin` rows. Keep the generic framework: `Provider`, `Registry`, `autoselect`,
  `match.go`, and habari parsing (`parse.go`).
- **Fix the type-tag**: standardize on `"torrent"` everywhere (the Hayase index value). Update the
  DB CHECK constraint (new goose migration; ASCII-only SQL), the Go `ExtType*` constants, and the
  `ListEnabledExtensionsByType` call sites. Drop `manga`/`online-streaming` from the consumed set
  (only `torrent` is sourced).
- **habari enrichment in the adapter**: run `enrich()` over extension-returned torrents so JS
  sources that return raw results still get release-group/resolution/episode/info-hash parsed,
  keeping `autoselect.SelectBest` effective. (Adapter previously did not parse JS results.)
- The manager's compile-and-register path becomes the ONLY way a provider enters the registry.

## 2. Live apply, no restart (backend)

- Add `Registry.Unregister(id string)`.
- Manager registers a provider into the registry on **install/enable** and unregisters on
  **disable/uninstall**, so adding or removing a source takes effect immediately without a daemon
  restart. (`LoadAndRegisterAll` still runs at boot for already-enabled extensions.)

## 3. Extensions management page (frontend)

- New `frontend/src/pages/Extensions.svelte` + a sidebar nav entry. Two sections:
  - **Repositories** — paste a repo index URL to add; list added repos with sync (re-fetch index)
    and remove. No bundled/suggested URL (legal posture).
  - **Installed sources** — each shows name, version, NSFW badge, an enable toggle, and remove.
    Install/enable/disable/remove apply live (section 2).
- Type the existing `api.ts` extension methods (currently `unknown[]`) against real DTOs
  (`ExtensionRepo`, `Extension`).
- **NSFW**: extensions with `nsfw:true` are hidden by default; a "Show NSFW sources" toggle (a new
  boolean setting) reveals them. Index entries still install; the UI filters display.

## 4. First-run + soft gating (full-stack)

- **`setup_completed`** boolean on the `settings` singleton (new column, default 0; ASCII-only
  migration). Exposed on the settings DTO.
- **Welcome dialog**: on first launch (`setup_completed` false), a one-screen modal explains the
  app needs a user-installed source, suggests no specific repo, and offers "Go to Extensions" +
  "Dismiss". Setting `setup_completed` true on dismiss or on first successful install.
- **Soft gating**: browsing/discovery always works (AniList, no source needed). Download/track
  actions — Hero "Download & track", SeriesDetail download/track + available-episode download —
  stay **enabled**. When clicked with no enabled source, they open an **"Add a source first"**
  prompt that routes to Extensions, instead of performing the action. Backed by a lightweight
  "has an enabled source" signal (derive from the extensions list, or a tiny `source_count` field
  on an existing payload — implementer picks the cheaper wiring).

## 5. Cleanup & disclaimer

- No data migration. Feeds that referenced the removed `nyaa`/`subsplease` providers are simply
  orphaned; the poller **skips any feed whose provider id is not registered** (defensive, no
  crash) — this is robustness, not backward-compat.
- Add the deferred README sentence to the Disclaimer section: "Extensions are unaffiliated with
  ssanime-gui and may be removed if they violate copyright laws."

## 6. Testing

- **Go**: `Registry.Register`/`Unregister`; manager install → register → `SmartSearch` via a fake
  in-memory JS extension; `ListEnabledExtensionsByType("torrent")` returns installed extensions
  after the type-tag fix; poller skips a feed with an unregistered provider; `setup_completed`
  round-trips through settings.
- **Frontend**: Extensions page repo-add + install/enable/remove; the no-source gating prompt
  fires on a download action; NSFW filter; `svelte-check` + build clean.
- **Live**: add a real repo **as a test fixture only** (e.g. a Hayase-format index — not bundled
  in the app), install + enable a torrent source, and run a real end-to-end
  discover → track → download → encode from the new button.

## Post-merge cleanup — full clean slate, restart at 0.1.0 (decided)

After Spec B merges and the repo is in final shape, do a complete fresh start (the v0.1–v0.3
releases predate a usable/finalized UI and aren't worth preserving). Scope is **decided**; only
the timing/go-ahead is confirmed immediately before the force-push. Order: do the doc deletions
+ scrub as a normal commit first, then the squash so the single initial commit is already clean.

1. **Delete `docs/superpowers/` entirely** — every spec and plan, including the specs written
   during this effort (about-page, series-detail, discovery-home, extensions-first-run) and the
   original 2026-06-06 spec/plans. From the clean slate forward, new specs/plans start over.
2. **Scrub every remaining doc of stale / built-in-scraper references** — `CLAUDE.md`, `README.md`,
   and `docs/reference/*`: remove or rewrite anything describing built-in nyaa/subsplease
   providers, "sourcing: torrents-primary via built-in providers", the abandoned-attempts
   transcripts, and any other content that no longer matches the extensions-only app. Delete
   reference docs that are wholly about the superseded design rather than scrubbing line-by-line.
3. **Squash** all git history into a single fresh `initial commit` on `main` and **force-push**
   (temporarily lift branch protection, then restore it). No prior history remains.
4. **Delete all GitHub releases + tags** (v0.1.0 → v0.3.x) via `gh release delete --cleanup-tag`.
5. **Reset versioning** — `.release-please-manifest.json` and `tauri.conf.json` version so the
   **next release is 0.1.0** and release-please starts a fresh changelog/lineage.

This is irreversible. Announce the force-push step before running it; do not do it silently.

Note: `CLAUDE.md`'s sourcing/architecture sections are updated to extensions-only **during** Spec B
implementation (it is the living project doc), so the clean-slate scrub is mostly a final
verification pass plus the `docs/superpowers/` deletion.

## Out of scope

- Streaming/manga extension types (only `torrent` is consumed).
- Extension settings UI / per-extension config beyond enable/disable + NSFW.
- Hot-reloading an extension's *code* on repo re-sync mid-run (re-sync updates the stored payload;
  it takes effect on next enable cycle or boot — not a live code swap).
