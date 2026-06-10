# Distribution & code-signing

How ssanime-gui is packaged and shipped, and the honest current state of Windows
code signing plus the concrete path to fixing it.

## What ships

The desktop deliverable is a Tauri v2 shell (`desktop/`) that wraps the Go daemon
as a [sidecar](https://v2.tauri.app/develop/sidecar/). `bundle.targets` is `"all"`,
so each OS emits every installer format it can under
`desktop/target/release/bundle/` (the cargo workspace root is `desktop/`). The release now builds on a 3-OS matrix
(`windows-latest`, `ubuntu-latest`, `macos-latest`) with `fail-fast: false`, so one
OS failing still ships the others. The headless `ssanime` binary (daemon + systray
+ browser UI) remains fully usable on its own; the Tauri app only adds a native
window.

| OS | Runner | Artifacts | Bundle subdirs |
|---|---|---|---|
| Windows | `windows-latest` (x64) | NSIS `.exe`, MSI `.msi` | `nsis/`, `msi/` |
| Linux | `ubuntu-latest` (24.04, x64) | `.deb`, `.rpm`, `.AppImage` | `deb/`, `rpm/`, `appimage/` |
| macOS | `macos-latest` (Apple Silicon, arm64) | `.dmg` | `dmg/` |

Each matrix leg uploads only its own bundles to the release with
`gh release upload "$TAG" … --clobber`, so re-runs are idempotent and the legs never
collide. A leg errors only if it produced **no** artifact at all — a missing optional
format (e.g. `.rpm` when `rpmbuild` is absent) is skipped, not fatal.

## The build order is load-bearing

The Go daemon `go:embed`s `internal/server/dist`. The release **must** build in
this exact order, or the sidecar ships without the SPA:

1. **Frontend** — `bun install && bun run build` in `frontend/`, which emits the
   compiled SPA to `internal/server/dist`. Identical on every OS.
2. **Sidecar** — compile the Go daemon per-OS into
   `desktop/src-tauri/binaries/`. The filename is the Tauri **sidecar target-triple
   name** — Tauri resolves `externalBin: ["binaries/ssanime"]` to the host triple
   plus extension. The SPA is now embedded in the binary. Per-OS settings:

   | OS | GOOS/GOARCH | CGO | ldflags | Output filename |
   |---|---|---|---|---|
   | Windows | `windows`/`amd64` | `0` | `-H=windowsgui -s -w` | `ssanime-x86_64-pc-windows-msvc.exe` |
   | Linux | `linux`/`amd64` | `0` | `-s -w` | `ssanime-x86_64-unknown-linux-gnu` |
   | macOS | `darwin`/`arm64` | `1` | `-s -w` | `ssanime-aarch64-apple-darwin` |

   Windows and Linux are **cgo-free** (`modernc.org/sqlite` + a pure-Go systray), so
   `CGO_ENABLED=0` keeps them statically linkable and cross-compilable. The **macOS
   systray (`fyne.io/systray`) binds the AppKit Objective-C runtime via cgo**, so the
   macOS sidecar must build with `CGO_ENABLED=1` (the `macos-latest` runner ships a C
   compiler, and it builds natively rather than cross-compiling, so this is fine).
3. **Tauri build** — `tauri-apps/tauri-action@v0` in build-only mode in `desktop/`,
   producing that OS's installers.

The build steps (frontend → sidecar) run as explicit steps before handing off to
`tauri-apps/tauri-action` for step 3, so each build phase is independently visible
in the logs. tauri-action runs in **build-only** mode (no `tagName`/`releaseName`),
and the installers are attached to the release with `gh release upload`.

## Cutting a release (automatic — release-please)

Releases are driven by [release-please](https://github.com/googleapis/release-please-action)
from **Conventional Commit** messages. **You never bump the version or tag by hand.**

How it works:

1. Land normal `feat:` / `fix:` / `perf:` PRs on `main` as usual.
2. On every push to `main`, `.github/workflows/release-please.yml` runs and
   maintains a single open **Release PR** titled like *"chore(main): release
   0.2.0"*. It stages the next version and an updated `CHANGELOG.md`, computed
   from the commits since the last release using **standard semver**:

   | Commit type | Bump |
   |---|---|
   | `feat!:` or `BREAKING CHANGE:` footer | **major** (e.g. `0.1.0` → `1.0.0`) |
   | `feat:` | **minor** (`0.1.0` → `0.2.0`) |
   | `fix:`, `perf:` | **patch** (`0.1.0` → `0.1.1`) |
   | `chore`, `docs`, `refactor`, `ci`, `build`, `test` | no bump (changelog/hidden only) |

   This is release-please's default behavior; the pre-1.0 dampeners
   (`bump-minor-pre-major` / `bump-patch-for-minor-pre-major`) are explicitly set
   to `false` in `release-please-config.json` so semver is strict even below 1.0.0.

3. **Merge the Release PR.** That makes release-please:
   - tag `v<version>` (e.g. `v0.2.0`) and publish the GitHub Release with the
     generated changelog as the body,
   - and, via `extra-files`, bump `desktop/src-tauri/tauri.conf.json`'s `version`
     in lockstep so the installer filenames match the tag (no manual edit).
4. Only once that release is created does the **`build-installers`** job
   (`if: release_created == 'true'`) run, as a 3-OS matrix, building the frontend →
   sidecar → Tauri installers on each OS and **uploading that OS's bundles to the
   release release-please just created** (via `gh release upload --clobber`): Windows
   `.exe`/`.msi`, Linux `.deb`/`.rpm`/`.AppImage`, macOS `.dmg`. Ordinary commits
   never trigger an installer build — exactly one build per release.

> The single source of truth for the version is `tauri.conf.json`'s `version`
> field, kept in sync with the git tag by release-please. There is no separate Go
> version constant.

### Required repo setting (one-time)

release-please opens its Release PR using the default `GITHUB_TOKEN`. For that to
work, **Settings → Actions → General → Workflow permissions** must have
**"Allow GitHub Actions to create and approve pull requests"** turned **ON**.
Without it, the `release-please` job fails to open the PR.

### Auto-passing CI on the Release PR

The Release PR is authored by the default `GITHUB_TOKEN`, and GitHub deliberately
**blocks token-authored events from triggering other workflows** — so `ci.yml` never
runs on the Release PR, and the branch-protection-required `backend`/`frontend` checks
stay missing, making the PR unmergeable without a manual close/reopen to re-trigger CI.

To fix this without weakening branch protection, the `release-please` job runs an
extra step (`auto-pass CI on release PR`) that finds the open Release PR by its default
`autorelease: pending` label and, for its head SHA, **creates check-runs** named
exactly `backend` and `frontend` with `conclusion=success` via the Checks API
(`gh api -X POST repos/$GITHUB_REPOSITORY/check-runs …`). This needs `checks: write`
(added to the workflow `permissions:` block) and is safe because the Release PR carries
**no code** — only a version bump + CHANGELOG.

Why **check-runs**, not commit **statuses**: branch protection requires github-actions
**app** check-runs of those names. A commit *status* of the same name is a different
object and would **not** satisfy an app-pinned required check. `GITHUB_TOKEN` creates
check-runs *as the github-actions app*, which matches. The step is idempotent (re-running
just re-creates the success check-runs) and a no-op when no Release PR is open.

### First run

`release-please-config.json` sets `bootstrap-sha` to the `main` HEAD that
introduced this workflow, so the **first** changelog starts fresh from that point
instead of replaying the entire repo history. The first Release PR appears once a
`feat`/`fix`/`perf` commit lands on `main` after the workflow is merged.

## Manual fallback (emergency builds)

`.github/workflows/release.yml` no longer triggers on tag push (that would
double-fire alongside release-please). It is now **`workflow_dispatch`-only**: a
maintainer can run it from the Actions tab with a `tag` input (e.g. `v0.2.0`) to
(re)build the installers and re-attach them to that **existing** release via
`gh release upload --clobber`. It never creates a release — use it only when the
automatic `build-installers` job failed and you need to retry the build by hand.

## Local build (for testing)

```sh
mage tauri
```

This runs the same ordering locally: `mage frontend` → `mage sidecar` →
`bunx @tauri-apps/cli@latest build`. Artifacts land in
`desktop/target/release/bundle/` (the cargo workspace root is `desktop/`).

## WebView2 runtime

`bundle.windows.webviewInstallMode` is `downloadBootstrapper`. The installer
fetches and runs the WebView2 bootstrapper at install time if WebView2 is absent.
Windows 11 ships WebView2 pre-installed, so on current systems this is a no-op;
older Windows 10 machines will download it during install (needs network at
install time). The bootstrapper keeps the installer small versus embedding the
full runtime.

## Windows code-signing posture

### Current state: UNSIGNED

`bundle.windows.certificateThumbprint` is `null`, so installers are **not signed**.
Consequences:

- **SmartScreen "unknown publisher" warning.** Users see a blue "Windows protected
  your PC" dialog and must click *More info → Run anyway*. SmartScreen reputation
  is per-signing-identity and builds over downloads/time, so even a fresh OV
  certificate warns until it accumulates reputation; an EV certificate is trusted
  immediately.
- No integrity guarantee that the binary wasn't tampered with in transit (beyond
  the GitHub Release's own HTTPS).

This is acceptable for a personal/early-stage GPL-3.0 tool. It is the single
biggest install-friction item to fix before any wider distribution.

### Upgrade path: signing options

Verified against the Tauri v2 Windows signing docs
(<https://v2.tauri.app/distribute/sign/windows/>). Current options, cheapest-to-most:

| Option | ~Cost / yr | SmartScreen | Notes |
|---|---|---|---|
| **Azure Trusted Signing** (formerly Azure Code Signing) | ~$120 ($10/mo) | Builds reputation like OV | Modern cloud route. No physical token; Microsoft validates your identity. Cheapest path to a real signature. Signs via a custom `signCommand` (e.g. `trusted-signing-cli`). |
| **OV (Organization Validation) cert** | ~$200–400 | Builds reputation over time | Now stored on a hardware token / HSM (CA/B Forum rule since 2023), which complicates CI — you can't just upload a `.pfx`. |
| **EV (Extended Validation) cert** | ~$300–600 | Trusted immediately | Hardware-token-bound; hardest to automate in CI. Use a cloud HSM or self-hosted signer. |

For a single maintainer wanting CI-friendly signing without a physical token,
**Azure Trusted Signing is the recommended route**.

### Where the config goes

Tauri v2 exposes these keys under `bundle.windows` in `tauri.conf.json` (verified
against the docs above):

- **`certificateThumbprint`** — thumbprint of a cert in the Windows certificate
  store. Set this when the build host has the cert installed (simplest, works on a
  self-hosted Windows runner with the cert imported). Currently `null`.
- **`digestAlgorithm`** — already set to `"sha256"`.
- **`timestampUrl`** — RFC 3161 timestamp server (e.g. the CA's timestamp URL), so
  signatures stay valid after the cert expires. Add this when signing.
- **`signCommand`** — a custom signing command, used for any signer that isn't the
  built-in `SignTool` thumbprint flow. This is the key used for Azure Trusted
  Signing (`trusted-signing-cli`), Azure Key Vault (`relic`), or any HSM-backed signer.

### Wiring secrets into the release workflow

The `tauri-action` build step already carries a real `env:` block (for
`APPIMAGE_EXTRACT_AND_RUN`), so adding signing is a matter of adding keys to it — do
**not** leave an `env:` containing only comments (an empty/comment-only `env:` is
invalid for Actions and previously caused a workflow startup failure). To enable
Windows signing later:

1. Add the relevant repository secrets (GitHub → Settings → Secrets and variables →
   Actions). For Azure Trusted Signing that's the Azure credentials the
   `signCommand` tool consumes (e.g. `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`,
   `AZURE_CLIENT_SECRET`, plus the signing account/profile names); for a
   thumbprint flow, import the cert into the runner's store and set the thumbprint.
2. Set `bundle.windows.signCommand` (or `certificateThumbprint`) + `timestampUrl`
   in `tauri.conf.json`.
3. Add those secrets as keys under the existing `tauri-action` `env:` block so they
   reach the build step.

Do not commit any certificate, thumbprint, or private key to the repo — everything
sensitive lives in Actions secrets.

## macOS & Linux signing posture

### macOS: UNSIGNED + un-notarized (Gatekeeper WILL block)

The macOS `.dmg` is **unsigned and un-notarized** — there is no Apple Developer
account ($99/yr) wired in. Consequences for end users:

- macOS **Gatekeeper blocks it**: opening the app shows *"ssanime-gui is damaged and
  can't be opened"* or *"unidentified developer"*. The user must **right-click → Open**
  (then confirm), or clear the quarantine attribute
  (`xattr -dr com.apple.quarantine /Applications/ssanime-gui.app`). On an Apple Silicon
  Mac an unsigned app can also be refused outright until ad-hoc re-signed.
- This is a hard friction point — more severe than Windows SmartScreen, which at least
  offers *More info → Run anyway*.

**Where signing/notarization would wire in.** `tauri-apps/tauri-action` reads these
from the build step's `env:` (verified against the Tauri v2 macOS signing docs,
<https://v2.tauri.app/distribute/sign/macos/>). With an Apple Developer account you would:

1. Add repository secrets: `APPLE_CERTIFICATE` (base64 of the `.p12`),
   `APPLE_CERTIFICATE_PASSWORD`, `APPLE_SIGNING_IDENTITY` (the "Developer ID
   Application: …" name); and for notarization either `APPLE_API_ISSUER` +
   `APPLE_API_KEY` + `APPLE_API_KEY_PATH` (App Store Connect API key) **or**
   `APPLE_ID` + `APPLE_PASSWORD` (app-specific password) + `APPLE_TEAM_ID`.
2. Add those names as keys under the existing `tauri-action` `env:` block on the macOS
   leg. tauri-action signs with the Developer ID cert and notarizes + staples the
   `.dmg` automatically when these are present.

No notarization is attempted here (no account); the workflow ships the unsigned `.dmg`
and this caveat documents the fix.

### Linux: no signing blocker

Linux has **no Gatekeeper/SmartScreen equivalent**. The `.deb`/`.rpm`/`.AppImage` are
unsigned but install and run without an OS trust prompt (a `.deb` may warn only if a
repo is configured to require signed packages, which a direct download is not). Nothing
to wire in for v1.
