//go:build !linux

package footprint

// statDataVolume on non-Linux dev builds returns an unavailable stat —
// the sidecar production runtime is always Linux; this stub exists so
// `go build ./...` works on Windows / macOS dev hosts.
func statDataVolume(path string) DiskStat {
	return DiskStat{Path: path, Available: false}
}
