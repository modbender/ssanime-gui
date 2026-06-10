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

The release workflow (`.github/workflows/release.yml`) runs steps 1 and 2 as
explicit steps before handing off to `tauri-apps/tauri-action` for step 3, so each
build phase is independently visible in the logs.

## Cutting a release

1. Bump `version` in `desktop/src-tauri/tauri.conf.json` (e.g. `0.1.0` → `0.2.0`).
   tauri-action derives the installer filenames from this field, **not** from the
   git tag — they must agree, so bump here before tagging.
2. Commit the bump.
3. Tag and push:
   ```sh
   git tag v0.2.0
   git push origin v0.2.0
   ```
4. The `Release` workflow runs on the `v*` tag: it builds the frontend, the
   sidecar, and the Tauri installers, then opens a **draft** GitHub Release with
   the NSIS `.exe` and MSI `.msi` attached.
5. Review the draft and its assets, then publish it manually.

The draft step is deliberate — nothing goes public without a human reviewing the
artifacts first.

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
