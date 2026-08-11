package dockerops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SystemDF returns the parsed `docker system df` summary.
func (c Config) SystemDF(ctx context.Context) ([]SystemDFEntry, error) {
	out, err := c.run(ctx, "system", "df", "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("docker system df: %w: %s", err, strings.TrimSpace(string(out)))
	}
	entries := []SystemDFEntry{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e SystemDFEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		e.SizeBytes = parseSizeString(e.Size)
		recl := e.Reclaimable
		if i := strings.Index(recl, " "); i >= 0 {
			recl = recl[:i]
		}
		e.ReclaimableBytes = parseSizeString(recl)
		entries = append(entries, e)
	}
	return entries, nil
}

// FindDFEntry returns the row with typ, or a zero value when absent.
func FindDFEntry(entries []SystemDFEntry, typ string) SystemDFEntry {
	for _, e := range entries {
		if e.Type == typ {
			return e
		}
	}
	return SystemDFEntry{}
}
