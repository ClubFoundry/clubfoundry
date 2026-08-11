package telemetry

import (
	"io"
	"os"
	"path/filepath"
)

// readLog returns the update-log tail within the telemetry payload budget.
func (r *Reporter) readLog(updateID string) string {
	path := filepath.Join(r.logDir(), updateID+".log")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	buf, err := io.ReadAll(f)
	if err != nil || len(buf) == 0 {
		return ""
	}
	const cap = 32 * 1024
	if len(buf) > cap {
		return "...[truncated]...\n" + string(buf[len(buf)-cap:])
	}
	return string(buf)
}
