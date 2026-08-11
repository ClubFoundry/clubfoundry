package updater

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

func TestWriteFailureBundleContract(t *testing.T) {
	dataDir := t.TempDir()
	logPath := filepath.Join(dataDir, "source.log")
	logBody := strings.Repeat("prefix", 1000) + strings.Repeat("z", failureBundleMaxLogBytes)
	if err := os.WriteFile(logPath, []byte(logBody), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := history.Entry{
		ID: "../upd bad", FromVersion: "1.3.137", ToVersion: "1.3.138",
		Outcome: history.OutcomeRollback, DurationMS: 42_000, Error: "health failed",
	}
	var logs bytes.Buffer
	writeFailureBundle(dataDir, entry, "HEALTH_TIMEOUT", "rollback", logPath, &logs)

	dir := updateFailuresDir(dataDir)
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].IsDir() {
		t.Fatalf("failure dir = (%v, %v)", entries, err)
	}
	if !strings.HasPrefix(entries[0].Name(), "___upd_bad-") || !strings.HasSuffix(entries[0].Name(), ".json") {
		t.Fatalf("filename = %q", entries[0].Name())
	}
	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var bundle FailureBundle
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.SchemaVersion != failureBundleSchemaVersion || bundle.UpdateID != entry.ID || bundle.ErrorCode != "HEALTH_TIMEOUT" || bundle.Source != "rollback" {
		t.Fatalf("bundle identity = %+v", bundle)
	}
	if bundle.HistoryEntry.ID != entry.ID || bundle.LogPath != logPath {
		t.Fatalf("bundle nested fields = %+v", bundle)
	}
	if len(bundle.LogTail) != failureBundleMaxLogBytes || strings.Trim(bundle.LogTail, "z") != "" {
		t.Fatalf("log tail length/content mismatch: %d", len(bundle.LogTail))
	}
	if _, err := time.Parse(time.RFC3339, bundle.WrittenAt); err != nil {
		t.Fatalf("written_at = %q: %v", bundle.WrittenAt, err)
	}
	if !strings.Contains(logs.String(), "failure-bundle: wrote") || !strings.Contains(logs.String(), "source=rollback") {
		t.Fatalf("log output = %q", logs.String())
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestArtifactFilenameAndLogTailContracts(t *testing.T) {
	for input, want := range map[string]string{
		"upd-123":   "upd-123",
		"../bad id": "___bad_id",
		"":          "unknown",
	} {
		if got := safeFilename(input); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", input, got, want)
		}
	}
	path := filepath.Join(t.TempDir(), "log.txt")
	if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := readLogTail(path, 20); err != nil || got != "0123456789" {
		t.Fatalf("full tail = (%q, %v)", got, err)
	}
	if got, err := readLogTail(path, 4); err != nil || got != "6789" {
		t.Fatalf("bounded tail = (%q, %v)", got, err)
	}
}

func TestOpenUpdateLogMigratesLegacyFlatLog(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "upd-legacy.log")
	if err := os.WriteFile(legacy, []byte("legacy line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := openUpdateLog(root, "upd-legacy", "op-1", state.KindMain)
	_, _ = log.writer().Write([]byte("new line\n"))
	log.close()
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file remains: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "upd-legacy", "update.log"))
	if err != nil || string(body) != "legacy line\nnew line\n" {
		t.Fatalf("migrated log = (%q, %v)", body, err)
	}
}

func TestUpdateLogStateSnapshotIsAtomic(t *testing.T) {
	root := t.TempDir()
	log := openUpdateLog(root, "upd-state", "op-1", state.KindMain)
	defer log.close()
	log.writeStateSnapshot("state-pre", state.Snapshot{Kind: state.KindMain, Phase: state.Updating, TargetVersion: "1.3.138"})
	path := filepath.Join(log.path(), "state-pre.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snap state.Snapshot
	if err := json.Unmarshal(body, &snap); err != nil || snap.Phase != state.Updating || snap.TargetVersion != "1.3.138" {
		t.Fatalf("snapshot = (%+v, %v)", snap, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("snapshot temporary remains: %v", err)
	}
}

func TestPruneByBytesDeletesOldestAcrossArtifactKinds(t *testing.T) {
	dataDir := t.TempDir()
	oldLog := filepath.Join(updateLogsDir(dataDir), "old", "update.log")
	newLog := filepath.Join(updateLogsDir(dataDir), "new", "update.log")
	failure := filepath.Join(updateFailuresDir(dataDir), "middle.json")
	for _, path := range []string{oldLog, newLog, failure} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 80), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(filepath.Dir(oldLog), now.Add(-3*time.Hour), now.Add(-3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(failure, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Dir(newLog), now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	pruneByBytes(dataDir, 160)
	if _, err := os.Stat(filepath.Dir(oldLog)); !os.IsNotExist(err) {
		t.Fatalf("oldest artifact remains: %v", err)
	}
	for _, path := range []string{failure, filepath.Dir(newLog)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("newer artifact %s was removed: %v", path, err)
		}
	}
}

func TestArchiveRemainsShallow(t *testing.T) {
	dataDir := t.TempDir()
	source := filepath.Join(updateLogsDir(dataDir), "upd-shallow")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "update.log"), []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "ignored.log"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	ArchiveUpdateLogToFailures(dataDir, "upd-shallow", "contract", &bytes.Buffer{})
	dirs, err := os.ReadDir(updateFailuresDir(dataDir))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("archive dirs = (%v, %v)", dirs, err)
	}
	destination := filepath.Join(updateFailuresDir(dataDir), dirs[0].Name())
	if _, err := os.Stat(filepath.Join(destination, "update.log")); err != nil {
		t.Fatalf("regular file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "nested")); !os.IsNotExist(err) {
		t.Fatalf("nested directory was copied: %v", err)
	}
}
