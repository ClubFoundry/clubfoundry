package state

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistedStateJSONContract(t *testing.T) {
	dataDir := t.TempDir()
	st := NewKindAware(KindMain, dataDir)
	if err := st.TransitionTo(Updating, "Installing release"); err != nil {
		t.Fatal(err)
	}
	st.UpdateSubStep(SubStepDownloading, "Downloading image")
	st.SetTarget("1.2.3")
	st.SetPendingMainTarget("1.2.4")
	st.RequestCancel()

	body, err := os.ReadFile(filepath.Join(dataDir, "sidecar-state", "main.json"))
	if err != nil {
		t.Fatalf("read persisted state: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode persisted state: %v", err)
	}
	for _, key := range []string{
		"kind", "phase", "sub_step", "detail", "target_version",
		"pending_main_target", "cancel_requested", "schema_version",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("persisted state missing %q: %s", key, body)
		}
	}
	if _, exists := raw["targetVersion"]; exists {
		t.Errorf("persisted state exposed camelCase key targetVersion: %s", body)
	}
	var schemaVersion int
	if err := json.Unmarshal(raw["schema_version"], &schemaVersion); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != persistSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", schemaVersion, persistSchemaVersion)
	}

	restored := NewKindAware(KindMain, dataDir).Snapshot()
	if restored.Phase != Updating || restored.SubStep != SubStepDownloading {
		t.Fatalf("restored phase = %s/%s, want updating/downloading", restored.Phase, restored.SubStep)
	}
	if restored.TargetVersion != "1.2.3" || restored.PendingMainTarget != "1.2.4" || !restored.CancelRequested {
		t.Fatalf("restored state lost contract fields: %+v", restored)
	}
}

func TestLegalTransitions(t *testing.T) {
	cases := []struct {
		from, to Phase
		ok       bool
	}{
		{Idle, Checking, true},
		{Idle, Updating, true},
		{Idle, RollingBack, true},
		{Idle, Error, false}, // must go via another phase
		{Checking, Updating, true},
		{Checking, Idle, true},
		{Updating, RollingBack, true},
		{Updating, Idle, true},
		{Error, Idle, true},
		{Error, RollingBack, true},
		{Error, Updating, true}, // operator-explicit retry path
		{RollingBack, Idle, true},
		{RollingBack, Updating, false},
	}
	for _, c := range cases {
		if got := legalTransition(c.from, c.to); got != c.ok {
			t.Errorf("legalTransition(%s → %s): got %v, want %v", c.from, c.to, got, c.ok)
		}
	}
}

func TestLegalTransitionMatrix(t *testing.T) {
	phases := []Phase{
		Idle,
		Checking,
		Staging,
		Staged,
		Updating,
		Cancelling,
		RollingBack,
		Error,
	}
	allowed := map[[2]Phase]bool{
		{Idle, Checking}:          true,
		{Idle, Staging}:           true,
		{Idle, Updating}:          true,
		{Idle, RollingBack}:       true,
		{Checking, Idle}:          true,
		{Checking, Updating}:      true,
		{Checking, Error}:         true,
		{Staging, Idle}:           true,
		{Staging, Staged}:         true,
		{Staging, Cancelling}:     true,
		{Staging, Error}:          true,
		{Staged, Idle}:            true,
		{Staged, Updating}:        true,
		{Updating, Idle}:          true,
		{Updating, Cancelling}:    true,
		{Updating, RollingBack}:   true,
		{Updating, Error}:         true,
		{Cancelling, Idle}:        true,
		{Cancelling, RollingBack}: true,
		{Cancelling, Error}:       true,
		{RollingBack, Idle}:       true,
		{RollingBack, Error}:      true,
		{Error, Idle}:             true,
		{Error, Updating}:         true,
		{Error, RollingBack}:      true,
	}

	for _, from := range phases {
		for _, to := range phases {
			want := allowed[[2]Phase{from, to}]
			if got := legalTransition(from, to); got != want {
				t.Errorf("legalTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestTransitionErrorPath(t *testing.T) {
	s := New()
	if err := s.TransitionTo(Updating, "start"); err != nil {
		t.Fatalf("Idle→Updating must succeed: %v", err)
	}
	s.MarkError("SIM", "simulated failure")
	if snap := s.Snapshot(); snap.Phase != Error || snap.LastError != "simulated failure" || snap.LastErrorCode != "SIM" {
		t.Fatalf("MarkError did not surface in snapshot: %+v", snap)
	}
	// Error → RollingBack is allowed; Error → Updating is also allowed (operator retry).
	if err := s.TransitionTo(RollingBack, "restoring"); err != nil {
		t.Fatalf("Error → RollingBack must succeed: %v", err)
	}
	if err := s.TransitionTo(Idle, ""); err != nil {
		t.Fatalf("RollingBack → Idle must succeed: %v", err)
	}
}

// TestPersistRoundTrip verifies that state survives restart. The "restart" is
// modeled by constructing a second NewKindAware with the same dataDir.
func TestPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s1 := NewKindAware(KindMain, dir)
	if err := s1.TransitionTo(Updating, "starting"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	s1.SetUpdateID("upd-test-1")
	s1.SetOpID("op-uuid-1")
	s1.SetTarget("1.1.99")
	s1.UpdateSubStep(SubStepDownloading, "downloading 1.1.99")
	s1.UpdateDownload(DownloadProgress{BytesDownloaded: 100, BytesTotal: 1000})

	// Simulate restart.
	s2 := NewKindAware(KindMain, dir)
	snap := s2.Snapshot()
	if snap.Phase != Updating {
		t.Errorf("phase after restart: got %q, want %q", snap.Phase, Updating)
	}
	if snap.UpdateID != "upd-test-1" {
		t.Errorf("update id after restart: got %q", snap.UpdateID)
	}
	if snap.OpID != "op-uuid-1" {
		t.Errorf("op id after restart: got %q", snap.OpID)
	}
	if snap.TargetVersion != "1.1.99" {
		t.Errorf("target after restart: got %q", snap.TargetVersion)
	}
	if snap.SubStep != SubStepDownloading {
		t.Errorf("sub step after restart: got %q", snap.SubStep)
	}
	// The byte counter deliberately does NOT survive a restart — see
	// UpdateDownload. What matters (an update was in flight, and it was in
	// the download step) is carried by Phase + SubStep, which do survive.
	if snap.Download != nil {
		t.Errorf("download must not be persisted, got %+v", snap.Download)
	}
}

// TestUpdateDownloadDoesNotTouchDisk verifies that the twice-per-second
// reporter cannot fsync while holding the mutex needed by /status. Samples
// must reach in-memory observers without changing the durable state file.
func TestUpdateDownloadDoesNotTouchDisk(t *testing.T) {
	dir := t.TempDir()
	s := NewKindAware(KindMain, dir)
	if err := s.TransitionTo(Updating, "starting"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	s.UpdateSubStep(SubStepDownloading, "downloading")

	path := stateFilePath(dir, KindMain)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	var hookFired int
	s.RegisterChangeHook(func(Snapshot) { hookFired++ })
	s.UpdateDownload(DownloadProgress{BytesDownloaded: 100, BytesTotal: 1000, BytesPerSecond: 50})

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read state file: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("UpdateDownload wrote the state file — the 2 Hz path must never fsync under s.mu")
	}
	if hookFired != 1 {
		t.Errorf("change hook fired %d times, want 1 — /status observers still need the sample", hookFired)
	}
	if snap := s.Snapshot(); snap.Download == nil || snap.Download.BytesDownloaded != 100 {
		t.Errorf("sample must be readable in memory, got %+v", snap.Download)
	}
}

// TestKindIsolation verifies that writes to KindMain must not
// pollute KindSelf and vice versa, even when both share a data directory.
func TestKindIsolation(t *testing.T) {
	dir := t.TempDir()
	mainSt := NewKindAware(KindMain, dir)
	selfSt := NewKindAware(KindSelf, dir)

	if err := mainSt.TransitionTo(Updating, "main op"); err != nil {
		t.Fatalf("main transition: %v", err)
	}
	mainSt.SetTarget("1.1.99")
	mainSt.SetOpID("main-op-uuid")

	// selfSt should see Idle still.
	if snap := selfSt.Snapshot(); snap.Phase != Idle || snap.TargetVersion != "" {
		t.Errorf("kind isolation broken: self-state polluted by main: %+v", snap)
	}

	// Now self does its own op.
	if err := selfSt.TransitionTo(Updating, "self op"); err != nil {
		t.Fatalf("self transition: %v", err)
	}
	selfSt.SetTarget("v1.M")
	selfSt.SetOpID("self-op-uuid")

	// mainSt's target must still be 1.1.99 — not "v1.M".
	if snap := mainSt.Snapshot(); snap.TargetVersion != "1.1.99" {
		t.Errorf("kind isolation broken: main-state target overwritten by self: got %q want %q", snap.TargetVersion, "1.1.99")
	}
	if snap := mainSt.Snapshot(); snap.OpID != "main-op-uuid" {
		t.Errorf("kind isolation broken: main op_id overwritten by self: got %q", snap.OpID)
	}

	// Restart simulation — both files on disk; new constructions must
	// NOT cross-load.
	mainSt2 := NewKindAware(KindMain, dir)
	selfSt2 := NewKindAware(KindSelf, dir)
	if snap := mainSt2.Snapshot(); snap.TargetVersion != "1.1.99" {
		t.Errorf("post-restart main target wrong: %q", snap.TargetVersion)
	}
	if snap := selfSt2.Snapshot(); snap.TargetVersion != "v1.M" {
		t.Errorf("post-restart self target wrong: %q", snap.TargetVersion)
	}
}

func TestForceResetRemovesFile(t *testing.T) {
	dir := t.TempDir()
	s := NewKindAware(KindMain, dir)
	_ = s.TransitionTo(Updating, "x")
	s.SetTarget("1.0.0")

	path := filepath.Join(dir, "sidecar-state", "main.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file to exist before ForceReset: %v", err)
	}

	s.ForceReset()
	if snap := s.Snapshot(); snap.Phase != Idle || snap.TargetVersion != "" {
		t.Errorf("ForceReset did not clear in-memory state: %+v", snap)
	}
	// After ForceReset the file is rewritten as Idle (writePersist runs at
	// the tail of the call). Either it exists with Phase=idle OR it's
	// absent because the os.Remove between erase and re-write happened to
	// land that way — both are acceptable; the contract is "no operator-
	// visible non-Idle state survives".
	if body, err := os.ReadFile(path); err == nil {
		if !contains(string(body), `"phase": "idle"`) {
			t.Errorf("post-ForceReset file does not record Idle: %s", string(body))
		}
	}
}

// contains is a tiny substring helper to avoid pulling strings into the
// test for one call.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestPendingMainTargetRoundTrip — sidecar-first transparent flow:
// pendingMainTarget set on the main-kind State by the /update handler
// must survive a sidecar restart (the trampoline kills the original
// sidecar before the chained main-update fires). Reset preserves the
// queued target; ForceReset wipes it.
func TestPendingMainTargetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s1 := NewKindAware(KindMain, dir)
	s1.SetPendingMainTarget("1.1.99")
	if got := s1.PendingMainTarget(); got != "1.1.99" {
		t.Fatalf("PendingMainTarget after Set: got %q, want %q", got, "1.1.99")
	}

	// Simulate restart: new instance reads the persisted file.
	s2 := NewKindAware(KindMain, dir)
	if got := s2.PendingMainTarget(); got != "1.1.99" {
		t.Fatalf("PendingMainTarget after restart: got %q, want %q", got, "1.1.99")
	}
	if snap := s2.Snapshot(); snap.PendingMainTarget != "1.1.99" {
		t.Fatalf("Snapshot.PendingMainTarget after restart: got %q", snap.PendingMainTarget)
	}

	// Reset preserves pendingMainTarget — operator dismissing an error
	// banner shouldn't drop the queued chained main-update.
	_ = s2.TransitionTo(Updating, "x")
	s2.MarkError("SIM", "sim")
	s2.Reset()
	if got := s2.PendingMainTarget(); got != "1.1.99" {
		t.Errorf("Reset must preserve pendingMainTarget: got %q", got)
	}

	// Clear path — explicit Clear wipes only this field.
	s2.SetTarget("1.1.99")
	s2.ClearPendingMainTarget()
	if got := s2.PendingMainTarget(); got != "" {
		t.Errorf("ClearPendingMainTarget did not wipe field: got %q", got)
	}
	if snap := s2.Snapshot(); snap.TargetVersion != "1.1.99" {
		t.Errorf("ClearPendingMainTarget must not touch other fields: TargetVersion=%q", snap.TargetVersion)
	}

	// ForceReset wipes pendingMainTarget too.
	s2.SetPendingMainTarget("1.2.0")
	s2.ForceReset()
	if got := s2.PendingMainTarget(); got != "" {
		t.Errorf("ForceReset must wipe pendingMainTarget: got %q", got)
	}
}

func TestRejectWrongKindFile(t *testing.T) {
	// Manually drop a self-kind file at the main path. NewKindAware should
	// refuse to import it (defensive against operator file-rename mishaps).
	dir := t.TempDir()
	statePath := filepath.Join(dir, "sidecar-state", "main.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := []byte(`{"kind": "self", "phase": "updating", "target_version": "v1.X", "schema_version": 1}`)
	if err := os.WriteFile(statePath, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := NewKindAware(KindMain, dir)
	if snap := s.Snapshot(); snap.Phase != Idle {
		t.Errorf("kind-mismatch import was not rejected: %+v", snap)
	}
	if s.PersistErr() == nil {
		t.Errorf("expected PersistErr to surface kind mismatch")
	}
}

// TestTransitionToIdleClearsLastErr verifies that Idle means no current
// operation and no current failure. Completed errors belong in history.
func TestTransitionToIdleClearsLastErr(t *testing.T) {
	s := New()
	if err := s.TransitionTo(Updating, ""); err != nil {
		t.Fatalf("Idle→Updating: %v", err)
	}
	s.MarkError("SIM", "simulated failure")
	if snap := s.Snapshot(); snap.LastError == "" {
		t.Fatalf("MarkError did not set lastErr: %+v", snap)
	}
	if err := s.TransitionTo(Idle, "operator dismissed"); err != nil {
		t.Fatalf("Error→Idle: %v", err)
	}
	if snap := s.Snapshot(); snap.LastError != "" || snap.LastErrorCode != "" {
		t.Fatalf("Idle still carries stale lastErr: %+v", snap)
	}
}

// TestRestoreBootInvariantIdleClearsLastErr verifies that an inconsistent
// persisted Idle state cannot restore a stale last_error.
func TestRestoreBootInvariantIdleClearsLastErr(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "sidecar-state", "main.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a synthetic inconsistent file: phase=idle but lastErr present.
	body := []byte(`{"kind":"main","phase":"idle","last_error":{"code":"OLD_FAILURE","message":"happened ages ago"},"schema_version":1}`)
	if err := os.WriteFile(statePath, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := NewKindAware(KindMain, dir)
	if snap := s.Snapshot(); snap.Phase != Idle {
		t.Fatalf("expected restored phase=Idle, got %s", snap.Phase)
	}
	if snap := s.Snapshot(); snap.LastError != "" || snap.LastErrorCode != "" {
		t.Fatalf("boot invariant did not clear stale lastErr: %+v", snap)
	}
	// Verify the next persisted write also writes the clean shape.
	s.UpdateDetail("trigger persist")
	body2, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if got := string(body2); got != "" && (containsAll(got, []string{`"code": "OLD_FAILURE"`})) {
		t.Errorf("on-disk file still has stale code after first persist: %s", got)
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !containsString(s, sub) {
			return false
		}
	}
	return true
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestAutoSoftRecoverNotInError — only fires from Error.
func TestAutoSoftRecoverNotInError(t *testing.T) {
	s := New()
	if s.AutoSoftRecoverIfStuck(1 * time.Nanosecond) {
		t.Errorf("AutoSoftRecover fired from Idle — should be no-op")
	}
}

// TestAutoSoftRecoverWindowGate — does not fire if duration < window.
func TestAutoSoftRecoverWindowGate(t *testing.T) {
	s := New()
	if err := s.TransitionTo(Updating, ""); err != nil {
		t.Fatalf("transition: %v", err)
	}
	s.MarkError("SIM", "test")
	// Window 1 hour, time since entry ~0 → must NOT fire.
	if s.AutoSoftRecoverIfStuck(1 * time.Hour) {
		t.Errorf("AutoSoftRecover fired before window — got premature recovery")
	}
	// Confirm still in Error.
	if snap := s.Snapshot(); snap.Phase != Error {
		t.Errorf("state moved out of Error during failed recovery: %s", snap.Phase)
	}
}

// TestAutoSoftRecoverFiresAfterWindow — flips to Idle when duration met.
func TestAutoSoftRecoverFiresAfterWindow(t *testing.T) {
	s := New()
	if err := s.TransitionTo(Updating, ""); err != nil {
		t.Fatalf("transition: %v", err)
	}
	s.MarkError("SIM", "old failure")
	// Sleep past the test window so time.Since(started) > 1ms when we check.
	// time.Now() resolution on Windows is low (~10ms) but a 50ms sleep is
	// safely above that floor without slowing the suite meaningfully.
	time.Sleep(50 * time.Millisecond)
	if !s.AutoSoftRecoverIfStuck(1 * time.Millisecond) {
		t.Fatalf("AutoSoftRecover did not fire when window=1ms")
	}
	snap := s.Snapshot()
	if snap.Phase != Idle {
		t.Errorf("expected Idle after auto-recover, got %s", snap.Phase)
	}
	if snap.LastError != "" || snap.LastErrorCode != "" {
		t.Errorf("auto-recover did not clear lastErr: %+v", snap)
	}
	if snap.Detail == "" {
		t.Errorf("auto-recover should set a Detail describing the recovery, got empty")
	}
	// Second call is no-op (already Idle).
	if s.AutoSoftRecoverIfStuck(1 * time.Nanosecond) {
		t.Errorf("second AutoSoftRecover call fired — should be no-op when not in Error")
	}
}
