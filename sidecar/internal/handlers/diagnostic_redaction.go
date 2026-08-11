package handlers

import (
	"regexp"
	"strings"
)

var secretKeyRe = regexp.MustCompile(`(?i)^(\s*[-]?\s*)([A-Z0-9_]*(?:PASSWORD|SECRET|TOKEN|API_KEY|PRIVATE|AUTH|CREDENTIAL|PASS)[A-Z0-9_]*)(\s*[:=]\s*)(.*)$`)

var envFileLineRe = regexp.MustCompile(`(?i)^([A-Z0-9_]*(?:PASSWORD|SECRET|TOKEN|API_KEY|PRIVATE|AUTH|CREDENTIAL|PASS)[A-Z0-9_]*)(\s*=\s*)(.*)$`)

// redactSecrets preserves configuration shape while removing secret values.
func redactSecrets(body []byte) []byte {
	const replacement = "<REDACTED>"
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		if m := secretKeyRe.FindStringSubmatch(line); m != nil {
			lines[i] = m[1] + m[2] + m[3] + replacement
			continue
		}
		if m := envFileLineRe.FindStringSubmatch(line); m != nil {
			lines[i] = m[1] + m[2] + replacement
			continue
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
