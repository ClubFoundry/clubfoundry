package updater

import (
	"os"
	"path/filepath"
)

func migrateLegacyFlatLog(rootDir, updateID, dir string) {
	if rootDir == "" || updateID == "" || dir == "" {
		return
	}
	legacy := filepath.Join(rootDir, safeFilename(updateID)+".log")
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	target := filepath.Join(dir, "update.log")
	if _, err := os.Stat(target); err == nil {
		return
	}
	_ = os.Rename(legacy, target)
}
