//go:build !windows

package download

import "golang.org/x/sys/unix"

// freeDiskBytes returns the bytes available to an unprivileged caller on the
// filesystem holding dir. Bavail (not Bfree) excludes blocks reserved for root.
func freeDiskBytes(dir string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}
