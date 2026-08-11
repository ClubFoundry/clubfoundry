package dockerops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SetServiceImage atomically replaces one service image while preserving layout.
// Bare tags reuse the current registry and repository.
func (c Config) SetServiceImage(service, newRef string) error {
	path := c.composeFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read compose: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	idx, indent, currentRef, found := findServiceImageLine(string(data), service)
	if !found {
		return fmt.Errorf("service %q has no image: line in %s", service, path)
	}

	resolved := resolveImageRef(currentRef, newRef)
	lines[idx] = fmt.Sprintf("%simage: %s", indent, resolved)

	tmp := path + ".part"
	// Sync the replacement before rename so a completed update survives power loss.
	body := []byte(strings.Join(lines, "\n"))
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("fsync tmp: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// A single-file bind mount rejects rename with EBUSY. Preserve its inode.
		if isBusyErr(err) {
			if e2 := writeComposeInPlace(path, body); e2 == nil {
				_ = os.Remove(tmp)
				if d, derr := os.Open(filepath.Dir(path)); derr == nil {
					_ = d.Sync()
					_ = d.Close()
				}
				return nil
			} else {
				_ = os.Remove(tmp)
				return fmt.Errorf("rename tmp: %w; in-place fallback also failed: %v", err, e2)
			}
		}
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp: %w", err)
	}
	if d, derr := os.Open(filepath.Dir(path)); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
