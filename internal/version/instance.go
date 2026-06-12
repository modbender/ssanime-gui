package version

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"
)

// instanceID is computed once on first call and cached. Composing it is cheap,
// but os.Stat hits the filesystem, so we memoize.
var (
	instanceOnce   sync.Once
	instanceCached string
)

// InstanceID returns a stable identifier for THIS running build, computed once
// per process. It distinguishes builds finely enough that a rebuilt dev binary
// (Version=="dev", Commit=="") is still recognized as different from the one
// already running: the identity folds in the executable's path, size, and
// modification time alongside Version+Commit.
//
// For released builds Version+Commit already differ across versions and match
// within one install, so double-launching the same install yields equal ids
// (a "reopen"). For dev builds the exe ModTime+Size differ across every
// `go build`/`mage` rebuild, so a fresh build yields a different id (a
// "takeover"). The same process always reports the same id across calls.
func InstanceID() string {
	instanceOnce.Do(func() {
		var exePath string
		var size int64
		var modUnixNano int64
		if p, err := os.Executable(); err == nil {
			exePath = p
			if fi, err := os.Stat(p); err == nil {
				size = fi.Size()
				modUnixNano = fi.ModTime().UnixNano()
			}
		}
		instanceCached = ComposeInstanceID(Version, Commit, exePath, size, time.Unix(0, modUnixNano))
	})
	return instanceCached
}

// ComposeInstanceID is the pure, side-effect-free composition behind InstanceID.
// It is exported so the identity logic can be unit-tested without touching the
// filesystem. Identical inputs always hash to the same id; any differing field
// changes the id.
func ComposeInstanceID(version, commit, exePath string, exeSize int64, exeModTime time.Time) string {
	raw := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d",
		version, commit, exePath, exeSize, exeModTime.UnixNano())
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}
