// Tests for diagnostic artifact logging, archival, and retention.
// retention sweep (count + age + bytes + flat-json), and the archive
// copy. The most-risky pieces — anything that DELETES files — gets the
// densest coverage (false positive = data loss).
//
// Tests use t.TempDir() so no manual cleanup needed; each test gets an
// isolated dataDir and the OS cleans up.
package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/state"
)

// --- SweepDiagnosticRetention --------------------------------------

func TestSweepDiagnosticRetention_EmptyDataDirNoOp(t *testing.T) {
	// Should not panic, should not error. Just exercises the early-return.
	SweepDiagnosticRetention("")
}

func TestSweepDiagnosticRetention_MissingDirsNoOp(t *testing.T) {
	dataDir := t.TempDir()
	// dataDir exists but neither update-logs/ nor update-failures/ — sweep
	// must walk both gracefully.
	SweepDiagnosticRetention(dataDir)
}

func TestSweepDiagnosticRetention_KeepsNewestSuccessLogs(t *testing.T) {
	dataDir := t.TempDir()
	logs := filepath.Join(dataDir, "update-logs")
	mustMkdir(t, logs)
	// Create retainSuccessLogs+3 fake update dirs with staggered mtimes.
	// The 3 oldest should be deleted; the retainSuccessLogs newest survive.
	keep := retainSuccessLogs
	totalDirs := keep + 3
	now := time.Now()
	for i := 0; i < totalDirs; i++ {
		name := "upd-" + pad(i)
		dir := filepath.Join(logs, name)
		mustMkdir(t, dir)
		mustWrite(t, filepath.Join(dir, "update.log"), []byte("payload "+name))
		// Older index = older mtime.
		mtime := now.Add(time.Duration(-(totalDirs - i)) * time.Hour)
		setMtime(t, dir, mtime)
	}
	SweepDiagnosticRetention(dataDir)
	remaining := listSubdirs(t, logs)
	if len(remaining) != keep {
		t.Fatalf("expected %d dirs remaining, got %d: %v", keep, len(remaining), remaining)
	}
	// Check that the surviving names are the highest-index ones (= newest).
	for i := 0; i < keep; i++ {
		expected := "upd-" + pad(totalDirs-1-i)
		found := false
		for _, n := range remaining {
			if n == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected surviving dir %q in remaining=%v", expected, remaining)
		}
	}
}

func TestSweepDiagnosticRetention_KeepsNewestFailureBundles(t *testing.T) {
	dataDir := t.TempDir()
	failures := filepath.Join(dataDir, "update-failures")
	mustMkdir(t, failures)
	keep := retainFailureBundles
	totalDirs := keep + 4
	now := time.Now()
	for i := 0; i < totalDirs; i++ {
		dir := filepath.Join(failures, "fail-"+pad(i))
		mustMkdir(t, dir)
		mustWrite(t, filepath.Join(dir, "x.txt"), []byte("x"))
		setMtime(t, dir, now.Add(time.Duration(-(totalDirs-i))*time.Hour))
	}
	SweepDiagnosticRetention(dataDir)
	remaining := listSubdirs(t, failures)
	if len(remaining) != keep {
		t.Fatalf("expected %d failure dirs remaining, got %d: %v", keep, len(remaining), remaining)
	}
}

// Flat failure-bundle JSON files count toward retention alongside directories.
func TestSweepDiagnosticRetention_PrunesFlatJsonFiles(t *testing.T) {
	dataDir := t.TempDir()
	failures := filepath.Join(dataDir, "update-failures")
	mustMkdir(t, failures)
	keep := retainFailureBundles
	// Create retainFailureBundles+5 flat *.json files (no dirs).
	totalFiles := keep + 5
	now := time.Now()
	for i := 0; i < totalFiles; i++ {
		path := filepath.Join(failures, "bundle-"+pad(i)+".json")
		mustWrite(t, path, []byte("{}"))
		setMtime(t, path, now.Add(time.Duration(-(totalFiles-i))*time.Hour))
	}
	SweepDiagnosticRetention(dataDir)
	remaining := listFiles(t, failures)
	if len(remaining) != keep {
		t.Fatalf("expected %d flat-json files remaining, got %d: %v", keep, len(remaining), remaining)
	}
}

func TestSweepDiagnosticRetention_AgeCutoffDeletesOld(t *testing.T) {
	dataDir := t.TempDir()
	logs := filepath.Join(dataDir, "update-logs")
	mustMkdir(t, logs)
	// One ancient dir (>retainMaxAge old) + one recent. After sweep, only
	// the recent survives.
	old := filepath.Join(logs, "upd-old")
	recent := filepath.Join(logs, "upd-recent")
	mustMkdir(t, old)
	mustMkdir(t, recent)
	mustWrite(t, filepath.Join(old, "update.log"), []byte("ancient"))
	mustWrite(t, filepath.Join(recent, "update.log"), []byte("fresh"))
	now := time.Now()
	setMtime(t, old, now.Add(-(retainMaxAge + 24*time.Hour)))
	setMtime(t, recent, now.Add(-time.Hour))

	SweepDiagnosticRetention(dataDir)

	if exists(filepath.Join(logs, "upd-old")) {
		t.Errorf("expected upd-old (older than %s) to be deleted", retainMaxAge)
	}
	if !exists(filepath.Join(logs, "upd-recent")) {
		t.Errorf("expected upd-recent (within retainMaxAge) to survive")
	}
}

func TestSweepDiagnosticRetention_AgeCutoffDeletesOldFlatFile(t *testing.T) {
	dataDir := t.TempDir()
	failures := filepath.Join(dataDir, "update-failures")
	mustMkdir(t, failures)
	oldFile := filepath.Join(failures, "bundle-old.json")
	recentFile := filepath.Join(failures, "bundle-recent.json")
	mustWrite(t, oldFile, []byte("{}"))
	mustWrite(t, recentFile, []byte("{}"))
	now := time.Now()
	setMtime(t, oldFile, now.Add(-(retainMaxAge + 24*time.Hour)))
	setMtime(t, recentFile, now.Add(-time.Hour))

	SweepDiagnosticRetention(dataDir)

	if exists(oldFile) {
		t.Errorf("expected old flat *.json to be age-cutoff deleted")
	}
	if !exists(recentFile) {
		t.Errorf("expected recent flat *.json to survive")
	}
}

// --- ArchiveUpdateLogToFailures -------------------------------------

func TestArchiveUpdateLogToFailures_CopiesAllRegularFiles(t *testing.T) {
	dataDir := t.TempDir()
	updateID := "upd-test-1"
	src := filepath.Join(dataDir, "update-logs", updateID)
	mustMkdir(t, src)
	mustWrite(t, filepath.Join(src, "update.log"), []byte("text-log"))
	mustWrite(t, filepath.Join(src, "phases.jsonl"), []byte(`{"phase":"updating"}`+"\n"))
	mustWrite(t, filepath.Join(src, "state-pre.json"), []byte(`{"phase":"idle"}`))

	ArchiveUpdateLogToFailures(dataDir, updateID, "test_source", nil)

	failures := filepath.Join(dataDir, "update-failures")
	dirs := listSubdirs(t, failures)
	if len(dirs) != 1 {
		t.Fatalf("expected exactly one archived dir, got %v", dirs)
	}
	dst := filepath.Join(failures, dirs[0])
	for _, name := range []string{"update.log", "phases.jsonl", "state-pre.json", "SOURCE.txt"} {
		if !exists(filepath.Join(dst, name)) {
			t.Errorf("expected %s in archive %s", name, dst)
		}
	}
	// SOURCE.txt should contain the source tag (with trailing newline).
	body, err := os.ReadFile(filepath.Join(dst, "SOURCE.txt"))
	if err != nil {
		t.Fatalf("read SOURCE.txt: %v", err)
	}
	if strings.TrimSpace(string(body)) != "test_source" {
		t.Errorf("SOURCE.txt = %q, want %q", string(body), "test_source")
	}
	// Content of update.log must match exactly.
	body, _ = os.ReadFile(filepath.Join(dst, "update.log"))
	if string(body) != "text-log" {
		t.Errorf("update.log content = %q, want %q", string(body), "text-log")
	}
}

func TestArchiveUpdateLogToFailures_NoOpMissingSource(t *testing.T) {
	dataDir := t.TempDir()
	// Source dir doesn't exist. Should not error, should not create dst.
	ArchiveUpdateLogToFailures(dataDir, "upd-missing", "x", nil)
	if exists(filepath.Join(dataDir, "update-failures")) {
		t.Errorf("expected no failures dir to be created when source missing")
	}
}

func TestArchiveUpdateLogToFailures_NoOpEmptyDataDir(t *testing.T) {
	ArchiveUpdateLogToFailures("", "upd-x", "x", nil)
	ArchiveUpdateLogToFailures("/tmp/foo", "", "x", nil)
}

// --- updateLog (open + hookFn + phase append) -----------------------

func TestUpdateLog_HookFnEmitsPhaseEvents(t *testing.T) {
	dataDir := t.TempDir()
	rootDir := filepath.Join(dataDir, "update-logs")
	updateID := "upd-hook-1"
	opID := "op-deadbeef"

	ulog := openUpdateLog(rootDir, updateID, opID, state.KindMain)
	defer ulog.close()
	if ulog.path() == "" {
		t.Fatalf("expected non-empty path after open")
	}
	if ulog.logFilePath() == "" {
		t.Fatalf("expected non-empty LogFilePath")
	}

	hook := ulog.hookFn()
	hook(state.Snapshot{
		Phase:    state.Updating,
		SubStep:  state.SubStepDownloading,
		Detail:   "Downloading 1.1.89",
		UpdateID: updateID,
	})
	hook(state.Snapshot{
		Phase:   state.Updating,
		SubStep: state.SubStepStarting,
		Detail:  "Starting new version",
	})
	ulog.close()

	body, err := os.ReadFile(filepath.Join(ulog.path(), "phases.jsonl"))
	if err != nil {
		t.Fatalf("read phases.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 phase events, got %d: %q", len(lines), string(body))
	}
	for i, line := range lines {
		var ev phaseEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d not valid JSON: %v: %q", i, err, line)
		}
		if ev.SchemaVersion != phaseEventSchemaVersion {
			t.Errorf("line %d schema_version = %d, want %d", i, ev.SchemaVersion, phaseEventSchemaVersion)
		}
		if ev.UpdateID != updateID {
			t.Errorf("line %d update_id = %q, want %q", i, ev.UpdateID, updateID)
		}
		if ev.OpID != opID {
			t.Errorf("line %d op_id = %q, want %q", i, ev.OpID, opID)
		}
		if ev.Phase != string(state.Updating) {
			t.Errorf("line %d phase = %q, want updating", i, ev.Phase)
		}
	}
}

func TestUpdateLog_NilSafe(t *testing.T) {
	// Nil-receiver path — a logging defect MUST NEVER fail an update.
	var u *updateLog
	u.appendPhaseFromSnapshot(state.Snapshot{}, nil)
	u.appendPhaseExtras(state.Updating, state.SubStepDownloading, "x", nil)
	u.writeStateSnapshot("state-pre", state.Snapshot{})
	if u.path() != "" {
		t.Errorf("nil ulog path = %q, want empty", u.path())
	}
	if u.logFilePath() != "" {
		t.Errorf("nil ulog logFilePath = %q, want empty", u.logFilePath())
	}
	hook := u.hookFn()
	hook(state.Snapshot{}) // must not panic
	u.close()              // must not panic
}

func TestOpenUpdateLog_DiscardModeOnEmptyArgs(t *testing.T) {
	// rootDir empty → discard mode (text=nil, phases=nil).
	u := openUpdateLog("", "upd-x", "op-x", state.KindMain)
	if u == nil {
		t.Fatalf("expected non-nil update log even in discard mode")
	}
	if u.path() != "" {
		t.Errorf("expected empty path in discard mode, got %q", u.path())
	}
	// Methods must not panic.
	u.hookFn()(state.Snapshot{Phase: state.Updating})
	u.appendPhaseExtras(state.Idle, "", "x", nil)
	u.writeStateSnapshot("state-pre", state.Snapshot{})
	u.close()
}

// --- Helpers --------------------------------------------------------

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", p, err)
	}
}

func mustWrite(t *testing.T, p string, body []byte) {
	t.Helper()
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", p, err)
	}
}

func setMtime(t *testing.T, p string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatalf("Chtimes %s: %v", p, err)
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func listSubdirs(t *testing.T, parent string) []string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", parent, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func listFiles(t *testing.T, parent string) []string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", parent, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// pad is a tiny zero-pad for stable lexicographic ordering of test
// dirs (so listSubdirs returns a predictable shape regardless of OS
// file-system ordering).
func pad(i int) string {
	if i < 10 {
		return "0" + itoa(i)
	}
	return itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
