package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

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

// registerProvider compiles a stored extension's JS payload into a JSProvider
// and registers it into the source registry. The per-extension settings come
// from the row's stored Settings column (a flat key:value JSON object written at
// install time from the index.json options defaults). No-op for rows without a
// payload.
func (m *Manager) registerProvider(row store.Extension) error {
	if row.Payload == nil || *row.Payload == "" {
		return fmt.Errorf("extension %s: no payload", row.ExtID)
	}
	settings := resolveSettings(nil, row.Settings)
	p, err := NewJSProviderWithDeps(row.ExtID, row.Name, *row.Payload, m.httpClient, m.resolver, settings, m.logger)
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

// SyncRepo fetches the repo's index and upserts all torrent extensions.
func (m *Manager) SyncRepo(ctx context.Context, repo store.ExtensionRepo) error {
	entries, err := m.FetchRepoIndex(ctx, repo.Url)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !strings.EqualFold(e.Type, ExtTypeTorrent) {
			continue
		}
		if _, err := m.InstallExtension(ctx, e, repo.ID); err != nil {
			m.logger.Warn("sync: install failed", "id", e.ID, "err", err)
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

		settings := resolveSettings(nil, row.Settings)
		p, err := NewJSProviderWithDeps(row.ExtID, row.Name, *row.Payload, m.httpClient, m.resolver, settings, m.logger)
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
