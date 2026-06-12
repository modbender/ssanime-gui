package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/modbender/ssanime-gui/internal/store"
)

// revealBuilder builds the argv for one GOOS's native file-explorer "reveal"
// command. path is an absolute, cleaned file path.
type revealBuilder func(path string) []string

// revealBuilders maps GOOS to its file-manager invocation. Adding an OS is one
// map entry. Each selects/opens the file's containing folder:
//   - windows: explorer /select,<path>  (selects the file; explorer.exe exits 1
//     even on success, so the caller ignores a non-zero exit)
//   - darwin:  open -R <path>           (reveal-and-select in Finder)
//   - linux:   xdg-open <dir>           (opens the containing dir; no portable
//     file-select across desktop environments)
var revealBuilders = map[string]revealBuilder{
	"windows": func(path string) []string {
		// explorer parses "/select,<path>" as a single token; spawned as
		// separate argv elements it still selects correctly.
		return []string{"explorer", "/select," + path}
	},
	"darwin": func(path string) []string {
		return []string{"open", "-R", path}
	},
	"linux": func(path string) []string {
		return []string{"xdg-open", filepath.Dir(path)}
	},
}

// revealArgv builds the OS-native reveal argv for goos and an absolute path.
// Pure (no exec) so it is unit-testable per-GOOS. Returns an error for an
// unsupported GOOS.
func revealArgv(goos, path string) ([]string, error) {
	b, ok := revealBuilders[goos]
	if !ok {
		return nil, fmt.Errorf("reveal not supported on %s", goos)
	}
	return b(path), nil
}

// revealPath validates that stored points under root, that the file still
// exists, then launches the OS file explorer at it (fire-and-forget). It writes
// the HTTP response: 204 on success, 404 if stored is empty, 403 if it escapes
// root, 409 if the file is gone, 500/501 on internal/launch failure.
func (h *Handler) revealPath(w http.ResponseWriter, stored, root string) {
	if strings.TrimSpace(stored) == "" {
		WriteError(w, http.StatusNotFound, "no path on record")
		return
	}
	abs, err := filepath.Abs(filepath.Clean(stored))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "invalid path")
		return
	}
	if !pathUnderRoot(abs, root) {
		WriteError(w, http.StatusForbidden, "path outside allowed root")
		return
	}
	if _, err := os.Stat(abs); errors.Is(err, os.ErrNotExist) {
		WriteError(w, http.StatusConflict, "file no longer exists")
		return
	} else if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to stat file")
		return
	}

	argv, err := revealArgv(runtime.GOOS, abs)
	if err != nil {
		WriteError(w, http.StatusNotImplemented, err.Error())
		return
	}
	// Fire-and-forget: don't block the request on the file manager. explorer.exe
	// returns exit 1 even on success, so a non-zero exit is not treated as error.
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		h.logger.Warn("reveal: launch file explorer", "argv", argv, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to open file explorer")
		return
	}
	go func() { _ = cmd.Wait() }()

	w.WriteHeader(http.StatusNoContent)
}

// pathUnderRoot reports whether abs resolves inside root. root is cleaned and
// abs'd; the comparison is prefix-based on a path-separator boundary so
// "/a/rootX" does not count as under "/a/root". Empty root denies everything.
func pathUnderRoot(abs, root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(cleanRoot, abs)
	if err != nil {
		return false
	}
	// rel == "." means abs == root; a leading ".." escapes the root.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// handleRevealEpisodeSource opens the OS file explorer at an episode's
// downloaded source file, guarded to the configured download_root.
func (h *Handler) handleRevealEpisodeSource(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ep, err := h.store.Read().GetEpisode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "episode not found")
		return
	}
	if err != nil {
		h.logger.Error("reveal episode: get", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to load episode")
		return
	}
	root, ok := h.settingsRoot(ctx, w, func(s store.Setting) string { return s.DownloadRoot })
	if !ok {
		return
	}
	stored := ""
	if ep.SourcePath != nil {
		stored = *ep.SourcePath
	}
	h.revealPath(w, stored, root)
}

// handleRevealOutput opens the OS file explorer at an encoded output's file,
// guarded to the configured encoded_root.
func (h *Handler) handleRevealOutput(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	out, err := h.store.Read().GetEncodedOutput(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "output not found")
		return
	}
	if err != nil {
		h.logger.Error("reveal output: get", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to load output")
		return
	}
	root, ok := h.settingsRoot(ctx, w, func(s store.Setting) string { return s.EncodedRoot })
	if !ok {
		return
	}
	stored := ""
	if out.EncodedPath != nil {
		stored = *out.EncodedPath
	}
	h.revealPath(w, stored, root)
}

// settingsRoot loads settings and returns one root via pick. On a store error it
// writes a 500 and returns ok=false.
func (h *Handler) settingsRoot(ctx context.Context, w http.ResponseWriter, pick func(store.Setting) string) (string, bool) {
	set, err := h.store.Read().GetSettings(ctx)
	if err != nil {
		h.logger.Error("reveal: load settings", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to load settings")
		return "", false
	}
	return pick(set), true
}
