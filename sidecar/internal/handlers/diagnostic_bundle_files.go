package handlers

import (
	"archive/zip"
	"os"
	"path/filepath"
)

// writeBundleDirTree copies regular files from each direct child directory.
func writeBundleDirTree(zw *zip.Writer, root, prefix string) {
	subs, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, sub := range subs {
		if !sub.IsDir() {
			continue
		}
		subRoot := filepath.Join(root, sub.Name())
		files, err := os.ReadDir(subRoot)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			body, err := os.ReadFile(filepath.Join(subRoot, f.Name()))
			if err != nil {
				continue
			}
			zipPath := prefix + "/" + sub.Name() + "/" + f.Name()
			addBundleFile(zw, zipPath, body)
		}
	}
}
