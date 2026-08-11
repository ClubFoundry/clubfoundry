// Package dockerops wraps Docker Compose CLI and image lifecycle operations.
// The runtime must provide the Docker CLI, Compose plugin, socket, and project.
package dockerops

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"time"
)

// Config identifies the Compose project, managed services, Docker executable,
// and default command timeout used by sidecar operations.
type Config struct {
	ComposeDir     string
	MainService    string
	UpdaterService string
	DockerBin      string
	DefaultTimeout time.Duration
}

// DefaultConfig returns the standard ClubFoundry Docker and Compose settings,
// with supported environment overrides applied.
func DefaultConfig() Config {
	return Config{
		ComposeDir:     envOr("CLUBFOUNDRY_COMPOSE_DIR", "/app"),
		MainService:    envOr("CLUBFOUNDRY_MAIN_SERVICE", "clubfoundry"),
		UpdaterService: envOr("CLUBFOUNDRY_UPDATER_SERVICE", "clubfoundry-updater"),
		DockerBin:      envOr("DOCKER_BIN", "docker"),
		DefaultTimeout: 5 * time.Minute,
	}
}

// MainServiceName returns the managed application service name.
func (c Config) MainServiceName() string { return c.MainService }

// UpdaterServiceName returns the managed sidecar service name.
func (c Config) UpdaterServiceName() string { return c.UpdaterService }

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (c Config) logf(format string, args ...any) {
	log.Printf("[dockerops] "+format, args...)
}

// run executes one Docker CLI command in the Compose project directory.
func (c Config) run(parent context.Context, args ...string) ([]byte, error) {
	timeout := c.DefaultTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.DockerBin, args...)
	cmd.Dir = c.ComposeDir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.Bytes(), err
}
