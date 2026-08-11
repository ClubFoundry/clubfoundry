package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
)

// hostDataDir returns the host path backing the sidecar data volume.
func hostDataDir() string {
	if v := os.Getenv("CLUBFOUNDRY_HOST_DATA_DIR"); v != "" {
		return v
	}
	return "/opt/clubfoundry/data"
}

// hostComposePath returns the host path mounted for future sidecar replacement.
func hostComposePath(_ string) string {
	if v := os.Getenv("CLUBFOUNDRY_HOST_COMPOSE_FILE"); v != "" {
		return v
	}
	return "/opt/clubfoundry/docker-compose.yml"
}

func orDev(s string) string {
	if s == "" || strings.EqualFold(s, "dev") {
		return "latest"
	}
	return s
}

// chownRecursive aligns bind-mounted data ownership with the main container.
func chownRecursive(path string, uid, gid int) error {
	return filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Lchown avoids following symlinks outside the managed data directory.
		return os.Lchown(p, uid, gid)
	})
}
