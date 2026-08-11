package dockerops

import (
	"fmt"
	"os"
	"path/filepath"
)

// composeFilePath returns the canonical compose file used by sidecar operations.
func (c Config) composeFilePath() string {
	return filepath.Join(c.ComposeDir, "docker-compose.yml")
}

// CurrentImageRef returns the image configured for a named service.
func (c Config) CurrentImageRef(service string) (string, error) {
	data, err := os.ReadFile(c.composeFilePath())
	if err != nil {
		return "", fmt.Errorf("read compose: %w", err)
	}
	_, _, rawImage, found := findServiceImageLine(string(data), service)
	if !found {
		return "", fmt.Errorf("service %q has no image: line in %s", service, c.composeFilePath())
	}
	return rawImage, nil
}
