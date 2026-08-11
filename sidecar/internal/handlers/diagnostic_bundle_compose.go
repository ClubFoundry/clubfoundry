package handlers

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
)

func writeBundleComposeFile(zw *zip.Writer, dataDir string) {
	candidate := composeFilePathFromEnv()
	body, err := os.ReadFile(candidate)
	if err != nil {
		addBundleFile(zw, "docker-compose.yml.MISSING.txt",
			[]byte(fmt.Sprintf("attempted path: %s\nerror: %v\n", candidate, err)))
		return
	}
	addBundleFile(zw, "docker-compose.yml", redactSecrets(body))
}

func composeFilePathFromEnv() string {
	dir := os.Getenv("CLUBFOUNDRY_COMPOSE_DIR")
	if dir == "" {
		dir = "/app"
	}
	return filepath.Join(dir, "docker-compose.yml")
}
