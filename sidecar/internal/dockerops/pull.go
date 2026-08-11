package dockerops

import (
	"context"
	"fmt"
)

// Pull fetches an image from mirrors, a single URL, or the compose registry.
// A non-empty tag updates compose intent before registry pulls.
func (c Config) Pull(ctx context.Context, service, tag string, opts PullOpts) error {
	// URL lists select streaming for one source and race-to-file for multiple.
	if len(opts.URLs) >= 1 {
		return c.loadFromURLs(ctx, service, tag, opts)
	}
	if opts.URL != "" {
		return c.loadFromURL(ctx, service, tag, opts)
	}
	if service != "" && tag != "" {
		if err := c.SetServiceImage(service, tag); err != nil {
			return fmt.Errorf("set image before pull: %w", err)
		}
	}
	args := []string{"compose", "pull"}
	if service != "" {
		args = append(args, service)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("docker compose pull %s failed: %w: %s", service, err, out)
	}
	return nil
}
