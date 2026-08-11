//go:build linux

package backup

import (
	"os"
	"syscall"
)

// preserveOwnership keeps a restored database writable by the unprivileged
// application container. New destinations retain the operating-system default.
func preserveOwnership(path, dst string) error {
	info, err := os.Stat(dst)
	if err != nil {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Chown(path, int(stat.Uid), int(stat.Gid))
}
