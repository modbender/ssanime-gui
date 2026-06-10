# ssanime-gui

Local anime **download → encode → archive** manager. Downloads via torrents — an embedded
`anacrolix/torrent` client or an external qBittorrent/Transmission backend — re-encodes to x265
with ffmpeg, and organises the library in a Jellyfin-compatible layout. A yt-dlp / direct-link
downloader is planned behind the `Downloader` interface but not wired in v1. Runs as a background
daemon with a system-tray icon; the UI is a Svelte SPA served over `http://127.0.0.1:4773/`.

## Requirements

- Go 1.25+
- [Bun](https://bun.sh) (for the Svelte frontend)
- [Mage](https://magefile.org) (`go install github.com/magefile/mage@latest`) — drives the build targets; run `mage -l` to list them

ffmpeg and ffprobe are **auto-downloaded on first run** into `{DataDir}/bin/` — you do not
need to install them separately. (yt-dlp is also provisioned for the planned direct-link
downloader, which is not yet active in v1.)

## Build

All build targets are driven by [Mage](https://magefile.org). Run `mage -l` to list them.

### Daemon (standalone, browser-served)

```sh
mage frontend   # build the Svelte SPA -> internal/server/dist (embedded via go:embed)
mage server     # build the host-OS daemon (Windows -> ssanime.exe, no console window)
```

`mage server` builds for the current OS: cgo-free on Windows/Linux, with CGO for the
AppKit systray on macOS. The manual equivalent on Windows is
`go build -ldflags "-H=windowsgui -s -w" -o ssanime.exe ./cmd/ssanime`.

### All platforms

```sh
mage buildAll   # ssanime-windows-amd64.exe, ssanime-linux-amd64, ssanime-darwin-arm64
```

macOS is built only when running natively on a Mac — `fyne.io/systray` uses the AppKit
Objective-C bridge (CGO), which can't be cross-compiled without a cross-CGO toolchain, so
`buildAll` skips it on Windows/Linux.

### Desktop app (Tauri)

The `desktop/` directory contains a Tauri v2 shell — like Seanime's "Denshi" — that wraps
the Go daemon in a native window. It bundles `ssanime.exe` as a Tauri **sidecar**, spawns it
on startup with `--no-open`, waits for the daemon to bind `127.0.0.1:4773`, then opens a
`WebviewWindow` pointed at `http://127.0.0.1:4773/`. On exit the sidecar is killed.

**Prerequisites:**

- Rust 1.75+ with the `x86_64-pc-windows-msvc` target
- [WebView2 runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)
  (pre-installed on Windows 11; the NSIS installer bundles a bootstrapper for older systems)
- [Bun](https://bun.sh) (used to invoke the Tauri CLI via `bunx`)

**Build:**

```sh
mage tauri
# Runs, in order: mage frontend -> mage sidecar -> `cd desktop && bunx @tauri-apps/cli@latest build`
```

Artifacts land in `desktop/target/release/bundle/` (the cargo workspace root is `desktop/`):
- `nsis/ssanime-gui_<version>_x64-setup.exe` — NSIS installer
- `msi/ssanime-gui_<version>_x64_en-US.msi` — MSI installer (if WiX is available)

**Both artifacts ship:** the headless `ssanime.exe` remains fully functional standalone
(daemon + systray, browser as the UI). The Tauri `.exe` adds a native window on top.

For cutting tagged releases (the `v*` GitHub Actions workflow) and the Windows
code-signing posture, see [docs/distribution.md](docs/distribution.md).

## Running

```sh
./ssanime.exe              # opens browser to http://127.0.0.1:4773/ automatically
./ssanime.exe --no-open    # start without opening the browser
```

The process keeps running when you close the browser tab. Use the **system-tray icon**
(bottom-right on Windows) to:

- **Open UI** — re-open the browser tab
- **Pause all** / **Resume all** — suspend or resume the download and encode queues
  (in-flight jobs finish before the next one is blocked)
- **Quit** — graceful shutdown (HTTP server → encode queue → download queue → store)

Ctrl-C in a console build also triggers graceful shutdown.

## Logs

Logs are written to `{DataDir}/ssanime.log` on every build. On a console build they also
appear on stdout. `DataDir` is:

- **Windows**: `%APPDATA%\ssanime-gui\`
- **Linux/macOS**: `$HOME/.config/ssanime-gui/` (or `$XDG_CONFIG_HOME/ssanime-gui/`)

## Development

```sh
mage test               # run all tests (go test ./...)
mage vet                # static analysis (go vet ./...)
```

## License

GPL-3.0
