//go:build mage

// Mage build targets for ssanime-gui. Run "mage -l" to list targets and
// "mage <target>" to invoke one (e.g. mage tauri). Mage compiles this file to a
// Go binary, so the build logic is plain Go — no shell-string portability quirks.
//
// Two artifacts ship from this repo:
//   - the standalone daemon (mage server) — binds HTTP on localhost and opens
//     the browser to the embedded SPA;
//   - the Tauri desktop app (mage tauri) — a native window wrapping that same
//     daemon as a sidecar (no browser).
package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Default target when `mage` is run with no arguments.
var Default = Server

const (
	cmdPkg     = "./cmd/ssanime"
	binBase    = "ssanime"
	sidecarDir = "desktop/src-tauri/binaries"
)

// The systray binds AppKit via cgo on macOS only; Windows & Linux use the
// pure-Go systray path, so they build cgo-free.
func hostBuildEnv() map[string]string {
	cgo := "0"
	if runtime.GOOS == "darwin" {
		cgo = "1"
	}
	return map[string]string{"CGO_ENABLED": cgo}
}

// -s strips the symbol table, -w omits DWARF; -H=windowsgui hides the console
// window so the daemon/sidecar doesn't pop a terminal on Windows.
func hostLDFlags() string {
	if runtime.GOOS == "windows" {
		return "-H=windowsgui -s -w"
	}
	return "-s -w"
}

func daemonOut() string {
	if runtime.GOOS == "windows" {
		return binBase + ".exe"
	}
	return binBase
}

// Tauri appends the Rust host target-triple to externalBin names, so the sidecar
// must be written as `ssanime-<triple>[.exe]` for `tauri build` to find it.
func rustTriple(goos, goarch string) string {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	switch goos {
	case "windows":
		return fmt.Sprintf("%s-pc-windows-msvc", arch)
	case "darwin":
		return fmt.Sprintf("%s-apple-darwin", arch)
	default:
		return fmt.Sprintf("%s-unknown-linux-gnu", arch)
	}
}

func goBuild(env map[string]string, ldflags, out string) error {
	return sh.RunWithV(env, "go", "build", "-ldflags", ldflags, "-o", out, cmdPkg)
}

// inDir runs fn with the working directory temporarily changed to dir. Mage runs
// targets serially, so the global chdir is safe.
func inDir(dir string, fn func() error) error {
	cur, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	defer os.Chdir(cur)
	return fn()
}

// Server builds the standalone daemon for the host OS (browser-served UI).
// On Windows -> ssanime.exe (no console window).
func Server() error {
	fmt.Println("building daemon for", runtime.GOOS+"/"+runtime.GOARCH, "->", daemonOut())
	return goBuild(hostBuildEnv(), hostLDFlags(), daemonOut())
}

// Frontend builds the Svelte SPA. Its Vite config emits to internal/server/dist,
// which the Go daemon go:embeds — so this must run before Server/Sidecar to pick
// up UI changes.
func Frontend() error {
	fmt.Println("building frontend (Svelte -> internal/server/dist)")
	return inDir("frontend", func() error {
		if err := sh.RunV("bun", "install", "--frozen-lockfile"); err != nil {
			return err
		}
		return sh.RunV("bun", "run", "build")
	})
}

// Sidecar builds the Go daemon as the Tauri sidecar for the host triple, written
// into desktop/src-tauri/binaries/ where the Tauri bundler picks it up.
func Sidecar() error {
	triple := rustTriple(runtime.GOOS, runtime.GOARCH)
	out := fmt.Sprintf("%s/%s-%s", sidecarDir, binBase, triple)
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	fmt.Println("building sidecar ->", out)
	return goBuild(hostBuildEnv(), hostLDFlags(), out)
}

// Tauri builds the native desktop app. Depends on Frontend (embedded SPA) and
// Sidecar (the bundled daemon), then runs the Tauri bundler. Installers land in
// desktop/target/release/bundle/ (the cargo workspace root is desktop/, so the
// target dir is desktop/target — NOT desktop/src-tauri/target).
func Tauri() error {
	mg.SerialDeps(Frontend, Sidecar)
	fmt.Println("bundling Tauri desktop app")
	return inDir("desktop", func() error {
		return sh.RunV("bunx", "@tauri-apps/cli@latest", "build")
	})
}

// Run builds the daemon and launches it.
func Run() error {
	mg.Deps(Server)
	return sh.RunV("./" + daemonOut())
}

// BuildAll cross-compiles the daemon binary for Windows, Linux, and macOS.
// macOS needs cgo for the AppKit systray and cannot be cross-compiled without a
// cross-cgo toolchain, so it is built only when running on a Mac; otherwise it
// is skipped with a notice.
func BuildAll() error {
	mg.Deps(Frontend)
	type plat struct {
		goos, goarch, ld, out string
		cgo                   string
	}
	plats := []plat{
		{"windows", "amd64", "-H=windowsgui -s -w", binBase + "-windows-amd64.exe", "0"},
		{"linux", "amd64", "-s -w", binBase + "-linux-amd64", "0"},
		{"darwin", "arm64", "-s -w", binBase + "-darwin-arm64", "1"},
	}
	for _, p := range plats {
		if p.goos == "darwin" && runtime.GOOS != "darwin" {
			fmt.Println("skipping darwin (cgo systray can't cross-compile; build on a Mac)")
			continue
		}
		fmt.Println("building", p.out)
		env := map[string]string{"GOOS": p.goos, "GOARCH": p.goarch, "CGO_ENABLED": p.cgo}
		if err := goBuild(env, p.ld, p.out); err != nil {
			return err
		}
	}
	return nil
}

// Test runs the full Go test suite.
func Test() error { return sh.RunV("go", "test", "./...") }

// Vet runs go vet across all packages.
func Vet() error { return sh.RunV("go", "vet", "./...") }

// Clean removes built binaries (daemon, cross builds, and the Tauri sidecar).
func Clean() error {
	paths := []string{
		binBase, binBase + ".exe",
		binBase + "-windows-amd64.exe",
		binBase + "-linux-amd64",
		binBase + "-darwin-arm64",
	}
	for _, p := range paths {
		_ = os.Remove(p)
	}
	return os.RemoveAll(sidecarDir)
}
