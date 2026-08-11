package updater

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

func pruneByBytes(dataDir string, cap int64) {
	if dataDir == "" || cap <= 0 {
		return
	}
	type retainedEntry struct {
		path  string
		bytes int64
		mtime time.Time
		isDir bool
	}
	var entries []retainedEntry
	for _, parent := range []string{updateLogsDir(dataDir), updateFailuresDir(dataDir)} {
		children, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, child := range children {
			path := filepath.Join(parent, child.Name())
			info, err := child.Info()
			if err != nil {
				continue
			}
			size := info.Size()
			if child.IsDir() {
				size = dirSize(path)
			}
			entries = append(entries, retainedEntry{path: path, bytes: size, mtime: info.ModTime(), isDir: child.IsDir()})
		}
	}
	var total int64
	for _, entry := range entries {
		total += entry.bytes
	}
	if total <= cap {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].mtime.Before(entries[j].mtime) })
	for _, entry := range entries {
		if total <= cap {
			return
		}
		var err error
		if entry.isDir {
			err = os.RemoveAll(entry.path)
		} else {
			err = os.Remove(entry.path)
		}
		if err == nil {
			total -= entry.bytes
		}
	}
}

func dirSize(dir string) int64 {
	var total int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if entry.IsDir() {
			total += dirSize(filepath.Join(dir, entry.Name()))
			continue
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
	}
	return total
}
