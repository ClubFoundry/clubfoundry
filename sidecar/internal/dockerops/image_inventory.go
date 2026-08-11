package dockerops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ListImagesByRepo returns exact-repository matches from the local daemon.
func (c Config) ListImagesByRepo(ctx context.Context, repo string) ([]ImageInfo, error) {
	if repo == "" {
		return nil, fmt.Errorf("ListImagesByRepo: empty repo")
	}
	out, err := c.run(ctx, "images", "--format", "{{json .}}", repo)
	if err != nil {
		return nil, fmt.Errorf("docker images %s: %w: %s", repo, err, strings.TrimSpace(string(out)))
	}
	images := []ImageInfo{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var img ImageInfo
		if err := json.Unmarshal([]byte(line), &img); err != nil {
			continue
		}
		if img.Repository != repo {
			continue
		}
		img.SizeBytes = parseSizeString(img.Size)
		images = append(images, img)
	}
	return images, nil
}

// RemoveImage runs `docker rmi <ref>` and retains Docker's diagnostic output.
func (c Config) RemoveImage(ctx context.Context, ref string) error {
	if ref == "" {
		return fmt.Errorf("RemoveImage: empty ref")
	}
	out, err := c.run(ctx, "rmi", ref)
	if err != nil {
		return fmt.Errorf("docker rmi %s: %w: %s", ref, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// IsImageInUse reports whether any running or stopped container uses ref.
func (c Config) IsImageInUse(ctx context.Context, ref string) bool {
	if ref == "" {
		return false
	}
	out, err := c.run(ctx, "ps", "-a", "--filter", fmt.Sprintf("ancestor=%s", ref), "--format", "{{.ID}}")
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(out)) != ""
}
