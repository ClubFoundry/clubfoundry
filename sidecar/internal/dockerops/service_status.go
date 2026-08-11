package dockerops

import (
	"context"
	"fmt"
	"strings"
)

// ServiceInfo is the normalized Compose service state used by the updater.
type ServiceInfo struct {
	Service string
	Image   string
	Tag     string
	State   string
}

// PS returns the known containers in the Compose project.
func (c Config) PS(ctx context.Context) ([]ServiceInfo, error) {
	out, err := c.run(ctx, "compose", "ps", "--format", "json", "--all")
	if err != nil {
		return nil, fmt.Errorf("docker compose ps failed: %w: %s", err, out)
	}
	return parsePS(string(out))
}

// ContainerExistsByName checks running and stopped containers by exact name.
func ContainerExistsByName(ctx context.Context, c Config, name string) (bool, error) {
	if name == "" {
		return false, fmt.Errorf("empty container name")
	}
	out, err := c.run(ctx, "ps", "-a", "--filter", fmt.Sprintf("name=^/%s$", name), "--format", "{{.Names}}")
	if err != nil {
		return false, fmt.Errorf("docker ps -a: %w: %s", err, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == name {
			return true, nil
		}
	}
	return false, nil
}
