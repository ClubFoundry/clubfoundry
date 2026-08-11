//go:build !linux

package updater

import "errors"

// freeDiskBytes stub for non-Linux dev builds. Sidecar production runtime
// is always Linux; this exists so `go build ./...` works on Windows / macOS
// dev hosts. Callers on those platforms get ErrUnsupported and the
// preflight code degrades gracefully (treats the disk check as inconclusive,
// not failed).
func freeDiskBytes(_ string) (int64, error) {
	return 0, errors.New("freeDiskBytes: not supported on this platform (linux-only)")
}
