package recoveryhistory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func persistError(s *EventStore) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistEr
}

func TestAppendAndRecentSince(t *testing.T) {
	s := NewStore("")
	s.Append(KindRecover, "test1", "1.2.8")
	s.Append(KindReinstall, "test2", "1.2.8")
	s.Append(KindHalt, "test3", "1.2.8")

	all := s.RecentSince(time.Time{})
	if len(all) != 3 {
		t.Fatalf("All() len=%d, want 3", len(all))
	}
	// All should be sorted ascending.
	for i := 1; i < len(all); i++ {
		if all[i].At.Before(all[i-1].At) {
			t.Fatalf("All() not sorted ascending at index %d", i)
		}
	}

	recent := s.RecentSince(time.Now().Add(-1 * time.Minute))
	if len(recent) != 3 {
		t.Fatalf("RecentSince(-1min) len=%d, want 3", len(recent))
	}

	future := s.RecentSince(time.Now().Add(1 * time.Hour))
	if len(future) != 0 {
		t.Fatalf("RecentSince(+1h) len=%d, want 0", len(future))
	}
}

func TestRetentionDropsOldEvents(t *testing.T) {
	s := NewStore("")
	// Inject old events directly to test pruneLocked. Append() always
	// stamps now, so we go through the field for this test only.
	s.events = []Event{
		{Kind: KindRecover, At: time.Now().Add(-31 * 24 * time.Hour), Reason: "old"},
		{Kind: KindRecover, At: time.Now().Add(-29 * 24 * time.Hour), Reason: "barely-kept"},
		{Kind: KindRecover, At: time.Now().Add(-1 * time.Hour), Reason: "fresh"},
	}
	s.Append(KindRecover, "trigger-prune", "1.2.8")
	if len(s.events) != 3 {
		t.Fatalf("after Append+prune len=%d, want 3 (old event dropped)", len(s.events))
	}
	if s.events[0].Reason == "old" {
		t.Fatalf("old event should have been pruned, found: %v", s.events[0])
	}
}

func TestCapEnforced(t *testing.T) {
	s := NewStore("")
	for i := 0; i < maxEvents+50; i++ {
		s.Append(KindRecover, "spam", "1.2.8")
	}
	if len(s.events) > maxEvents {
		t.Fatalf("len=%d exceeds maxEvents=%d", len(s.events), maxEvents)
	}
	if len(s.events) != maxEvents {
		t.Fatalf("len=%d, want exactly %d after spamming", len(s.events), maxEvents)
	}
}

func TestPersistRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)
	s1.Append(KindRecover, "first", "1.2.8")
	s1.Append(KindReinstall, "second", "1.2.8")

	if err := persistError(s1); err != nil {
		t.Fatalf("PersistErr after Append: %v", err)
	}

	// File must exist on disk.
	expectedPath := filepath.Join(dir, "sidecar-state", "recovery-events.json")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected file %s: %v", expectedPath, err)
	}

	// Fresh store re-reading from same dir must see both events.
	s2 := NewStore(dir)
	got := s2.RecentSince(time.Time{})
	if len(got) != 2 {
		t.Fatalf("after restore len=%d, want 2", len(got))
	}
	if got[0].Reason != "first" || got[1].Reason != "second" {
		t.Fatalf("restore order wrong: %+v", got)
	}
}

func TestRestoreIgnoresMissingFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir) // file doesn't exist yet
	if err := persistError(s); err != nil {
		t.Fatalf("missing file should not be an error, got: %v", err)
	}
	if len(s.RecentSince(time.Time{})) != 0 {
		t.Fatalf("missing file should yield empty store")
	}
}

func TestRestoreRejectsForwardIncompatibleSchema(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "sidecar-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schema_version": 999, "events": []}`)
	if err := os.WriteFile(filepath.Join(stateDir, "recovery-events.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(dir)
	if err := persistError(s); err == nil {
		t.Fatalf("expected schema-version error for forward-incompatible file")
	}
	if len(s.RecentSince(time.Time{})) != 0 {
		t.Fatalf("rejected file should yield empty store, got %d events", len(s.RecentSince(time.Time{})))
	}
}

func TestRestoreRejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "sidecar-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "recovery-events.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStore(dir)
	if err := persistError(s); err == nil {
		t.Fatal("expected malformed recovery history to set persistence error")
	}
	if got := s.RecentSince(time.Time{}); len(got) != 0 {
		t.Fatalf("malformed history restored %d events", len(got))
	}
}
