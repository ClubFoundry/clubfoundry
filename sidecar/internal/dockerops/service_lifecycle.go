package dockerops

import (
	"context"
	"fmt"
)

// Up recreates a Compose service and retries once after removing a conflicting
// ClubFoundry container that does not belong to the project.
func (c Config) Up(ctx context.Context, service string) error {
	args := []string{"compose", "up", "-d", "--force-recreate", "--remove-orphans"}
	if service != "" {
		args = append(args, service)
	}
	out, err := c.run(ctx, args...)
	if err == nil {
		return nil
	}
	if service != "" && containsContainerNameConflict(out, service) {
		c.logf("compose up %s: name conflict detected — removing orphan and retrying", service)
		if removeErr := c.forceRemoveContainer(ctx, service); removeErr != nil {
			return fmt.Errorf("docker compose up %s failed: %w; orphan removal also failed: %v: %s",
				service, err, removeErr, out)
		}
		out, err = c.run(ctx, args...)
		if err != nil {
			return fmt.Errorf("docker compose up %s failed (after orphan removal): %w: %s", service, err, out)
		}
		c.logf("compose up %s: succeeded after orphan removal", service)
		return nil
	}
	return fmt.Errorf("docker compose up %s failed: %w: %s", service, err, out)
}

// Stop halts a Compose service without removing it.
func (c Config) Stop(ctx context.Context, service string) error {
	out, err := c.run(ctx, "compose", "stop", service)
	if err != nil {
		return fmt.Errorf("docker compose stop %s failed: %w: %s", service, err, out)
	}
	return nil
}
