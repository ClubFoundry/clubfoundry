package dockerops

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// isBusyErr recognizes direct and wrapped EBUSY rename failures.
func isBusyErr(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return errors.Is(linkErr.Err, syscall.EBUSY)
	}
	return errors.Is(err, syscall.EBUSY)
}

// writeComposeInPlace preserves the inode of a single-file bind mount.
func writeComposeInPlace(path string, body []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return fmt.Errorf("open for in-place write: %w", err)
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return fmt.Errorf("in-place write: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsync in-place: %w", err)
	}
	return f.Close()
}
