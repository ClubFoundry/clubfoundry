package dockerops

import (
	"context"
	"fmt"
)

// TagImage assigns a local tag without copying image layers.
func (c Config) TagImage(ctx context.Context, src, dst string) error {
	if src == "" || dst == "" {
		return fmt.Errorf("TagImage: empty src=%q or dst=%q", src, dst)
	}
	out, err := c.run(ctx, "tag", src, dst)
	if err != nil {
		return fmt.Errorf("docker tag %s %s: %w: %s", src, dst, err, out)
	}
	return nil
}

// HasImage reports whether a reference exists in the local Docker daemon.
func (c Config) HasImage(ctx context.Context, tag string) bool {
	if tag == "" {
		return false
	}
	_, err := c.run(ctx, "image", "inspect", tag, "--format", "{{.Id}}")
	return err == nil
}
