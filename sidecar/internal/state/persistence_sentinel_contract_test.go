package state

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadPersistedInputContracts(t *testing.T) {
	t.Run("empty and missing", func(t *testing.T) {
		for _, path := range []string{"", filepath.Join(t.TempDir(), "missing.json")} {
			got, err := readPersisted(path)
			if err != nil || got != nil {
				t.Fatalf("readPersisted(%q) = (%+v, %v)", path, got, err)
			}
		}
	})

	t.Run("malformed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readPersisted(path); err == nil || !strings.Contains(err.Error(), "unmarshal state file") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("future schema", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		body := []byte(`{"kind":"main","phase":"idle","schema_version":2}`)
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readPersisted(path); err == nil || !strings.Contains(err.Error(), "sidecar downgrade not supported") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestPanickingChangeHookIsContained(t *testing.T) {
	s := New()
	s.RegisterChangeHook(func(Snapshot) { panic("observer failed") })
	s.UpdateDetail("trigger")
	if err := s.PersistErr(); err == nil || !strings.Contains(err.Error(), "onChange hook panicked: observer failed") {
		t.Fatalf("PersistErr = %v", err)
	}
}

func TestSentinelPathContract(t *testing.T) {
	if SentinelPath("", "tramp-1") != "" || SentinelPath("/data", "") != "" {
		t.Fatal("empty path component must disable sentinel persistence")
	}
	want := filepath.Join("/data", "sidecar-state", "recreate-status", "tramp-1.json")
	if got := SentinelPath("/data", "tramp-1"); got != want {
		t.Fatalf("SentinelPath = %q, want %q", got, want)
	}
}

func TestFinalizeSelfFromSentinelsOutcomeContracts(t *testing.T) {
	cases := []struct {
		name              string
		exitCode          int
		target            string
		current           string
		wantPhase         Phase
		wantLastErrorCode string
	}{
		{name: "success", target: "v3.AH", current: "v3.AH", wantPhase: Idle},
		{name: "version mismatch", target: "v3.AI", current: "v3.AH", wantPhase: Error, wantLastErrorCode: "SELF_UPDATE_VERSION_MISMATCH"},
		{name: "trampoline failure", exitCode: 17, target: "v3.AI", current: "v3.AH", wantPhase: Error, wantLastErrorCode: "SELF_UPDATE_TRAMPOLINE_FAILED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			selfState := activeSelfState(t, dataDir, "op-1")
			path := writeSentinelFixture(t, dataDir, TrampolineSentinel{
				TrampolineID: "tramp-1", TargetVersion: tc.target, Service: "clubfoundry-updater",
				OpID: "op-1", ExitCode: tc.exitCode, CompletedAt: time.Now().UTC().Format(time.RFC3339),
			})

			processed, err := FinalizeSelfFromSentinels(dataDir, tc.current, selfState)
			if err != nil || processed != 1 {
				t.Fatalf("FinalizeSelfFromSentinels = (%d, %v)", processed, err)
			}
			snap := selfState.Snapshot()
			if snap.Phase != tc.wantPhase || snap.LastErrorCode != tc.wantLastErrorCode {
				t.Fatalf("state = %+v, want phase=%s code=%q", snap, tc.wantPhase, tc.wantLastErrorCode)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("processed sentinel remains: %v", err)
			}
		})
	}
}

func TestFinalizeSelfFromSentinelsRejectsUncorrelatedInputs(t *testing.T) {
	t.Run("op id mismatch is discarded", func(t *testing.T) {
		dataDir := t.TempDir()
		selfState := activeSelfState(t, dataDir, "current-op")
		path := writeSentinelFixture(t, dataDir, TrampolineSentinel{
			TrampolineID: "stale", TargetVersion: "v3.AH", OpID: "old-op",
			CompletedAt: time.Now().UTC().Format(time.RFC3339),
		})
		processed, err := FinalizeSelfFromSentinels(dataDir, "v3.AH", selfState)
		if err != nil || processed != 0 || selfState.Snapshot().Phase != Updating {
			t.Fatalf("stale result = (%d, %+v, %v)", processed, selfState.Snapshot(), err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale sentinel remains: %v", err)
		}
	})

	t.Run("malformed json is retained", func(t *testing.T) {
		dataDir := t.TempDir()
		selfState := activeSelfState(t, dataDir, "op-1")
		path := SentinelPath(dataDir, "broken")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		processed, err := FinalizeSelfFromSentinels(dataDir, "v3.AH", selfState)
		if err != nil || processed != 0 {
			t.Fatalf("malformed result = (%d, %v)", processed, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("malformed sentinel was not retained: %v", err)
		}
	})

	t.Run("expired sentinel is swept", func(t *testing.T) {
		dataDir := t.TempDir()
		selfState := activeSelfState(t, dataDir, "op-1")
		path := writeSentinelFixture(t, dataDir, TrampolineSentinel{
			TrampolineID: "expired", TargetVersion: "v3.AH", OpID: "op-1",
			CompletedAt: time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339),
		})
		old := time.Now().Add(-25 * time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		processed, err := FinalizeSelfFromSentinels(dataDir, "v3.AH", selfState)
		if err != nil || processed != 0 {
			t.Fatalf("expired result = (%d, %v)", processed, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expired sentinel remains: %v", err)
		}
	})
}

func TestWaitForSelfFinalizeClosesBootRace(t *testing.T) {
	dataDir := t.TempDir()
	selfState := activeSelfState(t, dataDir, "op-1")
	writeSentinelFixture(t, dataDir, TrampolineSentinel{
		TrampolineID: "race", TargetVersion: "v3.AH", OpID: "op-1",
		CompletedAt: time.Now().UTC().Format(time.RFC3339),
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	WaitForSelfFinalize(ctx, dataDir, "v3.AH", selfState, time.Millisecond)
	if snap := selfState.Snapshot(); snap.Phase != Idle {
		t.Fatalf("state after wait = %+v", snap)
	}
}

func activeSelfState(t *testing.T, dataDir, opID string) *State {
	t.Helper()
	s := NewKindAware(KindSelf, dataDir)
	if err := s.TransitionTo(Updating, "self update"); err != nil {
		t.Fatal(err)
	}
	s.SetOpID(opID)
	return s
}

func writeSentinelFixture(t *testing.T, dataDir string, sentinel TrampolineSentinel) string {
	t.Helper()
	path := SentinelPath(dataDir, sentinel.TrampolineID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
