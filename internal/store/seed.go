package store

import (
	"context"
	"path/filepath"

	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/defaults"
)

// Seeded defaults. These mirror automin's tuned x265 config and the app-flow
// defaults; they are the immutable base the inheritance model resolves against
// and the singleton settings the app boots with. Sourced from the embedded
// defaults.json so all shipped defaults live in one place.
var (
	builtinProfileName = defaults.Values.Profiles[0].Name

	defaultNamingTemplate = defaults.Values.SeedSettings.NamingTemplate

	defaultDownloadClientName = defaults.Values.SeedSettings.DownloadClientName

	defaultConcurrencyDownload = defaults.Values.SeedSettings.ConcurrencyDownload
	defaultConcurrencyEncode   = defaults.Values.SeedSettings.ConcurrencyEncode

	// defaultTrustedReleaseGroups is the JSON-encoded trusted-group allowlist a
	// fresh install boots with. Mirrors the column default in migration 00013 and
	// source.TrustedReleaseGroups; an explicitly empty array disables the filter.
	defaultTrustedReleaseGroups = defaults.Values.SeedSettings.TrustedReleaseGroupsJSON()
)

// pointer helpers keep the seed literal readable: sqlc nullable columns are *T.
func p[T any](v T) *T { return &v }

// seed inserts the immutable builtin encode profile, the default embedded
// download client, and the singleton settings row — each idempotently, so a
// second boot is a no-op. Runs on the single-writer pool after migrations.
func (s *Store) seed(ctx context.Context, cfg *config.Config) error {
	profileID, err := s.seedBuiltinProfiles(ctx)
	if err != nil {
		return err
	}
	// The embedded client must exist so the Auto backend (null download_backend)
	// resolves to it via GetDefaultDownloadClient; its id is not pinned into
	// settings — Auto is the default.
	if _, err := s.seedDefaultDownloadClient(ctx); err != nil {
		return err
	}
	return s.seedSettings(ctx, cfg, profileID)
}

// seedBuiltinProfiles inserts every shipped builtin encode profile (each
// idempotently via the GetEncodeProfileByName check) and returns the id of the
// first one, which roots the default settings profile. The shipped values are
// automin's tuned defaults (crf 24.2, smartblur on, deblock '1,1',
// psy_rd/psy_rdoq/aq_strength 1, preset slow), fully specified so a profile can
// root an inheritance chain. Adding a profile is a single entry in defaults.json.
func (s *Store) seedBuiltinProfiles(ctx context.Context) (int64, error) {
	var firstID int64
	for i, prof := range defaults.Values.Profiles {
		id, err := s.seedBuiltinProfile(ctx, prof)
		if err != nil {
			return 0, err
		}
		if i == 0 {
			firstID = id
		}
	}
	return firstID, nil
}

// seedBuiltinProfile inserts one builtin profile if absent and returns its id.
func (s *Store) seedBuiltinProfile(ctx context.Context, prof defaults.Profile) (int64, error) {
	if existing, err := s.write.GetEncodeProfileByName(ctx, prof.Name); err == nil {
		return existing.ID, nil
	}
	row, err := s.write.CreateEncodeProfile(ctx, CreateEncodeProfileParams{
		Uuid:              newUUID(),
		Name:              prof.Name,
		Builtin:           b2i(prof.Builtin),
		ParentID:          nil,
		Codec:             p(prof.Codec),
		Crf:               p(prof.CRF),
		Preset:            p(prof.Preset),
		Smartblur:         p(b2i(prof.SmartBlur)),
		Deinterlace:       p(b2i(prof.Deinterlace)),
		Deblock:           p(prof.Deblock),
		PsyRd:             p(prof.PsyRD),
		PsyRdoq:           p(prof.PsyRDOQ),
		AqStrength:        p(prof.AQStrength),
		AqMode:            p(prof.AQMode),
		Scale:             nil, // per-output scale resolved from output_resolutions
		Audio:             p(prof.Audio),
		Container:         p(prof.Container),
		X265Params:        nil,
		OutputResolutions: p(prof.OutputResolutions),
	})
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

// b2i maps a bool to the int64 (0/1) the schema's boolean-as-integer columns use.
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// seedDefaultDownloadClient inserts the embedded anacrolix client as the default
// if no embedded client exists, returning its id.
func (s *Store) seedDefaultDownloadClient(ctx context.Context) (int64, error) {
	clients, err := s.write.ListDownloadClients(ctx)
	if err != nil {
		return 0, err
	}
	for _, c := range clients {
		if c.Kind == "embedded" {
			return c.ID, nil
		}
	}
	client, err := s.write.CreateDownloadClient(ctx, CreateDownloadClientParams{
		Uuid:      newUUID(),
		Kind:      "embedded",
		Name:      defaultDownloadClientName,
		Enabled:   1,
		IsDefault: 1,
	})
	if err != nil {
		return 0, err
	}
	return client.ID, nil
}

// seedSettings inserts the singleton settings row (id=1) if absent, wiring the
// default profile and paths rooted under the app data dir. download_backend is
// left null ("Auto"): the queue resolves it to the default download client (the
// embedded torrent client) at run time, so the backend isn't pinned to a
// specific client id.
func (s *Store) seedSettings(ctx context.Context, cfg *config.Config, profileID int64) error {
	exists, err := s.write.SettingsExist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.write.InsertSettings(ctx, InsertSettingsParams{
		DownloadRoot:         filepath.Join(cfg.DataDir, "downloads"),
		EncodedRoot:          filepath.Join(cfg.DataDir, "library"),
		CleanupPolicy:        "delete",
		NamingTemplate:       defaultNamingTemplate,
		DownloadBackend:      nil,
		DefaultProfileID:     &profileID,
		ConcurrencyDownload:  defaultConcurrencyDownload,
		ConcurrencyEncode:    defaultConcurrencyEncode,
		Port:                 int64(config.DefaultPort),
		DohEnabled:           1,
		TrustedReleaseGroups: defaultTrustedReleaseGroups,
	})
	return err
}
