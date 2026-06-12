// Package config resolves process-level paths and defaults. Persistent,
// user-editable settings (download/encoded roots, concurrency, cleanup policy)
// live in the SQLite `settings` row; this package only covers what's needed to
// boot before the DB is open: the app-data dir, DB path, and default port.
package config

import (
	"os"
	"path/filepath"
)

const (
	// AppName is used for the app-data directory and tray label.
	AppName = "ssanime-gui"
	// DefaultPort is the localhost port the daemon binds by default.
	DefaultPort = 4773
)

// Config holds boot-time configuration.
type Config struct {
	DataDir string // app-data dir (DB, provisioned binaries, extension payloads)
	DBPath  string // SQLite file
	Port    int
}

// Load resolves the app-data dir (creating it) and boot defaults.
func Load() (*Config, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		base, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	dataDir := filepath.Join(base, AppName)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	return &Config{
		DataDir: dataDir,
		DBPath:  filepath.Join(dataDir, AppName+".db"),
		Port:    DefaultPort,
	}, nil
}
