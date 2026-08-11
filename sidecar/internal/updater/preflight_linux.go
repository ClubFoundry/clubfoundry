//go:build linux

package updater

import "syscall"

// freeDiskBytes returns the free bytes available to the calling user on the
// filesystem hosting `path`. Sidecar runs in a Linux container in production.
func freeDiskBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	// Use Bavail (blocks available to the unprivileged user), not Bfree —
	// reserved blocks aren't usable by us.
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
