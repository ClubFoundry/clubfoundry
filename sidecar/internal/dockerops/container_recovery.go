package dockerops

import (
	"context"
	"fmt"
	"strings"
)

func containsContainerNameConflict(out []byte, name string) bool {
	message := string(out)
	if !strings.Contains(message, "already in use") {
		return false
	}
	return strings.Contains(message, "\"/"+name+"\"") || strings.Contains(message, "\""+name+"\"")
}

func isClubFoundryImage(ref string) bool {
	lastSlash := strings.LastIndexByte(ref, '/')
	image := ref[lastSlash+1:]
	tag := strings.LastIndexByte(image, ':')
	if tag <= 0 || tag == len(image)-1 {
		return false
	}
	repo := image[:tag]
	return repo == "clubfoundry" || repo == "clubfoundry-updater"
}

func (c Config) forceRemoveContainer(ctx context.Context, name string) error {
	// Match the installer's ownership rule before destructive recovery.
	inspect, err := c.run(ctx, "inspect", name, "--format", "{{.Config.Image}}")
	if err != nil {
		return fmt.Errorf("refusing to remove container %s: ownership check failed: %w: %s",
			name, err, strings.TrimSpace(string(inspect)))
	}
	image := strings.TrimSpace(string(inspect))
	if !isClubFoundryImage(image) {
		return fmt.Errorf("refusing to remove container %s: image %q is not ClubFoundry", name, image)
	}

	out, err := c.run(ctx, "rm", "-f", name)
	if err == nil || strings.Contains(string(out), "No such container") {
		return nil
	}
	return fmt.Errorf("docker rm -f %s: %w: %s", name, err, out)
}
