# Distribution & code-signing

How ssanime-gui is packaged and shipped, and the honest current state of Windows
code signing plus the concrete path to fixing it.

## What ships

The desktop deliverable is a Tauri v2 shell (`desktop/`) that wraps the Go daemon
as a [sidecar](https://v2.tauri.app/develop/sidecar/). On Windows the bundler emits
two installers under `desktop/src-tauri/target/release/bundle/`:

- `nsis/ssanime-gui_<version>_x64-setup.exe` — NSIS installer
- `msi/ssanime-gui_<version>_x64_en-US.msi` — MSI installer (needs WiX on the build host)

`bundle.targets` is `"all"`, so both are produced. The headless `ssanime.exe`
(daemon + systray + browser UI) remains fully usable on its own; the Tauri app
only adds a native window.

Windows is the only desktop target for v1. macOS systray uses the AppKit
Objective-C bridge and needs CGO + a Mac runner, so no macOS/Linux desktop
bundles are built (the release workflow is a single `windows-latest` job).

## The build order is load-bearing

The Go daemon `go:embed`s `internal/server/dist`. The release **must** build in
this exact order, or the sidecar ships without the SPA:

1. **Frontend** — `bun install && bun run build` in `frontend/`, which emits the
   compiled SPA to `internal/server/dist`.
2. **Sidecar** — compile the Go daemon (`-ldflags "-H=windowsgui -s -w"`,
   `GOOS=windows GOARCH=amd64`) to
   `desktop/src-tauri/binaries/ssanime-x86_64-pc-windows-msvc.exe`. The SPA is now
   embedded in the binary. The filename is the Tauri sidecar target-triple name —
   Tauri resolves `externalBin: ["binaries/ssanime"]` to this triple-suffixed file.
3. **Tauri build** — `bunx @tauri-apps/cli@latest build` in `desktop/`, producing
   the NSIS + MSI installers.

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
   (`if: release_created == 'true'`) build the frontend → sidecar → Tauri
   installers and **upload the NSIS `.exe` + MSI `.msi` to the release
   release-please just created** (via `gh release upload --clobber`). Ordinary
   commits never trigger an installer build — exactly one build per release.

> The single source of truth for the version is `tauri.conf.json`'s `version`
> field, kept in sync with the git tag by release-please. There is no separate Go
> version constant.

### Required repo setting (one-time)

release-please opens its Release PR using the default `GITHUB_TOKEN`. For that to
work, **Settings → Actions → General → Workflow permissions** must have
**"Allow GitHub Actions to create and approve pull requests"** turned **ON**.
Without it, the `release-please` job fails to open the PR.

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
task build-desktop
```

This runs the same ordering locally: `build-sidecar` (which assumes the frontend
was already built via `task frontend`) → `bunx @tauri-apps/cli@latest build`.
Artifacts land in `desktop/src-tauri/target/release/bundle/`.

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

`.github/workflows/release.yml` has a commented placeholder in the `tauri-action`
`env:` block marking exactly where signing secrets go. To enable signing later
**without rewriting the workflow**:

1. Add the relevant repository secrets (GitHub → Settings → Secrets and variables →
   Actions). For Azure Trusted Signing that's the Azure credentials the
   `signCommand` tool consumes (e.g. `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`,
   `AZURE_CLIENT_SECRET`, plus the signing account/profile names); for a
   thumbprint flow, import the cert into the runner's store and set the thumbprint.
2. Set `bundle.windows.signCommand` (or `certificateThumbprint`) + `timestampUrl`
   in `tauri.conf.json`.
3. Uncomment / extend the signing `env:` in the workflow so those secrets reach the
   build step.

Do not commit any certificate, thumbprint, or private key to the repo — everything
sensitive lives in Actions secrets.
