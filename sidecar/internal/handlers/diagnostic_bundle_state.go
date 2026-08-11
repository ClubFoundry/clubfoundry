package handlers

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func writeBundleInfo(zw *zip.Writer, dataDir, version string) {
	body := fmt.Sprintf(
		"sidecar_version: %s\ngenerated_at: %s\ndata_dir: %s\n",
		version,
		time.Now().UTC().Format(time.RFC3339),
		dataDir,
	)
	addBundleFile(zw, "INFO.txt", []byte(body))
}

func writeBundleStateFiles(zw *zip.Writer, dataDir string) {
	root := filepath.Join(dataDir, "sidecar-state")
	for _, name := range []string{"main.json", "self.json"} {
		src := filepath.Join(root, name)
		body, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		addBundleFile(zw, "sidecar-state/"+name, body)
	}
}

func writeBundleSentinels(zw *zip.Writer, dataDir string) {
	root := filepath.Join(dataDir, "sidecar-state", "recreate-status")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		addBundleFile(zw, "sidecar-state/recreate-status/"+name, body)
	}
}
