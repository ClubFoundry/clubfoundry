package updater

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

func pruneByCount(parent string, keep int) {
	if parent == "" || keep < 0 {
		return
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	type entryInfo struct {
		name  string
		mtime time.Time
		isDir bool
	}
	items := make([]entryInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, entryInfo{name: entry.Name(), mtime: info.ModTime(), isDir: entry.IsDir()})
	}
	if len(items) <= keep {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mtime.After(items[j].mtime) })
	for _, item := range items[keep:] {
		path := filepath.Join(parent, item.name)
		if item.isDir {
			_ = os.RemoveAll(path)
		} else {
			_ = os.Remove(path)
		}
	}
}

func pruneByAge(parent string, maxAge time.Duration) {
	if parent == "" || maxAge <= 0 {
		return
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		path := filepath.Join(parent, entry.Name())
		if entry.IsDir() {
			_ = os.RemoveAll(path)
		} else {
			_ = os.Remove(path)
		}
	}
}
