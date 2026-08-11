package bootstrap

import (
	"fmt"
	"os"
	"strings"
)

// EnsureProjectName adds a top-level Compose project anchor when absent.
func EnsureProjectName(path, projectName string) (bool, error) {
	if projectName == "" {
		return false, fmt.Errorf("EnsureProjectName: empty projectName")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	lines := strings.Split(string(raw), "\n")
	servicesIdx := -1
	for i, line := range lines {
		if len(line) == 0 || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		if strings.HasPrefix(line, "name:") {
			return false, nil
		}
		if strings.HasPrefix(line, "services:") {
			servicesIdx = i
			break
		}
	}
	if servicesIdx < 0 {
		return false, fmt.Errorf("EnsureProjectName: no top-level `services:` block found in %s", path)
	}

	// Keep one best-effort pre-migration copy for operator recovery.
	bakPath := path + ".bak-" + projectName + "-anchor"
	if _, statErr := os.Stat(bakPath); os.IsNotExist(statErr) {
		_ = os.WriteFile(bakPath, raw, 0o644)
	}

	anchor := []string{
		"# Project name anchor (added by sidecar bootstrap migration).",
		"# Compose v2.4+ uses this to anchor project name across invocation",
		"# contexts. See comment in template for full rationale.",
		"name: " + projectName,
		"",
	}
	out := make([]string, 0, len(lines)+len(anchor))
	out = append(out, lines[:servicesIdx]...)
	out = append(out, anchor...)
	out = append(out, lines[servicesIdx:]...)

	body := []byte(strings.Join(out, "\n"))
	if err := atomicWrite(path, body, 0o644); err != nil {
		return false, fmt.Errorf("rewrite %s: %w", path, err)
	}
	return true, nil
}
