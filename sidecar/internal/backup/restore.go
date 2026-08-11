package backup

import (
	"fmt"
	"os"
)

// RestoreBackup validates and restores one SQLite backup triplet.
func (c Config) RestoreBackup(backupPath string) error {
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}
	if err := validateSQLiteHeader(backupPath); err != nil {
		return fmt.Errorf("backup at %s rejected: %w", backupPath, err)
	}
	if err := copyFile(backupPath, c.DBPath); err != nil {
		return fmt.Errorf("restore copy: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		src := backupPath + suffix
		dst := c.DBPath + suffix
		if _, err := os.Stat(src); err != nil {
			// Never combine a restored main file with an unrelated live sibling.
			_ = os.Remove(dst)
			continue
		}
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("restore %s: %w", src, err)
		}
	}
	return nil
}
