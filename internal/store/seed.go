package store

import (
	"context"
	"path/filepath"

	"github.com/modbender/ssanime-gui/internal/config"
)

// Seeded constant defaults. These mirror automin's tuned x265 config and the
// app-flow defaults; they are the immutable base the inheritance model resolves
// against and the singleton settings the app boots with.
const (
	builtinProfileName = "Automin (x265)"

	defaultNamingTemplate = "{series}/Season {season}/{res}/{series} - S{season}E{episode}.{ext}"

	defaultDownloadClientName = "Embedded (anacrolix)"

	defaultConcurrencyDownload = 3
	defaultConcurrencyEncode   = 1
)

// pointer helpers keep the seed literal readable: sqlc nullable columns are *T.
func p[T any](v T) *T { return &v }

// seed inserts the immutable builtin encode profile, the default embedded
// download client, and the singleton settings row — each idempotently, so a
// second boot is a no-op. Runs on the single-writer pool after migrations.
func (s *Store) seed(ctx context.Context, cfg *config.Config) error {
	profileID, err := s.seedBuiltinProfile(ctx)
	if err != nil {
		return err
	}
	clientID, err := s.seedDefaultDownloadClient(ctx)
	if err != nil {
		return err
	}
	return s.seedSettings(ctx, cfg, profileID, clientID)
}

// seedBuiltinProfile inserts the immutable "Automin (x265)" profile if absent
// and returns its id. Values are automin's tuned defaults (crf 24.2, smartblur
// on, deblock '1,1', psy_rd/psy_rdoq/aq_strength 1, preset slow), fully
// specified so it can root an inheritance chain.
func (s *Store) seedBuiltinProfile(ctx context.Context) (int64, error) {
	if existing, err := s.write.GetEncodeProfileByName(ctx, builtinProfileName); err == nil {
		return existing.ID, nil
	}
	prof, err := s.write.CreateEncodeProfile(ctx, CreateEncodeProfileParams{
		Uuid:              newUUID(),
		Name:              builtinProfileName,
		Builtin:           1,
		ParentID:          nil,
		Codec:             p("x265"),
		Crf:               p(24.2),
		Preset:            p("slow"),
		Smartblur:         p[int64](1),
		Deinterlace:       p[int64](0),
		Deblock:           p("1,1"),
		PsyRd:             p(1.0),
		PsyRdoq:           p(1.0),
		AqStrength:        p(1.0),
		AqMode:            p[int64](2),
		Scale:             nil, // per-output scale resolved from output_resolutions
		Audio:             p("copy"),
		Container:         p("mkv"),
		X265Params:        nil,
		OutputResolutions: p("[1080,720,480]"),
	})
	if err != nil {
		return 0, err
	}
	return prof.ID, nil
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
// default profile + embedded download backend and paths rooted under the app
// data dir.
func (s *Store) seedSettings(ctx context.Context, cfg *config.Config, profileID, clientID int64) error {
	exists, err := s.write.SettingsExist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.write.InsertSettings(ctx, InsertSettingsParams{
		DownloadRoot:        filepath.Join(cfg.DataDir, "downloads"),
		EncodedRoot:         filepath.Join(cfg.DataDir, "library"),
		CleanupPolicy:       "delete",
		NamingTemplate:      defaultNamingTemplate,
		DownloadBackend:     &clientID,
		DefaultProfileID:    &profileID,
		ConcurrencyDownload: defaultConcurrencyDownload,
		ConcurrencyEncode:   defaultConcurrencyEncode,
		Port:                int64(config.DefaultPort),
		DohEnabled:          1,
	})
	return err
}
