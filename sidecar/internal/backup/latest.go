package backup

import (
	"os"
	"path/filepath"
	"time"
)

// LatestBackup returns the newest main backup, excluding its siblings.
func (c Config) LatestBackup() (string, error) {
	entries, err := os.ReadDir(c.BackupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var (
		bestPath string
		bestMod  time.Time
	)
	for _, e := range entries {
		if e.IsDir() || !isMainBackupName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestMod) {
			bestMod = info.ModTime()
			bestPath = filepath.Join(c.BackupsDir, e.Name())
		}
	}
	return bestPath, nil
}
