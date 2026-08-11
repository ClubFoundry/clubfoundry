//go:build linux

package footprint

import "syscall"

// statDataVolume returns the usage of the filesystem hosting `path`.
// Uses Bavail (free to unprivileged caller), so the reported "free" is
// what we can actually allocate — matches preflight.go::freeDiskBytes.
func statDataVolume(path string) DiskStat {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return DiskStat{Path: path, Available: false}
	}
	bsize := int64(st.Bsize)
	total := int64(st.Blocks) * bsize
	free := int64(st.Bavail) * bsize
	used := total - free
	if used < 0 {
		used = 0
	}
	return DiskStat{
		Path:       path,
		TotalBytes: total,
		UsedBytes:  used,
		FreeBytes:  free,
		Available:  true,
	}
}
