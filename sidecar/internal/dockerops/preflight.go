package dockerops

import (
	"context"
	"fmt"
	"strings"
)

// ValidateComposeForRecreate checks Compose parsing, service membership, and
// local image availability before a self-update can remove the old container.
func (c Config) ValidateComposeForRecreate(ctx context.Context, service string) error {
	if service == "" {
		return fmt.Errorf("compose-validate: empty service name")
	}
	out, err := c.run(ctx, "compose", "config", "-q")
	if err != nil {
		return fmt.Errorf("compose config -q failed (yaml unparseable or env_file unresolvable from sidecar view): %w: %s",
			err, strings.TrimSpace(string(out)))
	}
	out, err = c.run(ctx, "compose", "config", "--services")
	if err != nil {
		return fmt.Errorf("compose config --services failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if !containsService(out, service) {
		return fmt.Errorf("service %q not declared in compose file (services found: %q)",
			service, strings.TrimSpace(string(out)))
	}

	imageRef, err := c.CurrentImageRef(service)
	if err != nil {
		c.logf("compose-validate: could not read image ref for service %q: %v (skipping image-local check)", service, err)
		return nil
	}
	if imageRef == "" {
		c.logf("compose-validate: empty image ref for service %q (skipping image-local check)", service)
		return nil
	}
	if !c.HasImage(ctx, imageRef) {
		return fmt.Errorf("image %q referenced by service %q is not available locally (Pull may have failed silently or used a wrong tag)",
			imageRef, service)
	}
	return nil
}

func containsService(output []byte, service string) bool {
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == service {
			return true
		}
	}
	return false
}
