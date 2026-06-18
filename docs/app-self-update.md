# App self-update — design & implementation plan

**Status: design only — not implemented.** This captures the agreed approach for adding
Hayase-style app self-update as its own effort, to be reviewed before any code is written. It is the
companion to the dependency auto-update that already shipped (source extensions —
`internal/extension/update.go`; ffmpeg deliberately deferred).

## Why this is separate from dependency auto-update

The update story splits into two subsystems with different lifecycles:

| | Dependency auto-update (shipped) | App self-update (this doc) |
|---|---|---|
| Targets | source extensions (ffmpeg deferred) | the SSAnime desktop app (shell + bundled daemon) |
| Visibility | silent, no user action | user-visible badge — it changes the running app and needs a restart |
| Apply | live / next start, in the daemon | via the OS installer + relaunch, in the Tauri shell |

Extensions are *dependencies* — refresh them in the background and move on. The app is *the thing the
user is running* — so it gets a visible "Update available" affordance and a restart, exactly the
Hayase pattern: a badge on the sidebar → click to update now; if ignored, apply on quit.

## Scope and hard constraints

- **Desktop distribution only.** Self-update flows through Tauri's updater, which only exists in the
  Tauri desktop bundle. The standalone, browser-served daemon binary **cannot** self-update this way —
  those users update manually or via a package manager. The plan must surface this everywhere.
- **One update covers shell + daemon.** The Tauri bundle ships the Go daemon as a sidecar
  (`bundle.externalBin: ["binaries/ssanime"]`), so updating the app replaces the daemon too.
- **Platforms: Windows (NSIS) + Linux (AppImage) first.** macOS is deferred — its CI build is disabled
  (no Apple Developer cert; an unsigned `.dmg` is Gatekeeper-blocked), and the updater needs a signed
  `.app.tar.gz` artifact + notarization. `.deb`/`.rpm` are **not** self-updating (package-manager
  territory) — on those, the badge should link to the download page rather than self-install.

## Architecture — detection vs install split (the crux)

The Tauri window loads the SPA from `http://localhost:4773` (the daemon serves it; `frontendDist` is
a placeholder — see `desktop/src-tauri/tauri.conf.json`). To Tauri that is a **remote origin**, and
Tauri v2 gates plugin/IPC access by capability *and* origin. That forces a clean split:

1. **Detection → the daemon.** It already has a guarded HTTP client + DoH resolver and a build-stamped
   version (`internal/version`). It queries the GitHub `releases/latest` tag, compares to the running
   version, and exposes the result over REST (e.g. `GET /api/app/update` → `{available, version,
   notes, url}`). The SPA renders the sidebar badge from that. This works in **both** distributions —
   the standalone build just points the badge at the releases page.
2. **Install → Tauri (desktop only).** `tauri-plugin-updater` (Rust) downloads the signed bundle,
   verifies its minisign signature, runs the platform installer, and relaunches. The remote SPA cannot
   call the plugin directly unless a capability is granted to the `localhost:4773` origin.

**Bridge options (decide at implementation):**
- **(A, recommended) Rust-driven.** Keep updater logic in Rust (`setup` + window `CloseRequested`).
  Expose a single narrow custom command `apply_app_update` to the localhost origin via a capability
  scoped with `remote.urls: ["http://localhost:4773"]`, and push progress/state events to the SPA.
  Minimises what the remote origin can reach.
- **(B) JS-driven.** Expose `@tauri-apps/plugin-updater` + `@tauri-apps/plugin-process` to the remote
  origin via the capability and drive `check()`/`downloadAndInstall()`/`relaunch()` from the SPA.
  Less code, but exposes the whole updater surface to a remote origin — weaker security posture.

## Signing & keys — minisign, separate from Authenticode

Tauri's updater uses its **own minisign keypair**, distinct from the Windows Authenticode signing
already pre-wired (but inert) via SignPath in `release-please.yml`. Both are wanted: **Authenticode**
makes SmartScreen trust the *installer*; **minisign** lets installed apps verify *update* integrity.

- Generate: `bunx tauri signer generate -w ssanime-updater.key` (password-protected).
- **Public key** → `tauri.conf.json` `plugins.updater.pubkey` (committed).
- **Private key + password** → GitHub Actions secrets `TAURI_SIGNING_PRIVATE_KEY` +
  `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`. Never commit the private key.
- **Loss = no more updates.** If the private key is lost, already-installed apps will reject every
  future update (wrong/again-unsigned), forcing a manual re-install for all users. Back it up offline +
  in a password manager and document recovery.

## Manifest & endpoints

- `tauri.conf.json`: set `bundle.createUpdaterArtifacts: true` (emits `.sig` files + updater-shaped
  bundles), plus `plugins.updater.pubkey` and
  `plugins.updater.endpoints: ["https://github.com/modbender/ssanime-gui/releases/latest/download/latest.json"]`.
- `latest.json` (per-release manifest): maps platform → `{version, pub_date, url, signature}`.
  `tauri-action` generates it + the `.sig` files when the signing env is present; the workflow then
  uploads `latest.json` alongside the installers. The `releases/latest/download/...` redirect means
  installed apps always read the newest manifest. (`{{target}}`/`{{arch}}`/`{{current_version}}`
  templating is available if a dynamic endpoint is ever preferred over the static file.)

## CI changes (`release-please.yml`, mirrored in `release.yml`)

- Append `TAURI_SIGNING_PRIVATE_KEY` + `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` to the `tauri-action` `env:`
  block in **both** workflows. The block already carries a real key (`APPIMAGE_EXTRACT_AND_RUN`) — add
  to it, never reduce it to comments (an empty `env:` breaks the workflow).
- With `createUpdaterArtifacts`, extend the `gh release upload` steps to also attach `latest.json` and
  the updater bundles. **Verify exact artifact paths on the first signed release** (same "untested
  until secrets exist" caveat the SignPath steps carry).
- **Version sync.** The updater compares `tauri.conf.json` `version` against `latest.json`'s released
  version. release-please must bump `tauri.conf.json` in lockstep — add it to release-please
  `extra-files` (or inject the tag in a build step). Today `version` (0.4.0) is maintained separately;
  confirm it tracks the tag or the updater will mis-compare.

## Per-platform notes

- **Windows.** The updater runs the NSIS installer silently. The repo uses a **custom NSIS template**
  (`desktop/src-tauri/windows/installer.nsi`) — verify it honors the updater's silent/passive flags
  (`/S`, install dir, no UI) or silent self-update fails. The daemon sidecar may be running at swap
  time; ensure tray-quit stops it first (`procguard` reaps ffmpeg children).
- **Linux.** AppImage self-updates; `.deb`/`.rpm` do not. Detection still shows the badge, but the
  install path must branch — only self-install for AppImage; send package installs to the download
  page. (Detecting *how* it was installed is the hard part — likely gate self-install on a build flag
  or the AppImage env marker.)
- **macOS.** Deferred until an Apple Developer cert exists (signed `.app.tar.gz` updater artifact +
  notarization).

## Daemon-first lifecycle nuances

- **"Apply on quit" = the Tauri shell exit / window `CloseRequested`**, not browser-tab close. Confirm
  desktop-window close semantics first: `tauri.conf.json` has `windows: []` and the tray keeps the
  daemon alive in the standalone build — define whether closing the desktop window quits the app or
  minimises to tray, since that decides when the on-quit install fires.
- **In-flight jobs.** An app update relaunches the process and interrupts downloads/encodes. Gate the
  auto-on-quit install on "no jobs mid-flight," or checkpoint first. Verify torrent-resume and
  encode-restart actually recover across a relaunch before enabling silent on-quit install.

## Implementation phases

1. **Detection only** — daemon REST endpoint + sidebar badge, no install. Works in both distributions,
   low risk, immediately useful; badge links to the releases page.
2. **Updater plumbing (desktop)** — minisign keys, `tauri.conf.json` updater config +
   `createUpdaterArtifacts`, the capability + Rust `apply_app_update` command, CI signing env +
   `latest.json` upload. Validate against a throwaway pre-release.
3. **UX wiring** — "update now" from the badge → `apply_app_update`; apply-on-quit hook with the
   job-in-flight guard.
4. **Polish** — download progress, release-notes display (`update.body`), failure/rollback handling,
   macOS once a cert is available.

## Open decisions (resolve before/at implementation)

- Bridge **A vs B** (recommend A).
- Does closing the desktop window **quit or minimise to tray**? (gates apply-on-quit)
- **In-flight-job policy** for on-quit install: defer vs checkpoint-then-update.
- Linux: **AppImage-only** self-install; `.deb`/`.rpm` → download page.
- **Backup/storage** of the minisign private key.

## References

- Tauri v2 updater: <https://v2.tauri.app/plugin/updater>
- Release pipeline: `.github/workflows/release-please.yml`, `.github/workflows/release.yml`,
  `docs/distribution.md`
- Companion (shipped): `internal/extension/update.go` — dependency auto-update
