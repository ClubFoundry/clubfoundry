package backup

import (
	"os"
	"path/filepath"
	"strings"
)

// cleanOrphanSiblings removes WAL and SHM files without a main backup.
func (c Config) cleanOrphanSiblings() error {
	entries, err := os.ReadDir(c.BackupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "clm.db.pre-update-") {
			continue
		}
		var mainName string
		switch {
		case strings.HasSuffix(name, "-wal"):
			mainName = strings.TrimSuffix(name, "-wal")
		case strings.HasSuffix(name, "-shm"):
			mainName = strings.TrimSuffix(name, "-shm")
		default:
			continue
		}
		if _, err := os.Stat(filepath.Join(c.BackupsDir, mainName)); err == nil {
			continue
		}
		_ = os.Remove(filepath.Join(c.BackupsDir, name))
	}
	return nil
}
