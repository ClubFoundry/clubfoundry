package updater

import (
	"context"
	"fmt"
	"io"
)

// tagRetention maintains best-effort local `previous` and `current` aliases.
// Unresolved versions are skipped because they cannot identify an image.
func (d *Deps) tagRetention(ctx context.Context, fromVersion, toVersion string, logW io.Writer) {
	if d.Docker.MainServiceName() == "" {
		return
	}
	repo := d.Docker.MainServiceName() // e.g. "clubfoundry"
	if fromVersion != "" && fromVersion != "unknown" {
		src := fmt.Sprintf("%s:%s", repo, fromVersion)
		dst := fmt.Sprintf("%s:previous", repo)
		if err := d.Docker.TagImage(ctx, src, dst); err != nil {
			fmt.Fprintf(logW, "tag-retention: %s -> %s failed: %v (non-fatal)\n", src, dst, err)
		} else {
			fmt.Fprintf(logW, "tag-retention: %s -> %s\n", src, dst)
		}
	}
	if toVersion != "" && toVersion != "latest" && toVersion != "unknown" {
		src := fmt.Sprintf("%s:%s", repo, toVersion)
		dst := fmt.Sprintf("%s:current", repo)
		if err := d.Docker.TagImage(ctx, src, dst); err != nil {
			fmt.Fprintf(logW, "tag-retention: %s -> %s failed: %v (non-fatal)\n", src, dst, err)
		} else {
			fmt.Fprintf(logW, "tag-retention: %s -> %s\n", src, dst)
		}
	}
}
