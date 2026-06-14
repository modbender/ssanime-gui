package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// ErrNoIcon signals that an extension has no usable icon: its Icon field is
// empty, or the upstream fetch did not yield an image. Handlers map it to 404.
var ErrNoIcon = errors.New("extension: no icon")

// Manager owns the lifecycle of JS extensions: fetching repo indexes,
// installing extensions into the DB, loading enabled extensions into
// source.Registry on boot.
type Manager struct {
	st       *store.Store
	registry *source.Registry
	// httpClient is used for fetching repo index.json and JS payloads.
	// Use the DoH-backed client so nyaa-related extension requests work.
	httpClient *http.Client
	// dataDir is the app-data directory; JS payloads are cached there.
	dataDir string
	logger  *slog.Logger
	// resolver fills cross-tracker ids for the Hayase options object. Nil until
	// SetResolver is called; nil-safe (providers fall back to Media ids).
	resolver IDResolver
}

// SetResolver wires the ani.zip id-resolver used to populate the Hayase options
// object for every JS provider this manager registers. Call before
// LoadAndRegisterAll so boot-loaded providers get it.
func (m *Manager) SetResolver(r IDResolver) { m.resolver = r }

// b2i maps a Go bool to the SQLite integer sqlc uses for boolean columns.
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// NewManager builds a Manager. httpClient should be the DoH-backed client.
func NewManager(st *store.Store, registry *source.Registry, httpClient *http.Client, dataDir string, logger *slog.Logger) *Manager {
	return &Manager{
		st:         st,
		registry:   registry,
		httpClient: httpClient,
		dataDir:    dataDir,
		logger:     logger,
	}
}

// ExtensionsDir returns the directory where JS payload files are cached.
func (m *Manager) ExtensionsDir() string {
	return filepath.Join(m.dataDir, "extensions")
}

// FetchRepoIndex downloads a repo's index.json and returns the available extensions.
func (m *Manager) FetchRepoIndex(ctx context.Context, indexURL string) ([]IndexEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("extension repo: build request: %w", err)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extension repo %s: fetch: %w", indexURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("extension repo %s: HTTP %d", indexURL, resp.StatusCode)
	}

	var entries []IndexEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("extension repo %s: decode: %w", indexURL, err)
	}
	return entries, nil
}

// AddRepo persists a new extension repo to the DB (enabled by default).
func (m *Manager) AddRepo(ctx context.Context, name, url string) (store.ExtensionRepo, error) {
	return m.st.Write().CreateExtensionRepo(ctx, store.CreateExtensionRepoParams{
		Uuid:    uuid.NewString(),
		Name:    name,
		Url:     url,
		Enabled: 1,
	})
}

// ListRepos returns all known extension repos.
func (m *Manager) ListRepos(ctx context.Context) ([]store.ExtensionRepo, error) {
	return m.st.Read().ListExtensionRepos(ctx)
}

// InstallExtension downloads a JS extension from the repo index entry and
// upserts it into the extensions table. It is enabled by default.
func (m *Manager) InstallExtension(ctx context.Context, entry IndexEntry, repoID int64) (store.Extension, error) {
	payload, err := m.downloadPayload(ctx, entry.Code)
	if err != nil {
		return store.Extension{}, fmt.Errorf("install %s: download payload: %w", entry.ID, err)
	}

	version := entry.Version
	if version == "" {
		version = "0.0.1"
	}
	sourceURL := entry.Code
	var icon *string
	if entry.Icon != "" {
		icon = &entry.Icon
	}

	// Persist the resolved settings defaults (from the index.json options schema)
	// so later boot loads — which only see the DB row, not the schema — recover
	// the same flat key:value map via resolveSettings(nil, row.Settings).
	var settingsJSON *string
	if defaults := resolveSettings(entry.Options, nil); len(defaults) > 0 {
		if b, mErr := json.Marshal(defaults); mErr == nil {
			s := string(b)
			settingsJSON = &s
		}
	}

	ext, err := m.st.Write().UpsertExtensionByExtID(ctx, store.UpsertExtensionByExtIDParams{
		Uuid:      uuid.NewString(),
		RepoID:    &repoID,
		ExtID:     entry.ID,
		Name:      entry.Name,
		Type:      ExtTypeTorrent,
		Lang:      "javascript",
		Version:   &version,
		SourceUrl: &sourceURL,
		Payload:   &payload,
		Enabled:   1,
		IsBuiltin: 0,
		Settings:  settingsJSON,
		Nsfw:      b2i(entry.NSFW),
		Icon:      icon,
	})
	if err != nil {
		return store.Extension{}, fmt.Errorf("install %s: upsert: %w", entry.ID, err)
	}
	if err := m.registerProvider(ext); err != nil {
		m.logger.Warn("extension: register after install failed", "id", entry.ID, "err", err)
	}
	m.logger.Info("extension installed", "id", entry.ID, "name", entry.Name, "version", version)
	return ext, nil
}

// healthRecorder returns the centralized health-recording hook injected into
// every provider this manager builds for an INSTALLED extension. It persists the
// run outcome to the single per-ext_id health record using a short INDEPENDENT
// context — never the caller's search context — so health is written even when
// the user navigated away and cancelled the request mid-search. This is the one
// choke point through which add-time previews, the re-check endpoint, and real
// runtime search failures all funnel into one persisted health state.
func (m *Manager) healthRecorder() func(extID string, healthy bool, errMsg string) {
	return func(extID string, healthy bool, errMsg string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var healthyVal *int64
		v := b2i(healthy)
		healthyVal = &v
		var errPtr *string
		if errMsg != "" {
			errPtr = &errMsg
		}
		if err := m.st.Write().UpdateExtensionHealth(ctx, store.UpdateExtensionHealthParams{
			Healthy:     healthyVal,
			HealthError: errPtr,
			ExtID:       extID,
		}); err != nil {
			m.logger.Warn("extension: record health failed", "id", extID, "err", err)
		}
	}
}

// buildInstalledProvider compiles a stored extension's JS payload into a
// JSProvider wired with the manager's resolver, resolved settings, and the
// centralized health recorder. Used by every path that loads an INSTALLED
// extension (boot load, install, enable) so each gets the recorder.
func (m *Manager) buildInstalledProvider(row store.Extension) (*JSProvider, error) {
	if row.Payload == nil || *row.Payload == "" {
		return nil, fmt.Errorf("extension %s: no payload", row.ExtID)
	}
	settings := resolveSettings(nil, row.Settings)
	p, err := NewJSProviderWithDeps(row.ExtID, row.Name, *row.Payload, m.httpClient, m.resolver, settings, m.logger)
	if err != nil {
		return nil, err
	}
	p.SetHealthRecorder(m.healthRecorder())
	return p, nil
}

// registerProvider compiles a stored extension's JS payload into a JSProvider
// (with the centralized health recorder) and registers it into the source
// registry. No-op for rows without a payload.
func (m *Manager) registerProvider(row store.Extension) error {
	p, err := m.buildInstalledProvider(row)
	if err != nil {
		return err
	}
	m.registry.Register(p)
	return nil
}

// EnableExtension sets the enabled flag and registers the provider so the
// source becomes live without a restart.
func (m *Manager) EnableExtension(ctx context.Context, dbID int64) error {
	if err := m.st.Write().SetExtensionEnabled(ctx, store.SetExtensionEnabledParams{
		ID:      dbID,
		Enabled: 1,
	}); err != nil {
		return err
	}
	row, err := m.st.Read().GetExtension(ctx, dbID)
	if err != nil {
		m.logger.Warn("extension: load after enable failed", "id", dbID, "err", err)
		return nil
	}
	if err := m.registerProvider(row); err != nil {
		m.logger.Warn("extension: register after enable failed", "id", row.ExtID, "err", err)
	}
	return nil
}

// DisableExtension clears the enabled flag and unregisters the provider so the
// source stops serving immediately.
func (m *Manager) DisableExtension(ctx context.Context, dbID int64) error {
	if err := m.st.Write().SetExtensionEnabled(ctx, store.SetExtensionEnabledParams{
		ID:      dbID,
		Enabled: 0,
	}); err != nil {
		return err
	}
	if row, err := m.st.Read().GetExtension(ctx, dbID); err == nil {
		m.registry.Unregister(row.ExtID)
	}
	return nil
}

// UninstallExtension unregisters the provider and deletes its row (guarded to
// non-builtin rows by the DeleteExtension query).
func (m *Manager) UninstallExtension(ctx context.Context, dbID int64) error {
	row, err := m.st.Read().GetExtension(ctx, dbID)
	if err != nil {
		return err
	}
	m.registry.Unregister(row.ExtID)
	return m.st.Write().DeleteExtension(ctx, dbID)
}

// DeleteRepo unregisters and deletes every extension belonging to the repo,
// then deletes the repo row.
func (m *Manager) DeleteRepo(ctx context.Context, repoID int64) error {
	rows, err := m.st.Read().ListExtensionsByRepo(ctx, &repoID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		m.registry.Unregister(row.ExtID)
		_ = m.st.Write().DeleteExtension(ctx, row.ID)
	}
	return m.st.Write().DeleteExtensionRepo(ctx, repoID)
}

// liveProbeTimeout bounds a single extension liveness probe (test()/probe
// SmartSearch) during preview and sync.
const liveProbeTimeout = 15 * time.Second

// previewConcurrency caps how many extensions are probed in parallel during a
// repo preview so a large index can't open an unbounded number of upstream
// connections at once.
const previewConcurrency = 6

// PreviewEntry is one extension in a repo-preview result: its index metadata
// plus the liveness outcome. Usable is true when the extension loaded and its
// test()/probe ran without throwing; Error carries the failure reason otherwise.
type PreviewEntry struct {
	ExtID   string
	Name    string
	Version string
	Type    string
	NSFW    bool
	Usable  bool
	Error   string
}

// PreviewRepo fetches a repo index and liveness-checks every torrent extension
// it lists, concurrently and bounded, without installing anything. It builds a
// throwaway JSProvider per entry (no health recorder — nothing is persisted) and
// runs Test(). A single dead extension is reported as Usable:false, not an error.
// The whole call errors only when the index is unreachable, not valid JSON, or
// lists zero torrent extensions — the caller maps that to a 4xx.
func (m *Manager) PreviewRepo(ctx context.Context, indexURL string) ([]PreviewEntry, error) {
	entries, err := m.FetchRepoIndex(ctx, indexURL)
	if err != nil {
		return nil, fmt.Errorf("Repository unreachable or invalid: %w", err)
	}

	torrents := make([]IndexEntry, 0, len(entries))
	for _, e := range entries {
		if strings.EqualFold(e.Type, ExtTypeTorrent) {
			torrents = append(torrents, e)
		}
	}
	if len(torrents) == 0 {
		return nil, fmt.Errorf("Repository unreachable or invalid: no torrent extensions listed")
	}

	out := make([]PreviewEntry, len(torrents))
	sem := make(chan struct{}, previewConcurrency)
	var wg sync.WaitGroup
	for i, e := range torrents {
		wg.Add(1)
		go func(i int, e IndexEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = m.probeEntry(ctx, e)
		}(i, e)
	}
	wg.Wait()
	return out, nil
}

// probeEntry downloads an index entry's JS payload, builds a throwaway provider
// (no health recorder), and runs Test() under a bounded context. Any failure —
// payload fetch, compile, or a dead upstream — yields Usable:false with the
// reason. Pure liveness; nothing is persisted.
func (m *Manager) probeEntry(ctx context.Context, e IndexEntry) PreviewEntry {
	pe := PreviewEntry{
		ExtID:   e.ID,
		Name:    e.Name,
		Version: e.Version,
		Type:    e.Type,
		NSFW:    e.NSFW,
	}
	payload, err := m.downloadPayload(ctx, e.Code)
	if err != nil {
		pe.Error = err.Error()
		return pe
	}
	settings := resolveSettings(e.Options, nil)
	p, err := NewJSProviderWithDeps(e.ID, e.Name, payload, m.httpClient, m.resolver, settings, m.logger)
	if err != nil {
		pe.Error = err.Error()
		return pe
	}
	probeCtx, cancel := context.WithTimeout(ctx, liveProbeTimeout)
	defer cancel()
	if err := p.Test(probeCtx); err != nil {
		pe.Error = err.Error()
		return pe
	}
	pe.Usable = true
	return pe
}

// TestExtension loads an installed extension by DB id, runs its Test() under a
// bounded context, and persists the outcome to the centralized health record.
// It returns whether the extension is healthy and the failure reason (empty when
// healthy). The provider is built with no recorder here so the single persisted
// write is the explicit UpdateExtensionHealth below (rather than a duplicate via
// the recorder), keeping one canonical write per call.
func (m *Manager) TestExtension(ctx context.Context, dbID int64) (healthy bool, errMsg string, err error) {
	row, err := m.st.Read().GetExtension(ctx, dbID)
	if err != nil {
		return false, "", err
	}
	if row.Payload == nil || *row.Payload == "" {
		return false, "", fmt.Errorf("extension %s: no payload", row.ExtID)
	}
	settings := resolveSettings(nil, row.Settings)
	p, perr := NewJSProviderWithDeps(row.ExtID, row.Name, *row.Payload, m.httpClient, m.resolver, settings, m.logger)
	if perr != nil {
		errMsg = perr.Error()
	} else {
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		if terr := p.Test(probeCtx); terr != nil {
			errMsg = terr.Error()
		}
		cancel()
	}
	healthy = errMsg == ""

	if uerr := m.persistHealth(ctx, row.ExtID, healthy, errMsg); uerr != nil {
		m.logger.Warn("extension: persist health after test failed", "id", row.ExtID, "err", uerr)
	}
	return healthy, errMsg, nil
}

// persistHealth writes one health record for ext_id (health_checked_at defaults
// to now). Shared by the test endpoint and install-time health seeding.
func (m *Manager) persistHealth(ctx context.Context, extID string, healthy bool, errMsg string) error {
	v := b2i(healthy)
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	return m.st.Write().UpdateExtensionHealth(ctx, store.UpdateExtensionHealthParams{
		Healthy:     &v,
		HealthError: errPtr,
		ExtID:       extID,
	})
}

// SyncRepo fetches the repo's index and upserts only the torrent extensions that
// pass a liveness Test(). Dead entries are skipped (logged), pre-existing
// installed extensions absent from the usable set are left untouched (upsert
// never deletes), and each installed extension's health is seeded healthy.
func (m *Manager) SyncRepo(ctx context.Context, repo store.ExtensionRepo) error {
	entries, err := m.FetchRepoIndex(ctx, repo.Url)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !strings.EqualFold(e.Type, ExtTypeTorrent) {
			continue
		}
		if pe := m.probeEntry(ctx, e); !pe.Usable {
			m.logger.Warn("sync: skipping unusable extension", "id", e.ID, "err", pe.Error)
			continue
		}
		ext, err := m.InstallExtension(ctx, e, repo.ID)
		if err != nil {
			m.logger.Warn("sync: install failed", "id", e.ID, "err", err)
			continue
		}
		if err := m.persistHealth(ctx, ext.ExtID, true, ""); err != nil {
			m.logger.Warn("sync: seed health failed", "id", ext.ExtID, "err", err)
		}
	}
	if err := m.st.Write().MarkExtensionRepoSynced(ctx, repo.ID); err != nil {
		return err
	}
	return nil
}

// LoadAndRegisterAll loads every enabled torrent extension from the DB and
// registers it into the source.Registry. Called once on boot, before the
// poller starts.
func (m *Manager) LoadAndRegisterAll(ctx context.Context) error {
	rows, err := m.st.Read().ListEnabledExtensionsByType(ctx, ExtTypeTorrent)
	if err != nil {
		return fmt.Errorf("extension: load enabled: %w", err)
	}

	loaded := 0
	for _, row := range rows {
		if row.IsBuiltin != 0 {
			continue // native providers are already in the registry
		}
		if row.Payload == nil || *row.Payload == "" {
			m.logger.Warn("extension: no payload, skipping", "id", row.ExtID)
			continue
		}

		p, err := m.buildInstalledProvider(row)
		if err != nil {
			m.logger.Warn("extension: compile failed", "id", row.ExtID, "err", err)
			continue
		}
		m.registry.Register(p)
		loaded++
		m.logger.Info("extension: registered JS provider", "id", row.ExtID, "name", row.Name)
	}

	m.logger.Info("extensions: loaded", "count", loaded, "total", len(rows))
	return nil
}

// FetchIcon loads the extension by id and fetches its icon URL through the
// DoH/SSRF-guarded httpClient. The icon URL comes from an untrusted user-added
// repo, so the fetch MUST stay on m.httpClient. It validates that the upstream
// returns 200 with an image/* Content-Type and caps the body at 2 MB. Returns
// sql.ErrNoRows when the extension is missing and ErrNoIcon when it has no icon
// or the upstream response is not a usable image.
func (m *Manager) FetchIcon(ctx context.Context, dbID int64) (contentType string, body []byte, err error) {
	row, err := m.st.Read().GetExtension(ctx, dbID)
	if err != nil {
		return "", nil, err
	}
	if row.Icon == nil || *row.Icon == "" {
		return "", nil, ErrNoIcon
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, *row.Icon, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, ErrNoIcon
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return "", nil, ErrNoIcon
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB cap
	if err != nil {
		return "", nil, err
	}
	return ct, b, nil
}

// downloadPayload fetches a JS payload from url.
func (m *Manager) downloadPayload(ctx context.Context, url string) (string, error) {
	dlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5 MB cap
	if err != nil {
		return "", err
	}
	return string(body), nil
}
