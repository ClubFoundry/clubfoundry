package dockerops

import "strings"

// findServiceImageLine locates the first image field in one service block.
func findServiceImageLine(raw, service string) (lineIdx int, indent, value string, found bool) {
	lines := strings.Split(raw, "\n")
	header := service + ":"
	svcLineIdx := -1
	svcIndentLen := -1
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == header {
			svcLineIdx = i
			svcIndentLen = len(line) - len(trimmed)
			break
		}
	}
	if svcLineIdx < 0 {
		return 0, "", "", false
	}
	for i := svcLineIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimLeft(line, " "), "#") {
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		thisIndent := len(line) - len(trimmed)
		if thisIndent <= svcIndentLen {
			return 0, "", "", false
		}
		if strings.HasPrefix(trimmed, "image:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "image:"))
			// Compose accepts either quoted or unquoted image values.
			val = strings.Trim(val, "\"'")
			return i, strings.Repeat(" ", thisIndent), val, true
		}
	}
	return 0, "", "", false
}
