package updater

import (
	"io"
	"os"
	"path/filepath"
	"time"
)

func updateLogsDir(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "update-logs")
}

func updateFailuresDir(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "update-failures")
}

// ArchiveUpdateLogToFailures preserves one operation's diagnostic artifacts
// under update-failures. Archival is best-effort and never gates recovery.
func ArchiveUpdateLogToFailures(dataDir, updateID, source string, logW io.Writer) {
	if dataDir == "" || updateID == "" {
		return
	}
	sourceDir := filepath.Join(updateLogsDir(dataDir), safeFilename(updateID))
	if _, err := os.Stat(sourceDir); err != nil {
		return
	}

	failuresDir := updateFailuresDir(dataDir)
	if err := os.MkdirAll(failuresDir, 0o755); err != nil {
		writeArchiveLog(logW, "archive: mkdir %s: %v", failuresDir, err)
		return
	}
	destination := filepath.Join(
		failuresDir,
		time.Now().UTC().Format("20060102T150405Z")+"-"+safeFilename(updateID),
	)
	if err := copyDirShallow(sourceDir, destination); err != nil {
		writeArchiveLog(logW, "archive: copy %s → %s: %v", sourceDir, destination, err)
		return
	}
	if source != "" {
		_ = os.WriteFile(filepath.Join(destination, "SOURCE.txt"), []byte(source+"\n"), 0o644)
	}
	writeArchiveLog(logW, "archive: %s → %s (source=%s)", sourceDir, destination, source)
}
