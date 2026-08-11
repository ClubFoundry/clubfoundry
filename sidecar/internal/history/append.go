package history

import "sort"

// Append records an update attempt and applies the configured history cap.
func (l *Log) Append(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	entries, err := l.loadLocked()
	if err != nil {
		return err
	}
	entries = append(entries, e)

	// Trim to last `cap` by finished_at: newest kept.
	if len(entries) > l.cap {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].FinishedAt.After(entries[j].FinishedAt)
		})
		entries = entries[:l.cap]
	}

	return l.saveLocked(entries)
}
