package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndList(t *testing.T) {
	dir := t.TempDir()
	l := New(filepath.Join(dir, "history.json"))

	base := time.Now().Add(-time.Hour)
	for i := 0; i < 3; i++ {
		err := l.Append(Entry{
			ID:          "run-" + string(rune('a'+i)),
			StartedAt:   base.Add(time.Duration(i) * time.Minute),
			FinishedAt:  base.Add(time.Duration(i)*time.Minute + 30*time.Second),
			DurationMS:  30_000,
			FromVersion: "1.0.20",
			ToVersion:   "1.0.21",
			Outcome:     OutcomeSuccess,
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	got, err := l.List(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d", len(got))
	}
	// Newest first.
	if got[0].ID != "run-c" {
		t.Errorf("newest first expected, got %s", got[0].ID)
	}
}

func TestAppendTrimsToCap(t *testing.T) {
	dir := t.TempDir()
	l := New(filepath.Join(dir, "history.json"))
	l.cap = 3

	for i := 0; i < 10; i++ {
		err := l.Append(Entry{
			ID:         string(rune('a' + i)),
			FinishedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	got, err := l.List(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("trimmed to 3 expected, got %d", len(got))
	}
	if got[0].ID != "j" || got[1].ID != "i" || got[2].ID != "h" {
		t.Fatalf("retention kept wrong entries: %+v", got)
	}
}

func TestCorruptFileDoesntPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := New(path)
	if err := l.Append(Entry{ID: "post-corruption"}); err != nil {
		t.Fatalf("append after corruption: %v", err)
	}
	got, err := l.List(10)
	if err != nil || len(got) != 1 {
		t.Fatalf("post-corruption list: got %v err=%v", got, err)
	}
}

func TestFinalizePendingSelfUpdatesContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	log := New(path)
	started := time.Now().Add(-time.Minute)
	for _, entry := range []Entry{
		{ID: "self-pending", StartedAt: started, Outcome: OutcomePending},
		{ID: "self-failed", StartedAt: started, Outcome: OutcomeError},
		{ID: "main-pending", StartedAt: started, Outcome: OutcomePending},
	} {
		if err := log.Append(entry); err != nil {
			t.Fatalf("Append(%s): %v", entry.ID, err)
		}
	}

	updated, err := log.FinalizePendingSelfUpdates()
	if err != nil {
		t.Fatalf("FinalizePendingSelfUpdates: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	entries, err := log.List(10)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	if got := byID["self-pending"]; got.Outcome != OutcomeSuccess || got.FinishedAt.IsZero() || got.DurationMS <= 0 {
		t.Fatalf("self pending entry was not finalized: %+v", got)
	}
	if got := byID["self-failed"]; got.Outcome != OutcomeError || !got.FinishedAt.IsZero() {
		t.Fatalf("completed self entry changed: %+v", got)
	}
	if got := byID["main-pending"]; got.Outcome != OutcomePending || !got.FinishedAt.IsZero() {
		t.Fatalf("non-self pending entry changed: %+v", got)
	}

	updated, err = log.FinalizePendingSelfUpdates()
	if err != nil || updated != 0 {
		t.Fatalf("second finalize = (%d, %v), want (0, nil)", updated, err)
	}
}

func TestListLimitAndEmptyFileContracts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	log := New(path)
	entries, err := log.List(1)
	if err != nil || entries != nil {
		t.Fatalf("empty file result = (%v, %v), want (nil, nil)", entries, err)
	}

	base := time.Now()
	for i := 0; i < 3; i++ {
		if err := log.Append(Entry{ID: string(rune('a' + i)), FinishedAt: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err = log.List(2)
	if err != nil || len(entries) != 2 || entries[0].ID != "c" || entries[1].ID != "b" {
		t.Fatalf("limited list = (%+v, %v)", entries, err)
	}
}
