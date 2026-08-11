// Package backup manages offline SQLite backup triplets around updates.
// Callers must stop the main app before copying clm.db, clm.db-wal, and
// clm.db-shm so all three files describe one consistent database state.
package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CreateBackup copies the SQLite main, WAL, and SHM files when present.
func (c Config) CreateBackup(fromVersion string) (string, error) {
	if err := os.MkdirAll(c.BackupsDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir backups dir: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	name := fmt.Sprintf("clm.db.pre-update-v%s-%s", sanitize(fromVersion), stamp)
	dst := filepath.Join(c.BackupsDir, name)
	if err := copyFile(c.DBPath, dst); err != nil {
		return "", fmt.Errorf("copy db: %w", err)
	}
	// WAL and SHM are optional when SQLite checkpointed cleanly.
	for _, suffix := range []string{"-wal", "-shm"} {
		srcExtra := c.DBPath + suffix
		if _, err := os.Stat(srcExtra); err != nil {
			continue
		}
		if err := copyFile(srcExtra, dst+suffix); err != nil {
			// A main backup without its existing WAL cannot be restored safely.
			_ = os.Remove(dst)
			_ = os.Remove(dst + "-wal")
			return "", fmt.Errorf("copy %s: %w", srcExtra, err)
		}
	}
	return dst, nil
}
