package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

type dockerPSLine struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	State  string `json:"State"`
	Labels string `json:"Labels"`
}

func (l dockerPSLine) composeProject() string {
	for _, kv := range strings.Split(l.Labels, ",") {
		kv = strings.TrimSpace(kv)
		const prefix = "com.docker.compose.project="
		if strings.HasPrefix(kv, prefix) {
			return strings.TrimPrefix(kv, prefix)
		}
	}
	return ""
}

func listContainersByServiceLabel(ctx context.Context, dockerBin, service string) ([]dockerPSLine, error) {
	cmd := exec.CommandContext(ctx, dockerBin, "ps", "-a",
		"--filter", "label=com.docker.compose.service="+service,
		"--format", "{{json .}}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	var results []dockerPSLine
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var l dockerPSLine
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			log.Printf("[compose-drift] skip malformed ps line: %v: %q", err, line)
			continue
		}
		results = append(results, l)
	}
	return results, nil
}
