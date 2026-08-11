package history

import (
	"strings"
	"time"
)

// FinalizePendingSelfUpdates marks pending self-update entries successful after
// the replacement sidecar boots. Non-self flows remain untouched.
func (l *Log) FinalizePendingSelfUpdates() (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries, err := l.loadLocked()
	if err != nil {
		return 0, err
	}
	updated := 0
	for i := range entries {
		if !strings.HasPrefix(entries[i].ID, "self-") {
			continue
		}
		if entries[i].Outcome != OutcomePending {
			continue
		}
		entries[i].Outcome = OutcomeSuccess
		entries[i].FinishedAt = time.Now()
		entries[i].DurationMS = entries[i].FinishedAt.Sub(entries[i].StartedAt).Milliseconds()
		updated++
	}
	if updated == 0 {
		return 0, nil
	}
	return updated, l.saveLocked(entries)
}
