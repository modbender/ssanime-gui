# ssanime-gui

Local anime **download → encode → archive** manager. Downloads via torrents (embedded
`anacrolix/torrent`) or yt-dlp, re-encodes to x265 with ffmpeg, and organises the library
in a Jellyfin-compatible layout. Runs as a background daemon with a system-tray icon; the
UI is a Svelte SPA served over `http://127.0.0.1:4773/`.

## Requirements

- Go 1.25+
- [Bun](https://bun.sh) (for the Svelte frontend)
- `task` (optional, for the Taskfile targets) or use `build.ps1` directly on Windows

ffmpeg, ffprobe, and yt-dlp are **auto-downloaded on first run** into `{DataDir}/bin/`.
You do not need to install them separately.

## Build

### Frontend (required before building the Go binary)

```sh
cd frontend
bun install
bun run build     # produces frontend/dist/  (embedded via go:embed)
```

### Windows — no console window (production)

```powershell
go build -ldflags "-H=windowsgui -s -w" -o ssanime.exe ./cmd/ssanime
# or:
.\build.ps1
```

### Windows — with console (debugging / log to stdout)

```powershell
go build -o ssanime.exe ./cmd/ssanime
# or:
.\build.ps1 -Target windows-console
```

### Linux amd64

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags "-s -w" -o ssanime-linux-amd64 ./cmd/ssanime
```

### macOS amd64 (must build natively on a Mac)

`fyne.io/systray` on macOS uses the AppKit Objective-C bridge, which requires CGO.
Cross-compiling from Windows/Linux is not supported without a cross-CGO toolchain.

```sh
# On a Mac:
go build -ldflags "-s -w" -o ssanime-darwin-amd64 ./cmd/ssanime
```

### All platforms (via Taskfile)

```sh
task build-all
```

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
task build-desktop
# Equivalent manual steps:
#   go build -ldflags "-s -w" -o desktop/src-tauri/binaries/ssanime-x86_64-pc-windows-msvc.exe ./cmd/ssanime
#   cd desktop && bunx @tauri-apps/cli@latest build
```

Artifacts land in `desktop/src-tauri/target/release/bundle/`:
- `nsis/ssanime-gui_<version>_x64-setup.exe` — NSIS installer
- `msi/ssanime-gui_<version>_x64_en-US.msi` — MSI installer (if WiX is available)

**Both artifacts ship:** the headless `ssanime.exe` remains fully functional standalone
(daemon + systray, browser as the UI). The Tauri `.exe` adds a native window on top.

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
go test ./...           # run all tests (91 as of Phase 9)
go vet ./...            # static analysis
task test               # same via Taskfile
```

## License

GPL-3.0
