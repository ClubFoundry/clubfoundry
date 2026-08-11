package dockerops

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
)

type selfInspect struct {
	image          string
	composeHostDir string
	mounts         []selfMount
}

type selfMount struct {
	source      string
	destination string
	rw          bool
}

// inspectSelf finds the current image and bind mounts.
func (c Config) inspectSelf(ctx context.Context) (selfInspect, error) {
	selfID := os.Getenv("CLUBFOUNDRY_UPDATER_NAME")
	if selfID == "" {
		selfID = c.UpdaterService
	}
	if selfID == "" {
		selfID = os.Getenv("HOSTNAME")
	}
	if selfID == "" {
		return selfInspect{}, fmt.Errorf("cannot determine self container name (UpdaterService empty + HOSTNAME unset)")
	}

	format := `{{.Config.Image}}|{{range .Mounts}}{{.Destination}}|{{.Source}}|{{.RW}};{{end}}`
	out, err := c.run(ctx, "inspect", selfID, "--format", format)
	if err != nil {
		return selfInspect{}, fmt.Errorf("docker inspect %s: %w: %s", selfID, err, out)
	}
	return parseSelfInspect(string(out), c.ComposeDir)
}

func parseSelfInspect(raw, composeDir string) (selfInspect, error) {
	line := strings.TrimSpace(raw)
	firstPipe := strings.IndexByte(line, '|')
	if firstPipe < 0 {
		return selfInspect{}, fmt.Errorf("unexpected inspect output: %q", line)
	}

	info := selfInspect{image: line[:firstPipe]}
	if composeDir == "" {
		composeDir = "/app"
	}
	composeFileTarget := path.Join(composeDir, "docker-compose.yml")
	var composeFileSource string
	for _, entry := range strings.Split(line[firstPipe+1:], ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "|", 3)
		if len(parts) != 3 {
			continue
		}
		mount := selfMount{
			destination: parts[0],
			source:      parts[1],
			rw:          parts[2] == "true",
		}
		info.mounts = append(info.mounts, mount)
		if mount.destination == composeDir {
			info.composeHostDir = mount.source
		}
		if mount.destination == composeFileTarget {
			composeFileSource = mount.source
		}
	}
	if info.composeHostDir == "" && composeFileSource != "" {
		info.composeHostDir = path.Dir(composeFileSource)
	}
	return info, nil
}
