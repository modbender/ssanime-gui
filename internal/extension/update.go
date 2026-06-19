package extension

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/modbender/ssanime-gui/internal/defaults"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

// autoUpdateInterval is how often the background updater re-checks every repo
// index for newer versions of installed extensions. Source extensions break as
// the sites they scrape change (the same churn that breaks yt-dlp), so they are
// refreshed silently on a fixed cadence with no user action and no DB setting.
var autoUpdateInterval = defaults.Values.Extensions.AutoUpdateInterval()

// autoUpdateFirstDelay holds the first auto-update pass off until shortly after
// boot so startup (migrations, extension load, binary provisioning) isn't
// competing for the network. It never blocks startup.
var autoUpdateFirstDelay = defaults.Values.Extensions.AutoUpdateFirstDelay()

// shouldUpdateExtension decides whether an installed extension at version
// `installed` should be replaced by the repo index's `index` version. The repo
// is authoritative, but an empty/missing index version is never a valid update
// target (guards a malformed index from blanking a working extension). When both
// parse as semver the index must be strictly greater (no downgrades); otherwise
// any string inequality is treated as an update, since opaque revision tags
// ("rev-a") still signal the repo shipped something new.
func shouldUpdateExtension(installed, index string) bool {
	if index == "" {
		return false
	}
	if installed == "" {
		return true
	}
	if iv, ok1 := parseSemver(installed); ok1 {
		if xv, ok2 := parseSemver(index); ok2 {
			return compareSemver(xv, iv) > 0
		}
	}
	return installed != index
}

// parseSemver parses a dot-separated numeric version (optionally "v"-prefixed,
// with a "-pre"/"+build" suffix ignored) into its numeric components. ok is
// false when any leading component is non-numeric, so non-semver tags fall back
// to string comparison in shouldUpdateExtension.
func parseSemver(s string) ([]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return nil, false
	}
	parts := strings.Split(s, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
}

// compareSemver returns >0 if a>b, <0 if a<b, 0 if equal, comparing component by
// component and treating a missing trailing component as zero (1.2 == 1.2.0).
func compareSemver(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			return av - bv
		}
	}
	return 0
}

// UpdatedExtension is one entry in the SSE payload broadcast when extensions are
// auto-updated.
type UpdatedExtension struct {
	ExtID   string `json:"ext_id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AutoUpdateAll runs one auto-update pass over every repo. For each repo it
// fetches the index (best-effort: a fetch failure logs and skips only that repo)
// and, for each INSTALLED extension belonging to the repo, finds the matching
// index entry by ext_id and updates it when shouldUpdateExtension says so. On any
// update it broadcasts a single extensions.updated SSE event listing the changes.
// Never blocks on, or fails because of, an unreachable repo.
func (m *Manager) AutoUpdateAll(ctx context.Context) {
	repos, err := m.ListRepos(ctx)
	if err != nil {
		m.logger.Warn("extension: auto-update list repos failed", "err", err)
		return
	}

	var updated []UpdatedExtension
	for _, repo := range repos {
		if ctx.Err() != nil {
			return
		}
		updated = append(updated, m.autoUpdateRepo(ctx, repo)...)
	}

	if len(updated) == 0 {
		return
	}
	m.logger.Info("extension: auto-update applied", "count", len(updated))
	if m.hub != nil {
		m.hub.Broadcast(events.TypeExtensionsUpdated, map[string]any{
			"count":      len(updated),
			"extensions": updated,
		})
	}
}

// autoUpdateRepo updates the installed extensions of a single repo and returns
// the ones it actually changed. A repo whose index can't be fetched is skipped
// (logged), never aborting the whole pass.
func (m *Manager) autoUpdateRepo(ctx context.Context, repo store.ExtensionRepo) []UpdatedExtension {
	entries, err := m.FetchRepoIndex(ctx, repo.Url)
	if err != nil {
		m.logger.Warn("extension: auto-update fetch index failed (skipping repo)", "repo", repo.Name, "err", err)
		return nil
	}
	byID := make(map[string]IndexEntry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}

	repoID := repo.ID
	rows, err := m.st.Read().ListExtensionsByRepo(ctx, &repoID)
	if err != nil {
		m.logger.Warn("extension: auto-update list installed failed", "repo", repo.Name, "err", err)
		return nil
	}

	var updated []UpdatedExtension
	for _, row := range rows {
		if ctx.Err() != nil {
			return updated
		}
		if row.IsBuiltin != 0 {
			continue
		}
		entry, ok := byID[row.ExtID]
		if !ok {
			continue
		}
		installedVer := ""
		if row.Version != nil {
			installedVer = *row.Version
		}
		if !shouldUpdateExtension(installedVer, entry.Version) {
			continue
		}
		if err := m.updateExtensionTo(ctx, entry, row); err != nil {
			m.logger.Warn("extension: auto-update apply failed", "id", row.ExtID, "err", err)
			continue
		}
		updated = append(updated, UpdatedExtension{ExtID: entry.ID, Name: entry.Name, Version: entry.Version})
	}
	return updated
}

// updateExtensionTo applies the index entry to an already-installed row WITHOUT
// going through InstallExtension, which would enable/register unconditionally and
// recompute settings from defaults only. It preserves three things the naive
// upsert would clobber:
//
//   - enabled state: the row's Enabled is never touched; the live provider is
//     re-registered only when the row was already enabled (a disabled extension
//     stays disabled and unregistered).
//   - user settings: new schema defaults are merged UNDER the stored settings via
//     resolveSettings(entry.Options, existing) so user overrides win.
//   - row identity: UpdateExtensionPayload updates by id and keeps uuid/enabled,
//     so the row is not re-minted.
func (m *Manager) updateExtensionTo(ctx context.Context, entry IndexEntry, existing store.Extension) error {
	payload, err := m.downloadPayload(ctx, entry.Code)
	if err != nil {
		return err
	}

	version := entry.Version
	sourceURL := entry.Code
	var icon *string
	if entry.Icon != "" {
		icon = &entry.Icon
	}

	// Merge the new schema defaults under the existing stored settings so a newly
	// added option gains its default while every existing user override is kept.
	var settingsJSON *string
	if merged := resolveSettings(entry.Options, existing.Settings); len(merged) > 0 {
		if b, mErr := json.Marshal(merged); mErr == nil {
			s := string(b)
			settingsJSON = &s
		}
	} else {
		settingsJSON = existing.Settings
	}

	row, err := m.st.Write().UpdateExtensionPayload(ctx, store.UpdateExtensionPayloadParams{
		Name:      entry.Name,
		Version:   &version,
		SourceUrl: &sourceURL,
		Payload:   &payload,
		Nsfw:      b2i(entry.NSFW),
		Icon:      icon,
		Settings:  settingsJSON,
		ID:        existing.ID,
	})
	if err != nil {
		return err
	}

	// Refresh the live provider only for enabled extensions. The updated payload
	// replaces the registered provider in place; a disabled extension is left
	// unregistered.
	if row.Enabled != 0 {
		if err := m.registerProvider(row); err != nil {
			m.logger.Warn("extension: re-register after update failed", "id", row.ExtID, "err", err)
		}
	}
	m.logger.Info("extension updated", "id", entry.ID, "name", entry.Name, "version", version)
	return nil
}

// StartAutoUpdater launches the background auto-update loop: an initial pass
// shortly after boot (non-blocking) plus a fixed-interval ticker. It mirrors the
// poller/metadata lifecycle (Start/Stop, top-level recover per pass). Stop ends
// it and waits for the goroutine to exit.
func (m *Manager) StartAutoUpdater() {
	m.updaterMu.Lock()
	defer m.updaterMu.Unlock()
	if m.updaterStarted {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.updaterCancel = cancel
	m.updaterDone = make(chan struct{})
	m.updaterStarted = true
	go m.autoUpdateLoop(ctx)
}

// StopAutoUpdater ends the auto-update loop and waits for it to exit. Idempotent.
func (m *Manager) StopAutoUpdater() {
	m.updaterMu.Lock()
	cancel, done := m.updaterCancel, m.updaterDone
	m.updaterStarted = false
	m.updaterMu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (m *Manager) autoUpdateLoop(ctx context.Context) {
	defer close(m.updaterDone)

	select {
	case <-ctx.Done():
		return
	case <-time.After(autoUpdateFirstDelay):
	}
	m.runAutoUpdatePass(ctx)

	ticker := time.NewTicker(autoUpdateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runAutoUpdatePass(ctx)
		}
	}
}

// runAutoUpdatePass wraps one pass in a recover so a panic is isolated to the
// pass (the loop and daemon survive).
func (m *Manager) runAutoUpdatePass(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			m.logger.Error("extension: auto-update recovered from panic", "panic", rec)
		}
	}()
	m.AutoUpdateAll(ctx)
}
