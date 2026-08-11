package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/clubfoundry/updater/internal/history"
)

// writeFailureBundle stores a best-effort local post-mortem artifact.
func writeFailureBundle(dataDir string, entry history.Entry, errCode, source, logPath string, logW io.Writer) {
	if dataDir == "" {
		return
	}
	dir := filepath.Join(dataDir, failureBundleDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(logW, "failure-bundle: mkdir %s: %v\n", dir, err)
		return
	}
	bundle := FailureBundle{
		SchemaVersion: failureBundleSchemaVersion,
		UpdateID:      entry.ID, WrittenAt: time.Now().UTC().Format(time.RFC3339),
		FromVersion: entry.FromVersion, ToVersion: entry.ToVersion,
		Outcome: string(entry.Outcome), DurationMS: entry.DurationMS,
		Error: entry.Error, ErrorCode: errCode, LogPath: logPath,
		Source: source, HistoryEntry: entry,
	}
	if logPath != "" {
		if tail, err := readLogTail(logPath, failureBundleMaxLogBytes); err == nil {
			bundle.LogTail = tail
		}
	}
	body, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		fmt.Fprintf(logW, "failure-bundle: marshal: %v\n", err)
		return
	}
	filename := fmt.Sprintf("%s-%d.json", safeFilename(entry.ID), time.Now().Unix())
	path := filepath.Join(dir, filename)
	tmp := path + ".tmp"
	if err := writeFileFsync(tmp, body, 0o644); err != nil {
		fmt.Fprintf(logW, "failure-bundle: write tmp %s: %v\n", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		fmt.Fprintf(logW, "failure-bundle: rename %s: %v\n", path, err)
		return
	}
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	fmt.Fprintf(logW, "failure-bundle: wrote %s (source=%s)\n", path, source)
}
