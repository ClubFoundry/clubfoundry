package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// PruneOld keeps the newest KeepN complete backup triplets.
func (c Config) PruneOld() error {
	if c.KeepN <= 0 {
		return nil
	}
	entries, err := os.ReadDir(c.BackupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var list []candidate
	for _, e := range entries {
		if e.IsDir() || !isMainBackupName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, candidate{
			path: filepath.Join(c.BackupsDir, e.Name()),
			mod:  info.ModTime(),
		})
	}
	if len(list) <= c.KeepN {
		return c.cleanOrphanSiblings()
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })
	for _, victim := range list[c.KeepN:] {
		if err := os.Remove(victim.path); err != nil {
			return fmt.Errorf("prune %s: %w", victim.path, err)
		}
		_ = os.Remove(victim.path + "-wal")
		_ = os.Remove(victim.path + "-shm")
	}
	return c.cleanOrphanSiblings()
}
